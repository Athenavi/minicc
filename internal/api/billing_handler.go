package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/billing"
	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/db"
	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/checkout/session"
	"github.com/stripe/stripe-go/v79/webhook"
)

type BillingHandler struct {
	mgr           *billing.Manager
	authenticator *auth.Authenticator
	cfg           *config.Config
	payPalClient  *http.Client
}

func NewBillingHandler(mgr *billing.Manager, authenticator *auth.Authenticator, cfg *config.Config) *BillingHandler {
	if cfg.StripeSecretKey != "" {
		stripe.Key = cfg.StripeSecretKey
	}
	return &BillingHandler{
		mgr: mgr, authenticator: authenticator, cfg: cfg,
		payPalClient: &http.Client{Timeout: 30 * time.Second},
	}
}


func (h *BillingHandler) firstOrigin() string {
	parts := strings.SplitN(h.cfg.CORSOrigins, ",", 2)
	return strings.TrimRight(parts[0], "/ ")
}

func (h *BillingHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID := h.resolveUserID(r)
	if userID == "" {
		Unauthorized(w, "auth required")
		return
	}

	balance, err := h.mgr.GetBalance(userID)
	if err != nil {
		OK(w, map[string]interface{}{"user_id": userID, "balance": 0, "note": "new user"})
		return
	}

	// Also return daily free count for diagnosis
	count, countErr := h.mgr.DailyFreeCount(r.Context(), userID)
	diag := map[string]interface{}{
		"user_id":              userID,
		"balance":              balance,
		"daily_free_limit":     billing.DailyFreeLimit,
		"daily_free_used":      count,
		"daily_free_remaining": billing.DailyFreeLimit - count,
		"within_free_quota":    count < billing.DailyFreeLimit,
	}
	if countErr != nil {
		diag["daily_free_error"] = countErr.Error()
	}

	OK(w, diag)
}

func (h *BillingHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	userID := h.resolveUserID(r)
	if userID == "" {
		Unauthorized(w, "auth required")
		return
	}

	history, err := h.mgr.GetHistory(r.Context(), userID, 50)
	if err != nil {
		OK(w, map[string]interface{}{"user_id": userID, "history": []interface{}{}})
		return
	}

	OK(w, map[string]interface{}{"user_id": userID, "history": history})
}

// GetUsage returns aggregated usage stats for the user: daily token consumption and costs.
func (h *BillingHandler) GetUsage(w http.ResponseWriter, r *http.Request) {
	userID := h.resolveUserID(r)
	if userID == "" {
		Unauthorized(w, "auth required")
		return
	}

	if db.Pool == nil {
		OK(w, map[string]interface{}{"daily": []interface{}{}})
		return
	}

	rows, err := db.ReadPool().Query(r.Context(),
		`SELECT DATE(created_at) as day,
			COUNT(*) as tx_count,
			SUM(CASE WHEN amount < 0 THEN ABS(amount) ELSE 0 END) as credits_spent,
			SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END) as credits_added
		 FROM credit_transactions
		 WHERE user_id = $1 AND created_at >= NOW() - INTERVAL '30 days'
		 GROUP BY DATE(created_at)
		 ORDER BY day DESC`, userID)
	if err != nil {
		OK(w, map[string]interface{}{"daily": []interface{}{}})
		return
	}
	defer rows.Close()

	type dailyUsage struct {
		Day           string `json:"day"`
		TxCount       int    `json:"tx_count"`
		CreditsSpent  int    `json:"credits_spent"`
		CreditsAdded  int    `json:"credits_added"`
	}

	var daily []dailyUsage
	for rows.Next() {
		var d dailyUsage
		var dayTime time.Time
		if err := rows.Scan(&dayTime, &d.TxCount, &d.CreditsSpent, &d.CreditsAdded); err != nil {
			continue
		}
		d.Day = dayTime.Format("2006-01-02")
		daily = append(daily, d)
	}
	if err := rows.Err(); err != nil {
		InternalError(w, "failed to iterate daily usage")
		return
	}

	// Compute totals
	totalSpent := 0
	totalAdded := 0
	for _, d := range daily {
		totalSpent += d.CreditsSpent
		totalAdded += d.CreditsAdded
	}

	OK(w, map[string]interface{}{
		"daily":        daily,
		"total_spent":  totalSpent,
		"total_added":  totalAdded,
		"period_days":  30,
	})
}

