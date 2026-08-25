package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// 鈹€鈹€ 鐭俊楠岃瘉鐮佺櫥褰?+ 鎵嬫満鍙风粦瀹?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
//
// 闃叉互鐢ㄥ洓淇濋櫓锛堝鐢ㄦ棦鏈夎鏂斤級锛?
//  1. 浜烘満楠岃瘉鏍呮爮锛氬彂鐮?鐧诲綍鍧囪繃 CaptchaHandler.Enforce锛堝惎鐢?澶辫触鍗囩骇寮哄埗锛夛紱
//  2. 鍙戦€佸喎鍗达細鍚屽彿鍐峰嵈鏈熷唴鎷掔粷閲嶅彂锛圧edis TTL 鏍囪锛夛紱
//  3. 姣忔棩涓婇檺锛氬悓鍙锋瘡鏃ュ彂閫佹鏁板彈闄愶紙Redis 24h 璁℃暟锛夛紱
//  4. 楠岃瘉鐮佸皾璇曟鏁帮細閿?5 娆′綔搴燂紝闇€閲嶆柊鑾峰彇锛涚櫥褰曞け璐ヨ鍏?IP 澶辫触璁℃暟銆?

const (
	smsMaxTries         = 5                // 楠岃瘉鐮佹渶澶у皾璇曟鏁帮紝瓒呰繃浣滃簾
	smsCodeKeyPrefix    = "sms:code:"      // 楠岃瘉鐮?
	smsTriesKeyPrefix   = "sms:tries:"     // 灏濊瘯璁℃暟
	smsCoolKeyPrefix    = "sms:cool:"      // 鍙戦€佸喎鍗存爣璁?
	smsDailyKeyPrefix   = "sms:day:"       // 姣忔棩鍙戦€佽鏁?
	smsCodeDigits       = 6                // 楠岃瘉鐮佷綅鏁?
	smsDailyWindow      = 24 * time.Hour   // 姣忔棩璁℃暟绐楀彛
	smsMaxDailyLimit    = 100              // 姣忔棩涓婇檺閰嶇疆涓婇檺
	smsMaxCodeTTL       = 15 * time.Minute // 楠岃瘉鐮佹湁鏁堟湡涓婇檺
)

// smsCodeStore 鎶借薄楠岃瘉鐮佸瓨鍙栵紙鐢熶骇 Redis锛屾祴璇曞唴瀛?fake锛夈€?
type smsCodeStore interface {
	SetCode(ctx context.Context, phone, code string, ttl time.Duration) error
	GetCode(ctx context.Context, phone string) (string, error)
	DelCode(ctx context.Context, phone string) error
	IncrTries(ctx context.Context, phone string) (int, error)
	ResetTries(ctx context.Context, phone string) error
	MarkCooldown(ctx context.Context, phone string, ttl time.Duration) error
	InCooldown(ctx context.Context, phone string) (bool, error)
	IncrDaily(ctx context.Context, phone string) (int, error)
}

// redisSmsCodeStore 鏄?Redis 瀹炵幇銆?
type redisSmsCodeStore struct {
	rdb db.RedisClient
}

func (s redisSmsCodeStore) SetCode(ctx context.Context, phone, code string, ttl time.Duration) error {
	if s.rdb == nil {
		return errors.New("sms: redis unavailable")
	}
	return s.rdb.Set(ctx, smsCodeKeyPrefix+phone, code, ttl).Err()
}

func (s redisSmsCodeStore) GetCode(ctx context.Context, phone string) (string, error) {
	if s.rdb == nil {
		return "", errors.New("sms: redis unavailable")
	}
	v, err := s.rdb.Get(ctx, smsCodeKeyPrefix+phone).Result()
	if err != nil {
		// 杩囨湡/涓嶅瓨鍦ㄨ涓虹┖鐮?
		return "", nil
	}
	return v, nil
}

func (s redisSmsCodeStore) DelCode(ctx context.Context, phone string) error {
	if s.rdb == nil {
		return errors.New("sms: redis unavailable")
	}
	return s.rdb.Del(ctx, smsCodeKeyPrefix+phone).Err()
}