func (h *BillingHandler) Recharge(w http.ResponseWriter, r *http.Request) {
	claims := getAuthClaims(r, h.authenticator)
	if claims == nil {
		Unauthorized(w, "auth required")
		return
	}
	if !auth.HasPermission(claims, auth.PermAdminWrite) {
		Forbidden(w, "admin permission required")
		return
	}
	userID := claims.UserID

	var body struct {
		Amount int `json:"amount"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.Amount <= 0 {
		BadRequest(w, "amount > 0 required")
		return
	}

	balance, err := h.mgr.AddCredits(userID, "recharge", body.Amount)
	if err != nil {
		InternalError(w, err.Error())
		return
	}

	OK(w, map[string]interface{}{"user_id": userID, "amount": body.Amount, "balance": balance})
}

func (h *BillingHandler) resolveUserID(r *http.Request) string {
	claims := getAuthClaims(r, h.authenticator)
	if claims != nil {
		return claims.UserID
	}
	return ""
}

// CreateCheckoutSession creates a Stripe Checkout Session.
func (h *BillingHandler) CreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	userID := h.resolveUserID(r)
	if userID == "" {
		Unauthorized(w, "auth required")
		return
	}

	if h.cfg.StripeSecretKey == "" {
		JSON(w, http.StatusNotImplemented, APIResponse{
			Success: false,
			Error:   "Stripe not configured �� set STRIPE_SECRET_KEY",
		})
		return
	}

	var body struct {
		Credits  int    `json:"credits"`  // credits to purchase
		Provider string `json:"provider"` // stripe / paypal
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.Credits <= 0 {
		body.Credits = 1000
	}
	if body.Provider == "" {
		body.Provider = "stripe"
	}

	unitAmount := int64(body.Credits)
	priceID := h.cfg.StripePriceID

	switch body.Provider {
	case "paypal":
		h.createPayPalCheckout(w, r, userID, body.Credits)
		return
	default:
		// Stripe Checkout with card + alipay + wechat_pay
		params := &stripe.CheckoutSessionParams{
			Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
			SuccessURL:        stripe.String(fmt.Sprintf("%s/billing?success=1", h.firstOrigin())),
			CancelURL:         stripe.String(fmt.Sprintf("%s/billing?canceled=1", h.firstOrigin())),
			ClientReferenceID: stripe.String(userID),
			PaymentMethodTypes: []*string{
				stripe.String("card"),
				stripe.String("alipay"),
				stripe.String("wechat_pay"),
			},
		}

		if priceID != "" && priceID != "price_1000_credits" {
			params.LineItems = []*stripe.CheckoutSessionLineItemParams{
				{Price: stripe.String(priceID), Quantity: stripe.Int64(1)},
			}
		} else {
			params.LineItems = []*stripe.CheckoutSessionLineItemParams{
				{
					PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
						Currency: stripe.String("usd"),
						ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
							Name: stripe.String(fmt.Sprintf("%d Credits", body.Credits)),
						},
						UnitAmount: stripe.Int64(unitAmount),
					},
					Quantity: stripe.Int64(1),
				},
			}
		}

		s, err := session.New(params)
		if err != nil {
			slog.Error("stripe checkout session creation failed", "error", err)
			InternalError(w, "Failed to create checkout session")
			return
		}

		if db.Pool != nil {
			if _, err := db.Pool.Exec(r.Context(),
				`INSERT INTO stripe_payments (session_id, user_id, credits, amount_cents, status, created_at)
				 VALUES ($1, $2, $3, $4, 'pending', NOW())
				 ON CONFLICT (session_id) DO NOTHING`,
				s.ID, userID, body.Credits, unitAmount); err != nil {
				slog.Error("insert stripe payment record", "error", err, "session_id", s.ID)
			}
		}

		OK(w, map[string]string{
			"session_id":   s.ID,
			"checkout_url": s.URL,
			"credits":      fmt.Sprintf("%d", body.Credits),
		})
	}
}

// StripeWebhook handles Stripe payment event callbacks.
func (h *BillingHandler) StripeWebhook(w http.ResponseWriter, r *http.Request) {
	if h.cfg.StripeWebhookSecret == "" {
		JSON(w, http.StatusNotImplemented, APIResponse{
			Success: false,
			Error:   "Stripe webhook secret not configured",
		})
		return
	}

	const maxBodyBytes = 65536
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		slog.Error("stripe webhook read body", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	event, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), h.cfg.StripeWebhookSecret)
	if err != nil {
		slog.Error("stripe webhook signature verification failed", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "checkout.session.completed":
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			slog.Error("stripe webhook unmarshal session", "error", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		userID := sess.ClientReferenceID
		if userID == "" {
			slog.Warn("stripe checkout missing client_reference_id")
			w.WriteHeader(http.StatusOK) // acknowledge but don't credit
			return
		}

		// Determine credits from payment record (atomic to prevent double-credit on retry)
		credits := 1000 // default
		if db.Pool != nil {
			var storedCredits int
			err := db.Pool.QueryRow(r.Context(),
				`UPDATE stripe_payments SET status = 'completed', completed_at = NOW()
				 WHERE session_id = $1 AND status = 'pending'
				 RETURNING credits`,
				sess.ID).Scan(&storedCredits)
			if err != nil {
				// Already completed or not found �� skip crediting
				slog.Info("stripe webhook already processed or not found", "session", sess.ID)
				w.WriteHeader(http.StatusOK)
				return
			}
			if storedCredits > 0 {
				credits = storedCredits
			}
		}

		balance, err := h.mgr.AddCredits(userID, "stripe_topup", credits)
		if err != nil {
			slog.Error("stripe webhook credit failed", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		slog.Info("stripe payment completed",
			"user", userID, "credits", credits,
			"session", sess.ID, "balance", balance)

	default:
		slog.Debug("stripe webhook unhandled event", "type", event.Type)
	}

	OK(w, map[string]string{"status": "ok"})
}

// createPayPalCheckout creates a PayPal order and returns the approval URL.
func (h *BillingHandler) createPayPalCheckout(w http.ResponseWriter, r *http.Request, userID string, credits int) {
	if h.cfg.PayPalClientID == "" || h.cfg.PayPalSecret == "" {
		JSON(w, http.StatusNotImplemented, APIResponse{Success: false, Error: "PayPal not configured"})
		return
	}

	amount := fmt.Sprintf("%.2f", float64(credits)/100)
	orderID, approvalURL, err := h.payPalCreateOrder(r.Context(), credits, amount, userID)
	if err != nil {
		slog.Error("paypal order failed", "error", err)
		InternalError(w, "PayPal order failed")
		return
	}

	if db.Pool != nil {
		if _, err := db.Pool.Exec(r.Context(),
			`INSERT INTO stripe_payments (session_id, user_id, credits, status, created_at)
			 VALUES ($1, $2, $3, 'pending', NOW()) ON CONFLICT (session_id) DO NOTHING`,
			orderID, userID, credits); err != nil {
			slog.Error("failed to record paypal payment", "order_id", orderID, "error", err)
			InternalError(w, "Failed to initiate payment")
			return
		}
	}

	OK(w, map[string]string{
		"session_id": orderID, "checkout_url": approvalURL,
		"credits": fmt.Sprintf("%d", credits), "provider": "paypal",
	})
}

// PayPalCapture captures an approved PayPal order and credits the user.
func (h *BillingHandler) PayPalCapture(w http.ResponseWriter, r *http.Request) {
	userID := h.resolveUserID(r)
	if userID == "" {
		Unauthorized(w, "auth required")
		return
	}
	var body struct{ OrderID string `json:"order_id"` }
	if err := DecodeJSON(w, r, &body); err != nil || body.OrderID == "" {
		BadRequest(w, "order_id required")
		return
	}
	if result, err := h.payPalCaptureOrder(r.Context(), body.OrderID); err != nil {
		InternalError(w, "PayPal capture failed")
		return
	} else if status, _ := result["status"].(string); status != "COMPLETED" {
		slog.Error("paypal capture not completed", "order", body.OrderID, "status", status)
		InternalError(w, "PayPal capture was not completed")
		return
	}
	credits := 1000
	if db.Pool != nil {
		var storedCredits int
		err := db.Pool.QueryRow(r.Context(),
			`UPDATE stripe_payments SET status='completed', completed_at=NOW()
			 WHERE session_id=$1 AND user_id=$2 AND status='pending'
			 RETURNING credits`,
			body.OrderID, userID).Scan(&storedCredits)
		if err != nil {
			slog.Info("paypal capture already processed or not found", "order", body.OrderID)
			OK(w, map[string]interface{}{"status": "already_processed"})
			return
		}
		if storedCredits > 0 {
			credits = storedCredits
		}
	}
	balance, err := h.mgr.AddCredits(userID, "paypal_topup", credits)
	if err != nil {
		slog.Error("paypal capture credit failed", "error", err)
		InternalError(w, "crediting failed")
		return
	}
	OK(w, map[string]interface{}{"status": "completed", "balance": balance, "credits": credits})
}

// ���� PayPal REST API helpers ����

func (h *BillingHandler) payPalBaseURL() string {
	if h.cfg.PayPalSandbox {
		return "https://api-m.sandbox.paypal.com"
	}
	return "https://api-m.paypal.com"
}

func (h *BillingHandler) payPalAccessToken(ctx context.Context) (string, error) {
	p := strings.NewReader("grant_type=client_credentials")
	req, err := http.NewRequestWithContext(ctx, "POST", h.payPalBaseURL()+"/v1/oauth2/token", p)
	if err != nil {
		return "", fmt.Errorf("paypal token request: %w", err)
	}
	req.SetBasicAuth(h.cfg.PayPalClientID, h.cfg.PayPalSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := h.payPalClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("paypal token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("paypal token: HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var r struct{ AccessToken string `json:"access_token"` }
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("paypal token decode: %w", err)
	}
	return r.AccessToken, nil
}

func (h *BillingHandler) payPalCreateOrder(ctx context.Context, credits int, amount, userID string) (string, string, error) {
	token, err := h.payPalAccessToken(ctx)
	if err != nil {
		return "", "", err
	}
	body, _ := json.Marshal(map[string]interface{}{
		"intent": "CAPTURE",
		"purchase_units": []map[string]interface{}{{
			"reference_id": userID,
			"description":  fmt.Sprintf("%d Credits", credits),
			"amount":       map[string]string{"currency_code": "USD", "value": amount},
		}},
		"payment_source": map[string]interface{}{
			"paypal": map[string]interface{}{
				"experience_context": map[string]string{
					"payment_method_preference": "IMMEDIATE_PAYMENT_REQUIRED",
					"return_url":               fmt.Sprintf("%s/billing?success=1&provider=paypal", h.firstOrigin()),
					"cancel_url":               fmt.Sprintf("%s/billing?canceled=1", h.firstOrigin()),
				},
			},
		},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", h.payPalBaseURL()+"/v2/checkout/orders", strings.NewReader(string(body)))
	if err != nil {
		return "", "", fmt.Errorf("paypal create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := h.payPalClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("paypal create: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("paypal create: HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var r struct {
		ID    string `json:"id"`
		Links []struct{ Rel, Href string } `json:"links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", fmt.Errorf("paypal decode: %w", err)
	}
	for _, l := range r.Links {
		if l.Rel == "payer-action" {
			return r.ID, l.Href, nil
		}
	}
	return r.ID, "", nil
}

func (h *BillingHandler) payPalCaptureOrder(ctx context.Context, orderID string) (map[string]interface{}, error) {
	token, err := h.payPalAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", h.payPalBaseURL()+"/v2/checkout/orders/"+orderID+"/capture", nil)
	if err != nil {
		return nil, fmt.Errorf("paypal capture request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := h.payPalClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paypal capture: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("paypal capture: HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var r map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("paypal capture decode: %w", err)
	}
	return r, nil
}