func (s redisSmsCodeStore) IncrTries(ctx context.Context, phone string) (int, error) {
	if s.rdb == nil {
		return 0, errors.New("sms: redis unavailable")
	}
	key := smsTriesKeyPrefix + phone
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		s.rdb.Expire(ctx, key, smsMaxCodeTTL)
	}
	return int(n), nil
}

func (s redisSmsCodeStore) ResetTries(ctx context.Context, phone string) error {
	if s.rdb == nil {
		return errors.New("sms: redis unavailable")
	}
	return s.rdb.Del(ctx, smsTriesKeyPrefix+phone).Err()
}

func (s redisSmsCodeStore) MarkCooldown(ctx context.Context, phone string, ttl time.Duration) error {
	if s.rdb == nil {
		return errors.New("sms: redis unavailable")
	}
	return s.rdb.Set(ctx, smsCoolKeyPrefix+phone, "1", ttl).Err()
}

func (s redisSmsCodeStore) InCooldown(ctx context.Context, phone string) (bool, error) {
	if s.rdb == nil {
		return false, errors.New("sms: redis unavailable")
	}
	n, err := s.rdb.Exists(ctx, smsCoolKeyPrefix+phone).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s redisSmsCodeStore) IncrDaily(ctx context.Context, phone string) (int, error) {
	if s.rdb == nil {
		return 0, errors.New("sms: redis unavailable")
	}
	key := smsDailyKeyPrefix + phone
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		s.rdb.Expire(ctx, key, smsDailyWindow)
	}
	return int(n), nil
}

// smsConfigRow 鏄?ent_sms_config 鐨勫唴瀛樺舰鎬侊紙secret 淇濈暀瀵嗘枃锛夈€?
type smsConfigRow struct {
	Provider         string
	SignName         string
	TemplateID       string
	AccessKeyID      string
	SecretEnc        string
	Endpoint         string
	CodeTTLSeconds   int
	SendIntervalSecs int
	DailyLimit       int
	LoginEnabled     bool
	AutoRegister     bool
	Enabled          bool
}

// SmsHandler 鎻愪緵鐭俊楠岃瘉鐮佺櫥褰曘€佹墜鏈哄彿缁戝畾/瑙ｇ粦涓庣煭淇℃湇鍔￠厤缃鐞嗐€?
type SmsHandler struct {
	auth    *auth.Authenticator
	cfg     *config.Config
	db      entQuerier
	encKey  []byte
	sender  auth.SmsSender
	captcha *CaptchaHandler // 鍙€夛細nil 璺宠繃浜烘満楠岃瘉锛堝崟娴嬬敤锛?
	store   smsCodeStore
}

// NewSmsHandler 鏋勯€犵煭淇?handler锛涘姞瀵嗗瘑閽ユ部鐢?SSO 瀵嗛挜锛岄獙璇佺爜瀛樺偍渚濊禆 Redis銆?
func NewSmsHandler(authenticator *auth.Authenticator, cfg *config.Config, captcha *CaptchaHandler) *SmsHandler {
	return &SmsHandler{
		auth:    authenticator,
		cfg:     cfg,
		db:      pgEntStore{},
		encKey:  auth.LoadOIDCEncryptionKey(),
		sender:  auth.NewHTTPSmsSender(),
		captcha: captcha,
		store:   redisSmsCodeStore{rdb: db.Redis},
	}
}

// RegisterPublicRoutes 鎸傝浇鍏紑璺敱锛堟棤 authMW锛涘灞傞』濂?rlMW锛夈€?
func (h *SmsHandler) RegisterPublicRoutes(mux *http.ServeMux, rlMW func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/auth/sms/status", rlMW(http.HandlerFunc(h.PublicStatus)))
	mux.Handle("POST /v1/auth/sms/code", rlMW(http.HandlerFunc(h.SendCode)))
	mux.Handle("POST /v1/auth/sms/login", rlMW(http.HandlerFunc(h.Login)))
}

// RegisterUserRoutes 鎸傝浇鐢ㄦ埛鑷姪璺敱锛坅uthMW锛夛細鎵嬫満鍙锋煡璇?/ 缁戝畾 / 瑙ｇ粦銆?
func (h *SmsHandler) RegisterUserRoutes(mux *http.ServeMux, authMW func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/auth/sms/bind", authMW(http.HandlerFunc(h.GetBind)))
	mux.Handle("POST /v1/auth/sms/bind", authMW(http.HandlerFunc(h.Bind)))
	mux.Handle("DELETE /v1/auth/sms/bind", authMW(http.HandlerFunc(h.Unbind)))
}

// RegisterAdminRoutes 鎸傝浇绠＄悊璺敱锛坅uthMW + RequireEntPerm("sso:manage")锛夈€?
func (h *SmsHandler) RegisterAdminRoutes(mux *http.ServeMux, authMW func(http.Handler) http.Handler) {
	guard := func(hf http.HandlerFunc) http.Handler {
		return authMW(RequireEntPerm("sso:manage")(hf))
	}
	mux.Handle("GET /v1/ent/sms/config", guard(h.GetConfig))
	mux.Handle("PUT /v1/ent/sms/config", guard(h.UpdateConfig))
}

// 鈹€鈹€ 鍏紑璺敱 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// PublicStatus GET /v1/auth/sms/status
// 鍓嶇鎹鍐冲畾鏄惁灞曠ず"鐭俊鐧诲綍"鏍囩椤点€?
func (h *SmsHandler) PublicStatus(w http.ResponseWriter, r *http.Request) {
	row, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if row == nil || !row.Enabled {
		OK(w, map[string]any{"enabled": false, "login_enabled": false})
		return
	}
	OK(w, map[string]any{"enabled": true, "login_enabled": row.LoginEnabled})
}

type sendSmsCodeRequest struct {
	Phone          string `json:"phone"`
	CaptchaToken   string `json:"captcha_token"`
	CaptchaRandstr string `json:"captcha_randstr"`
	Purpose        string `json:"purpose"` // login锛堥粯璁わ級| bind
}

// SendCode POST /v1/auth/sms/code锛堝叕寮€锛岄』濂?rlMW锛?
// 闃叉互鐢細浜烘満楠岃瘉 + 鍙戦€佸喎鍗?+ 姣忔棩涓婇檺銆?
func (h *SmsHandler) SendCode(w http.ResponseWriter, r *http.Request) {
	var req sendSmsCodeRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	phone := auth.NormalizeSmsPhone(req.Phone)
	if !auth.ValidateSmsPhone(phone) {
		BadRequest(w, "invalid phone number")
		return
	}

	if h.captcha != nil {
		if err := h.captcha.Enforce(w, r, &auth.CaptchaToken{
			Token:   req.CaptchaToken,
			Randstr: req.CaptchaRandstr,
		}); err != nil {
			return
		}
	}

	row, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if row == nil || !row.Enabled {
		Forbidden(w, "鐭俊鏈嶅姟鏈惎鐢?)
		return
	}
	if req.Purpose == "login" || req.Purpose == "" {
		if !row.LoginEnabled {
			Forbidden(w, "鐭俊鐧诲綍鏈惎鐢?)
			return
		}
	}

	ctx := r.Context()
	cool, err := h.store.InCooldown(ctx, phone)
	if err != nil {
		logAndRespond(w, err, http.StatusServiceUnavailable, "楠岃瘉鐮佸瓨鍌ㄤ笉鍙敤")
		return
	}
	if cool {
		TooManyRequests(w)
		return
	}
	daily, err := h.store.IncrDaily(ctx, phone)
	if err != nil {
		logAndRespond(w, err, http.StatusServiceUnavailable, "楠岃瘉鐮佸瓨鍌ㄤ笉鍙敤")
		return
	}
	if daily > row.DailyLimit {
		TooManyRequests(w)
		return
	}

	code, err := auth.GenerateSmsCode(smsCodeDigits)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "generate code failed")
		return
	}

	secret, err := auth.DecryptAESGCM(h.encKey, row.SecretEnc)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "sms secret unavailable")
		return
	}
	if err := h.sender.Send(ctx, &auth.SmsConfig{
		Provider:        row.Provider,
		SignName:        row.SignName,
		TemplateID:      row.TemplateID,
		AccessKeyID:     row.AccessKeyID,
		AccessKeySecret: secret,
		Endpoint:        row.Endpoint,
	}, phone, code); err != nil {
		if errors.Is(err, auth.ErrSmsUnreachable) {
			db.AuditLog(ctx, "", "sms_send_unreachable", r.URL.Path, "phone="+phone, r.RemoteAddr, nil)
			logAndRespond(w, err, http.StatusBadGateway, "鐭俊鏈嶅姟鍟嗕笉鍙揪")
			return
		}
		db.AuditLog(ctx, "", "sms_send_failed", r.URL.Path, "phone="+phone, r.RemoteAddr, nil)
		logAndRespond(w, err, http.StatusBadGateway, "鐭俊鍙戦€佸け璐?)
		return
	}

	ttl := time.Duration(row.CodeTTLSeconds) * time.Second
	if err := h.store.SetCode(ctx, phone, code, ttl); err != nil {
		logAndRespond(w, err, http.StatusServiceUnavailable, "楠岃瘉鐮佸瓨鍌ㄤ笉鍙敤")
		return
	}
	if err := h.store.ResetTries(ctx, phone); err != nil {
		slog.Warn("sms reset tries failed", "error", err)
	}
	interval := time.Duration(row.SendIntervalSecs) * time.Second
	if err := h.store.MarkCooldown(ctx, phone, interval); err != nil {
		slog.Warn("sms mark cooldown failed", "error", err)
	}
	db.AuditLog(ctx, "", "sms_code_sent", r.URL.Path, "phone="+phone+" purpose="+req.Purpose, r.RemoteAddr, nil)
	OK(w, map[string]any{
		"status":         "sent",
		"expire_seconds": row.CodeTTLSeconds,
		"interval":       row.SendIntervalSecs,
	})
}

type smsLoginRequest struct {
	Phone          string `json:"phone"`
	Code           string `json:"code"`
	CaptchaToken   string `json:"captcha_token"`
	CaptchaRandstr string `json:"captcha_randstr"`
}

// Login POST /v1/auth/sms/login锛堝叕寮€锛岄』濂?rlMW锛?
func (h *SmsHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req smsLoginRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	phone := auth.NormalizeSmsPhone(req.Phone)
	if !auth.ValidateSmsPhone(phone) {
		BadRequest(w, "invalid phone number")
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		BadRequest(w, "楠岃瘉鐮佷笉鑳戒负绌?)
		return
	}

	if h.captcha != nil {
		if err := h.captcha.Enforce(w, r, &auth.CaptchaToken{
			Token:   req.CaptchaToken,
			Randstr: req.CaptchaRandstr,
		}); err != nil {
			return
		}
	}

	row, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if row == nil || !row.Enabled || !row.LoginEnabled {
		Forbidden(w, "鐭俊鐧诲綍鏈惎鐢?)
		return
	}

	if !h.verifyCode(w, r, phone, req.Code) {
		return
	}

	ctx := r.Context()
	var user UserResponse
	err = h.db.QueryRow(ctx,
		`SELECT id, email, name, role FROM users WHERE tenant_id = $1 AND phone = $2`,
		db.DefaultTenantID, phone).Scan(&user.ID, &user.Email, &user.Name, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		if !row.AutoRegister {
			NotFound(w, "璇ユ墜鏈哄彿鏈敞鍐?)
			return
		}
		user, err = h.provisionSmsUser(ctx, phone)
		if err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "auto register failed")
			return
		}
	} else if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	if h.captcha != nil {
		h.captcha.ClearFailures(ctx, r)
	}
	token, err := h.auth.GenerateToken(user.ID, user.Email, user.Role, db.DefaultTenantID, auth.RolePermissions[user.Role])
	if err != nil {
		InternalError(w, "authentication failed")
		return
	}
	SetTokenCookie(w, token, int(h.cfg.JWTExpiration.Seconds()), h.cfg.CookieSecure)
	db.AuditLog(ctx, user.ID, "login_success", "/v1/auth/sms/login", "phone="+phone, r.RemoteAddr, nil)
	OK(w, map[string]interface{}{
		"token": token,
		"user":  user,
	})
}

// provisionSmsUser 鑷姩寤哄彿锛歟mail 浠ユ墜鏈哄彿鍏滃簳銆侀殢鏈轰笉鍙櫥褰曞瘑鐮侊紙password_set=FALSE锛夈€?
func (h *SmsHandler) provisionSmsUser(ctx context.Context, phone string) (UserResponse, error) {
	randomPassword := make([]byte, 32)
	if _, err := rand.Read(randomPassword); err != nil {
		return UserResponse{}, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(randomPassword)), bcrypt.DefaultCost)
	if err != nil {
		return UserResponse{}, err
	}
	email := phone + "@sms.local"
	name := "鐢ㄦ埛" + phone[max(0, len(phone)-4):]
	var user UserResponse
	err = h.db.QueryRow(ctx,
		`INSERT INTO users (tenant_id, email, name, password_hash, role, phone, password_set)
		 VALUES ($1, $2, $3, $4, 'user', $5, FALSE)
		 ON CONFLICT DO NOTHING
		 RETURNING id, email, name, role`,
		db.DefaultTenantID, email, name, string(passwordHash), phone).Scan(&user.ID, &user.Email, &user.Name, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		// 骞跺彂寤哄彿锛氬彟涓€璇锋眰宸茬敤璇ユ墜鏈哄彿寤哄彿锛岀洿鎺ュ鐢?
		err = h.db.QueryRow(ctx,
			`SELECT id, email, name, role FROM users WHERE tenant_id = $1 AND phone = $2`,
			db.DefaultTenantID, phone).Scan(&user.ID, &user.Email, &user.Name, &user.Role)
	}
	if err != nil {
		return UserResponse{}, err
	}
	db.AuditLog(ctx, user.ID, "sms_provision", "/v1/auth/sms/login", "phone="+phone, "", nil)
	return user, nil
}

// verifyCode 鏍￠獙楠岃瘉鐮侊紱澶辫触鏃跺凡鍐欏搷搴斿苟杩斿洖 false銆?
// 閿欒绱 smsMaxTries 娆″悗浣滃簾楠岃瘉鐮侊紝骞惰鍏?IP 澶辫触璁℃暟锛堣Е鍙戜汉鏈洪獙璇佸崌绾э級銆?
func (h *SmsHandler) verifyCode(w http.ResponseWriter, r *http.Request, phone, code string) bool {
	ctx := r.Context()
	stored, err := h.store.GetCode(ctx, phone)
	if err != nil {
		logAndRespond(w, err, http.StatusServiceUnavailable, "楠岃瘉鐮佸瓨鍌ㄤ笉鍙敤")
		return false
	}
	if stored == "" {
		BadRequest(w, "楠岃瘉鐮佸凡杩囨湡锛岃閲嶆柊鑾峰彇")
		return false
	}
	if stored != strings.TrimSpace(code) {
		tries, terr := h.store.IncrTries(ctx, phone)
		if terr != nil {
			slog.Warn("sms incr tries failed", "error", terr)
		}
		if h.captcha != nil {
			h.captcha.RecordFailure(ctx, r)
		}
		if tries >= smsMaxTries {
			_ = h.store.DelCode(ctx, phone)
			_ = h.store.ResetTries(ctx, phone)
			BadRequest(w, "楠岃瘉鐮侀敊璇鏁拌繃澶氾紝璇烽噸鏂拌幏鍙?)
			return false
		}
		BadRequest(w, "楠岃瘉鐮侀敊璇?)
		return false
	}
	// 楠岃瘉閫氳繃鍗充綔搴燂紙涓€娆℃€э級锛岄槻閲嶆斁
	_ = h.store.DelCode(ctx, phone)
	_ = h.store.ResetTries(ctx, phone)
	return true
}

// 鈹€鈹€ 鐢ㄦ埛鑷姪锛氭墜鏈哄彿缁戝畾 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// GetBind GET /v1/auth/sms/bind锛坅uthMW锛夎繑鍥炲綋鍓嶇粦瀹氭墜鏈哄彿銆?
func (h *SmsHandler) GetBind(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	var phone *string
	err := h.db.QueryRow(r.Context(),
		`SELECT phone FROM users WHERE id = $1`, claims.UserID).Scan(&phone)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, ErrNotFound)
		return
	}
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	phoneStr := ""
	if phone != nil {
		phoneStr = *phone
	}
	OK(w, map[string]any{"phone": phoneStr, "bound": phoneStr != ""})
}

type smsBindRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

// Bind POST /v1/auth/sms/bind锛坅uthMW锛夐獙璇佺爜鏍￠獙鍚庣粦瀹氭墜鏈哄彿銆?
func (h *SmsHandler) Bind(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	var req smsBindRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	phone := auth.NormalizeSmsPhone(req.Phone)
	if !auth.ValidateSmsPhone(phone) {
		BadRequest(w, "invalid phone number")
		return
	}
	row, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if row == nil || !row.Enabled {
		Forbidden(w, "鐭俊鏈嶅姟鏈惎鐢?)
		return
	}
	if !h.verifyCode(w, r, phone, req.Code) {
		return
	}

	ctx := r.Context()
	if _, err := h.db.Exec(ctx,
		`UPDATE users SET phone = $2, updated_at = NOW() WHERE id = $1`,
		claims.UserID, phone); err != nil {
		if isUniqueViolation(err) {
			JSON(w, http.StatusConflict, APIResponse{Success: false, Error: "璇ユ墜鏈哄彿宸茬粦瀹氬叾浠栬处鍙?})
			return
		}
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	db.AuditLog(ctx, claims.UserID, "sms_bind", "/v1/auth/sms/bind", "phone="+phone, r.RemoteAddr, nil)
	OK(w, map[string]any{"status": "bound", "phone": phone})
}

// Unbind DELETE /v1/auth/sms/bind锛坅uthMW锛?
// 瀹堝崼锛氭棤鍙ｄ护瀵嗙爜涓旀棤涓夋柟韬唤鏃舵嫆缁濊В缁戯紙淇濈暀鑷冲皯涓€绉嶇櫥褰曟柟寮忥級銆?
func (h *SmsHandler) Unbind(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	ctx := r.Context()
	var phone *string
	var passwordSet bool
	var identityCount int
	err := h.db.QueryRow(ctx,
		`SELECT u.phone, u.password_set,
		        (SELECT COUNT(*) FROM ent_user_identities ui WHERE ui.user_id = u.id)
		 FROM users u WHERE u.id = $1`, claims.UserID).
		Scan(&phone, &passwordSet, &identityCount)
	if errors.Is(err, pgx.ErrNoRows) {
		NotFound(w, ErrNotFound)
		return
	}
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if phone == nil || *phone == "" {
		BadRequest(w, "褰撳墠璐﹀彿鏈粦瀹氭墜鏈哄彿")
		return
	}
	if !passwordSet && identityCount == 0 {
		Forbidden(w, "瑙ｇ粦鍚庡皢鏃犲彲鐢ㄧ櫥褰曟柟寮忥紝璇峰厛璁剧疆瀵嗙爜鎴栫粦瀹氬叾浠栫櫥褰曟柟寮?)
		return
	}
	if _, err := h.db.Exec(ctx,
		`UPDATE users SET phone = NULL, updated_at = NOW() WHERE id = $1`, claims.UserID); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	db.AuditLog(ctx, claims.UserID, "sms_unbind", "/v1/auth/sms/bind", "phone="+*phone, r.RemoteAddr, nil)
	OK(w, map[string]string{"status": "unbound"})
}

// 鈹€鈹€ 绠＄悊绔厤缃?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// GetConfig GET /v1/ent/sms/config锛坰ecret 鑴辨晱锛夈€?
func (h *SmsHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	row, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if row == nil {
		OK(w, h.configResponse(&smsConfigRow{
			Provider: auth.SmsAliyun, CodeTTLSeconds: 300, SendIntervalSecs: 60, DailyLimit: 10,
		}, false))
		return
	}
	OK(w, h.configResponse(row, true))
}

func (h *SmsHandler) configResponse(row *smsConfigRow, exists bool) map[string]any {
	return map[string]any{
		"provider":             row.Provider,
		"sign_name":            row.SignName,
		"template_id":          row.TemplateID,
		"access_key_id":        row.AccessKeyID,
		"secret":               maskedSecret,
		"endpoint":             row.Endpoint,
		"code_ttl_seconds":     row.CodeTTLSeconds,
		"send_interval_seconds": row.SendIntervalSecs,
		"daily_limit":          row.DailyLimit,
		"login_enabled":        row.LoginEnabled,
		"auto_register":        row.AutoRegister,
		"enabled":              row.Enabled,
		"exists":               exists,
	}
}

type updateSmsConfigRequest struct {
	Provider          *string `json:"provider"`
	SignName          *string `json:"sign_name"`
	TemplateID        *string `json:"template_id"`
	AccessKeyID       *string `json:"access_key_id"`
	Secret            *string `json:"secret"` // 绌轰覆/鑴辨晱鍗犱綅 = 淇濈暀鍘熷€?
	Endpoint          *string `json:"endpoint"`
	CodeTTLSeconds    *int    `json:"code_ttl_seconds"`
	SendIntervalSecs  *int    `json:"send_interval_seconds"`
	DailyLimit        *int    `json:"daily_limit"`
	LoginEnabled      *bool   `json:"login_enabled"`
	AutoRegister      *bool   `json:"auto_register"`
	Enabled           *bool   `json:"enabled"`
}

// UpdateConfig PUT /v1/ent/sms/config锛堝崟绉熸埛鍗曡 upsert锛夈€?
func (h *SmsHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req updateSmsConfigRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	existing, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	// 浠ユ棦鏈夊€硷紙鎴栫己鐪侊級涓哄簳锛岄€愬瓧娈佃鐩?
	row := &smsConfigRow{
		Provider: auth.SmsAliyun, CodeTTLSeconds: 300, SendIntervalSecs: 60, DailyLimit: 10,
	}
	if existing != nil {
		*row = *existing
	}
	if req.Provider != nil {
		row.Provider = strings.TrimSpace(*req.Provider)
	}
	if !auth.IsKnownSmsProvider(row.Provider) {
		BadRequest(w, "unknown sms provider: "+row.Provider)
		return
	}
	if req.SignName != nil {
		row.SignName = strings.TrimSpace(*req.SignName)
	}
	if req.TemplateID != nil {
		row.TemplateID = strings.TrimSpace(*req.TemplateID)
	}
	if req.AccessKeyID != nil {
		row.AccessKeyID = strings.TrimSpace(*req.AccessKeyID)
	}
	if req.Endpoint != nil {
		row.Endpoint = strings.TrimSpace(*req.Endpoint)
	}
	if req.CodeTTLSeconds != nil {
		row.CodeTTLSeconds = *req.CodeTTLSeconds
	}
	if req.SendIntervalSecs != nil {
		row.SendIntervalSecs = *req.SendIntervalSecs
	}
	if req.DailyLimit != nil {
		row.DailyLimit = *req.DailyLimit
	}
	if req.LoginEnabled != nil {
		row.LoginEnabled = *req.LoginEnabled
	}
	if req.AutoRegister != nil {
		row.AutoRegister = *req.AutoRegister
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}

	// secret锛氭柊鍊煎姞瀵嗭紱绌轰覆/鍗犱綅淇濈暀鍘熷瘑鏂?
	secretEnc := ""
	if existing != nil {
		secretEnc = existing.SecretEnc
	}
	if req.Secret != nil && *req.Secret != "" && *req.Secret != maskedSecret {
		if h.encKey == nil {
			ServiceUnavailable(w, "sms is not configured: "+auth.EnvOIDCSecretKey+" missing")
			return
		}
		enc, err := auth.EncryptAESGCM(h.encKey, *req.Secret)
		if err != nil {
			logAndRespond(w, err, http.StatusInternalServerError, "encrypt sms secret failed")
			return
		}
		secretEnc = enc
	}

	// 鏁板€艰竟鐣?
	if row.CodeTTLSeconds < 60 || row.CodeTTLSeconds > int(smsMaxCodeTTL.Seconds()) {
		BadRequest(w, fmt.Sprintf("code_ttl_seconds 闇€鍦?60-%d 涔嬮棿", int(smsMaxCodeTTL.Seconds())))
		return
	}
	if row.SendIntervalSecs < 0 || row.SendIntervalSecs > 3600 {
		BadRequest(w, "send_interval_seconds 闇€鍦?0-3600 涔嬮棿")
		return
	}
	if row.DailyLimit < 1 || row.DailyLimit > smsMaxDailyLimit {
		BadRequest(w, fmt.Sprintf("daily_limit 闇€鍦?1-%d 涔嬮棿", smsMaxDailyLimit))
		return
	}
	if len(row.SignName) > 64 || len(row.TemplateID) > 64 || len(row.AccessKeyID) > 256 || len(row.Endpoint) > 512 {
		BadRequest(w, "field too long")
		return
	}

	// 鍚敤鍓嶇疆鏍￠獙锛坒ail-loud锛?
	if row.Enabled {
		if secretEnc == "" {
			BadRequest(w, "鐭俊 AccessKeySecret 蹇呴』鍏堥厤缃墠鑳藉惎鐢?)
			return
		}
		if row.Provider != auth.SmsCustom {
			if row.SignName == "" {
				BadRequest(w, "鐭俊绛惧悕锛坰ign_name锛夊繀椤诲厛閰嶇疆鎵嶈兘鍚敤")
				return
			}
			if row.TemplateID == "" {
				BadRequest(w, "鐭俊妯℃澘 ID锛坱emplate_id锛夊繀椤诲厛閰嶇疆鎵嶈兘鍚敤")
				return
			}
		}
		if row.Provider == auth.SmsCustom && row.Endpoint == "" {
			BadRequest(w, "custom 鐭俊鏈嶅姟蹇呴』閰嶇疆鍙戦€佺鐐癸紙endpoint锛?)
			return
		}
	}
	if row.LoginEnabled && !row.Enabled {
		BadRequest(w, "鐭俊鐧诲綍锛坙ogin_enabled锛変緷璧栧彂閫佽兘鍔涳紙enabled锛夛紝璇峰厛鍚敤鐭俊鏈嶅姟")
		return
	}

	if _, err := h.db.Exec(r.Context(),
		`INSERT INTO ent_sms_config (tenant_id, provider, sign_name, template_id, access_key_id,
		                            secret_enc, endpoint, code_ttl_seconds, send_interval_seconds,
		                            daily_limit, login_enabled, auto_register, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT (tenant_id) DO UPDATE SET
		   provider = EXCLUDED.provider, sign_name = EXCLUDED.sign_name,
		   template_id = EXCLUDED.template_id, access_key_id = EXCLUDED.access_key_id,
		   secret_enc = EXCLUDED.secret_enc, endpoint = EXCLUDED.endpoint,
		   code_ttl_seconds = EXCLUDED.code_ttl_seconds,
		   send_interval_seconds = EXCLUDED.send_interval_seconds,
		   daily_limit = EXCLUDED.daily_limit, login_enabled = EXCLUDED.login_enabled,
		   auto_register = EXCLUDED.auto_register, enabled = EXCLUDED.enabled,
		   updated_at = NOW()`,
		db.DefaultTenantID, row.Provider, row.SignName, row.TemplateID, row.AccessKeyID,
		secretEnc, nullString(row.Endpoint), row.CodeTTLSeconds, row.SendIntervalSecs,
		row.DailyLimit, row.LoginEnabled, row.AutoRegister, row.Enabled); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}

	updated, err := h.loadConfig(r.Context())
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, ErrDBUnavailable)
		return
	}
	if updated == nil {
		logAndRespond(w, errors.New("sms config vanished after upsert"),
			http.StatusInternalServerError, "sms config unavailable")
		return
	}
	OK(w, h.configResponse(updated, true))
}

// 鈹€鈹€ 鍐呴儴 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func (h *SmsHandler) loadConfig(ctx context.Context) (*smsConfigRow, error) {
	var row smsConfigRow
	var endpoint *string
	err := h.db.QueryRow(ctx,
		`SELECT provider, sign_name, template_id, access_key_id, secret_enc, endpoint,
		        code_ttl_seconds, send_interval_seconds, daily_limit,
		        login_enabled, auto_register, enabled
		 FROM ent_sms_config WHERE tenant_id = $1`, db.DefaultTenantID).
		Scan(&row.Provider, &row.SignName, &row.TemplateID, &row.AccessKeyID, &row.SecretEnc,
			&endpoint, &row.CodeTTLSeconds, &row.SendIntervalSecs, &row.DailyLimit,
			&row.LoginEnabled, &row.AutoRegister, &row.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if endpoint != nil {
		row.Endpoint = *endpoint
	}
	return &row, nil
}
