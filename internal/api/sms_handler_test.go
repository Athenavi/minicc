package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// 鈹€鈹€ fake 鍩虹璁炬柦 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// fakeSmsSender 鏄?auth.SmsSender 鐨勬祴璇曟浛韬€?
type fakeSmsSender struct {
	err       error
	calls     int
	lastPhone string
	lastCode  string
}

func (f *fakeSmsSender) Send(ctx context.Context, cfg *auth.SmsConfig, phone, code string) error {
	f.calls++
	f.lastPhone, f.lastCode = phone, code
	return f.err
}

// memSmsStore 鏄?smsCodeStore 鐨勫唴瀛樺疄鐜般€?
type memSmsStore struct {
	codes   map[string]string
	tries   map[string]int
	cool    map[string]bool
	daily   map[string]int
	setErr  error
}

func newMemSmsStore() *memSmsStore {
	return &memSmsStore{codes: map[string]string{}, tries: map[string]int{}, cool: map[string]bool{}, daily: map[string]int{}}
}

func (m *memSmsStore) SetCode(ctx context.Context, phone, code string, ttl time.Duration) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.codes[phone] = code
	return nil
}

func (m *memSmsStore) GetCode(ctx context.Context, phone string) (string, error) {
	return m.codes[phone], nil
}

func (m *memSmsStore) DelCode(ctx context.Context, phone string) error {
	delete(m.codes, phone)
	return nil
}

func (m *memSmsStore) IncrTries(ctx context.Context, phone string) (int, error) {
	m.tries[phone]++
	return m.tries[phone], nil
}

func (m *memSmsStore) ResetTries(ctx context.Context, phone string) error {
	delete(m.tries, phone)
	return nil
}

func (m *memSmsStore) MarkCooldown(ctx context.Context, phone string, ttl time.Duration) error {
	m.cool[phone] = true
	return nil
}

func (m *memSmsStore) InCooldown(ctx context.Context, phone string) (bool, error) {
	return m.cool[phone], nil
}

func (m *memSmsStore) IncrDaily(ctx context.Context, phone string) (int, error) {
	m.daily[phone]++
	return m.daily[phone], nil
}

// smsScan 鏋勯€?ent_sms_config 琛屾壂鎻忥紙鍒楀簭涓?loadConfig 涓€鑷达紝12 鍒楋級銆?
func smsScan(provider, signName, templateID, keyID, secretEnc string, endpoint string,
	ttl, interval, daily int, loginEnabled, autoRegister, enabled bool) func(dest ...any) error {
	return func(dest ...any) error {
		*dest[0].(*string) = provider
		*dest[1].(*string) = signName
		*dest[2].(*string) = templateID
		*dest[3].(*string) = keyID
		*dest[4].(*string) = secretEnc
		var ep *string
		if endpoint != "" {
			ep = &endpoint
		}
		*dest[5].(**string) = ep
		*dest[6].(*int) = ttl
		*dest[7].(*int) = interval
		*dest[8].(*int) = daily
		*dest[9].(*bool) = loginEnabled
		*dest[10].(*bool) = autoRegister
		*dest[11].(*bool) = enabled
		return nil
	}
}

func userScan(id, email, name, role string) func(dest ...any) error {
	return func(dest ...any) error {
		*dest[0].(*string) = id
		*dest[1].(*string) = email
		*dest[2].(*string) = name
		*dest[3].(*string) = role
		return nil
	}
}

func newTestSmsHandler(rowScan func(dest ...any) error, sender auth.SmsSender, store smsCodeStore,
	queryRow func(sql string, args ...any) pgx.Row, exec func(sql string, args ...any) (pgconn.CommandTag, error),
	withCaptcha bool) *SmsHandler {
	var captcha *CaptchaHandler
	if withCaptcha {
		// 浜烘満楠岃瘉鏈厤缃紙涓嶅己鍒讹級浣嗗け璐ヨ鏁板彲鐢?鈫?RecordFailure 鐢熸晥
		captcha = newTestCaptchaHandler(nil, &fakeCaptchaVerifier{}, newMemCounter())
	}
	if queryRow == nil {
		queryRow = func(sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "ent_sms_config") {
				if rowScan == nil {
					return &fakeRow{}
				}
				return &fakeRow{scan: rowScan}
			}
			return &fakeRow{}
		}
	}
	h := &SmsHandler{
		auth:    auth.NewAuthenticator(strings.Repeat("s", 32), time.Hour),
		cfg:     &config.Config{JWTExpiration: time.Hour},
		db:      &fakeQuerier{queryRow: queryRow, exec: exec},
		encKey:  testEncKey,
		sender:  sender,
		captcha: captcha,
		store:   store,
	}
	return h
}

func testSmsReq(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func enabledSmsScan() func(dest ...any) error {
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "sms-secret")
	return smsScan(auth.SmsAliyun, "Chiron", "SMS_1", "key-1", secretEnc, "",
		300, 60, 10, true, false, true)
}

// 鈹€鈹€ PublicStatus 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestSmsPublicStatus_Disabled(t *testing.T) {
	h := newTestSmsHandler(nil, &fakeSmsSender{}, newMemSmsStore(), nil, nil, false)
	w := httptest.NewRecorder()
	h.PublicStatus(w, testSmsReq("GET", "/v1/auth/sms/status", ""))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("expected disabled, got %d %s", w.Code, w.Body.String())
	}
}

func TestSmsPublicStatus_Enabled(t *testing.T) {
	h := newTestSmsHandler(enabledSmsScan(), &fakeSmsSender{}, newMemSmsStore(), nil, nil, false)
	w := httptest.NewRecorder()
	h.PublicStatus(w, testSmsReq("GET", "/v1/auth/sms/status", ""))
	if !strings.Contains(w.Body.String(), `"login_enabled":true`) {
		t.Fatalf("expected login_enabled=true, got %s", w.Body.String())
	}
}

// 鈹€鈹€ SendCode 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestSmsSendCode_Success(t *testing.T) {
	sender := &fakeSmsSender{}
	store := newMemSmsStore()
	h := newTestSmsHandler(enabledSmsScan(), sender, store, nil, nil, true)

	w := httptest.NewRecorder()
	h.SendCode(w, testSmsReq("POST", "/v1/auth/sms/code", `{"phone":"13800138000"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if sender.calls != 1 || sender.lastPhone != "13800138000" {
		t.Fatalf("sender not called correctly: %+v", sender)
	}
	if len(sender.lastCode) != 6 {
		t.Fatalf("expected 6-digit code, got %q", sender.lastCode)
	}
	if store.codes["13800138000"] != sender.lastCode {
		t.Fatalf("code not stored")
	}
	if !store.cool["13800138000"] || store.daily["13800138000"] != 1 {
		t.Fatalf("cooldown/daily not tracked: %+v", store)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]any)
	if data["expire_seconds"].(float64) != 300 {
		t.Fatalf("expected expire_seconds=300, got %v", data["expire_seconds"])
	}
}

func TestSmsSendCode_Cooldown(t *testing.T) {
	store := newMemSmsStore()
	store.cool["13800138000"] = true
	h := newTestSmsHandler(enabledSmsScan(), &fakeSmsSender{}, store, nil, nil, false)
	w := httptest.NewRecorder()
	h.SendCode(w, testSmsReq("POST", "/v1/auth/sms/code", `{"phone":"13800138000"}`))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
}

func TestSmsSendCode_DailyLimit(t *testing.T) {
	store := newMemSmsStore()
	store.daily["13800138000"] = 10 // daily_limit=10
	h := newTestSmsHandler(enabledSmsScan(), &fakeSmsSender{}, store, nil, nil, false)
	w := httptest.NewRecorder()
	h.SendCode(w, testSmsReq("POST", "/v1/auth/sms/code", `{"phone":"13800138000"}`))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
}

func TestSmsSendCode_ProviderRejected(t *testing.T) {
	sender := &fakeSmsSender{err: errors.New("boom: aliyun Code=isv.SMS_SIGNATURE_ILLEGAL")}
	h := newTestSmsHandler(enabledSmsScan(), sender, newMemSmsStore(), nil, nil, false)
	w := httptest.NewRecorder()
	h.SendCode(w, testSmsReq("POST", "/v1/auth/sms/code", `{"phone":"13800138000"}`))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", w.Code)
	}
}

func TestSmsSendCode_NotEnabled(t *testing.T) {
	h := newTestSmsHandler(nil, &fakeSmsSender{}, newMemSmsStore(), nil, nil, false)
	w := httptest.NewRecorder()
	h.SendCode(w, testSmsReq("POST", "/v1/auth/sms/code", `{"phone":"13800138000"}`))
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestSmsSendCode_LoginPurposeRequiresLoginEnabled(t *testing.T) {
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "s")
	scan := smsScan(auth.SmsAliyun, "S", "T", "k", secretEnc, "", 300, 60, 10, false, false, true)
	h := newTestSmsHandler(scan, &fakeSmsSender{}, newMemSmsStore(), nil, nil, false)
	w := httptest.NewRecorder()
	h.SendCode(w, testSmsReq("POST", "/v1/auth/sms/code", `{"phone":"13800138000","purpose":"login"}`))
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (login disabled), got %d", w.Code)
	}
}

func TestSmsSendCode_InvalidPhone(t *testing.T) {
	h := newTestSmsHandler(enabledSmsScan(), &fakeSmsSender{}, newMemSmsStore(), nil, nil, false)
	w := httptest.NewRecorder()
	h.SendCode(w, testSmsReq("POST", "/v1/auth/sms/code", `{"phone":"abc"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// 鈹€鈹€ Login 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestSmsLogin_Success(t *testing.T) {
	store := newMemSmsStore()
	store.codes["13800138000"] = "123456"
	queryRow := func(sql string, args ...any) pgx.Row {
		if strings.Contains(sql, "ent_sms_config") {
			return &fakeRow{scan: enabledSmsScan()}
		}
		if strings.Contains(sql, "phone = $2") {
			return &fakeRow{scan: userScan("u-1", "a@b.c", "Alice", "user")}
		}
		return &fakeRow{}
	}
	h := newTestSmsHandler(nil, &fakeSmsSender{}, store, queryRow, nil, true)

	w := httptest.NewRecorder()
	h.Login(w, testSmsReq("POST", "/v1/auth/sms/login", `{"phone":"13800138000","code":"123456"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// 涓€娆℃€т綔搴?
	if _, exists := store.codes["13800138000"]; exists {
		t.Fatal("code should be deleted after use")
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]any)
	if data["token"] == nil || data["user"] == nil {
		t.Fatalf("expected token+user, got %v", data)
	}
	// SetTokenCookie 鐢熸晥
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == tokenCookieName {
			found = true
		}
	}
	if !found {
		t.Fatal("expected token cookie set")
	}
}

func TestSmsLogin_WrongCodeIncrementsTries(t *testing.T) {
	store := newMemSmsStore()
	store.codes["13800138000"] = "123456"
	h := newTestSmsHandler(enabledSmsScan(), &fakeSmsSender{}, store, nil, nil, true)
	w := httptest.NewRecorder()
	h.Login(w, testSmsReq("POST", "/v1/auth/sms/login", `{"phone":"13800138000","code":"000000"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if store.tries["13800138000"] != 1 {
		t.Fatalf("expected tries=1, got %d", store.tries["13800138000"])
	}
	// 楠岃瘉鐮佷粛鍦紙鏈揪 5 娆★級
	if store.codes["13800138000"] != "123456" {
		t.Fatal("code should survive until max tries")
	}
}

func TestSmsLogin_MaxTriesInvalidatesCode(t *testing.T) {
	store := newMemSmsStore()
	store.codes["13800138000"] = "123456"
	store.tries["13800138000"] = 4 // 鍐嶉敊涓€娆″嵆浣滃簾
	h := newTestSmsHandler(enabledSmsScan(), &fakeSmsSender{}, store, nil, nil, true)
	w := httptest.NewRecorder()
	h.Login(w, testSmsReq("POST", "/v1/auth/sms/login", `{"phone":"13800138000","code":"000000"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if _, exists := store.codes["13800138000"]; exists {
		t.Fatal("code should be invalidated after max tries")
	}
}

func TestSmsLogin_ExpiredCode(t *testing.T) {
	h := newTestSmsHandler(enabledSmsScan(), &fakeSmsSender{}, newMemSmsStore(), nil, nil, false)
	w := httptest.NewRecorder()
	h.Login(w, testSmsReq("POST", "/v1/auth/sms/login", `{"phone":"13800138000","code":"123456"}`))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "杩囨湡") {
		t.Fatalf("expected expired-code 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestSmsLogin_Unregistered(t *testing.T) {
	store := newMemSmsStore()
	store.codes["13800138000"] = "123456"
	// users 鏌ヨ涓?sms_config 鍧囪繑鍥炵浉搴旇锛泆sers 鏃犺褰曪紙fakeRow{} 鈫?ErrNoRows锛?
	queryRow := func(sql string, args ...any) pgx.Row {
		if strings.Contains(sql, "ent_sms_config") {
			return &fakeRow{scan: enabledSmsScan()}
		}
		return &fakeRow{} // users 鈫?ErrNoRows
	}
	h := newTestSmsHandler(nil, &fakeSmsSender{}, store, queryRow, nil, false)
	w := httptest.NewRecorder()
	h.Login(w, testSmsReq("POST", "/v1/auth/sms/login", `{"phone":"13800138000","code":"123456"}`))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSmsLogin_AutoRegister(t *testing.T) {
	store := newMemSmsStore()
	store.codes["13800138000"] = "123456"
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "s")
	scan := smsScan(auth.SmsAliyun, "S", "T", "k", secretEnc, "", 300, 60, 10, true, true, true)
	inserted := false
	queryRow := func(sql string, args ...any) pgx.Row {
		if strings.Contains(sql, "ent_sms_config") {
			return &fakeRow{scan: scan}
		}
		if strings.Contains(sql, "INSERT INTO users") {
			inserted = true
			return &fakeRow{scan: userScan("u-2", "13800138000@sms.local", "鐢ㄦ埛8000", "user")}
		}
		return &fakeRow{} // users SELECT 鈫?ErrNoRows 鈫?瑙﹀彂鑷姩寤哄彿
	}
	h := newTestSmsHandler(nil, &fakeSmsSender{}, store, queryRow, nil, false)
	w := httptest.NewRecorder()
	h.Login(w, testSmsReq("POST", "/v1/auth/sms/login", `{"phone":"13800138000","code":"123456"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !inserted {
		t.Fatal("expected auto-register INSERT")
	}
	if !strings.Contains(w.Body.String(), "13800138000@sms.local") {
		t.Fatalf("expected provisioned email, got %s", w.Body.String())
	}
}

// 鈹€鈹€ Bind / Unbind / GetBind 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func testSmsAuthReq(method, target, body string) *http.Request {
	req := testSmsReq(method, target, body)
	ctx := auth.WithClaims(req.Context(), &auth.Claims{UserID: "u-1", Email: "a@b.c", Role: "user"})
	return req.WithContext(ctx)
}

func TestSmsBind_Success(t *testing.T) {
	store := newMemSmsStore()
	store.codes["13800138000"] = "123456"
	execCalled := false
	exec := func(sql string, args ...any) (pgconn.CommandTag, error) {
		execCalled = true
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	h := newTestSmsHandler(enabledSmsScan(), &fakeSmsSender{}, store, nil, exec, false)
	w := httptest.NewRecorder()
	h.Bind(w, testSmsAuthReq("POST", "/v1/auth/sms/bind", `{"phone":"13800138000","code":"123456"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !execCalled {
		t.Fatal("expected UPDATE users")
	}
}

func TestSmsBind_Conflict(t *testing.T) {
	store := newMemSmsStore()
	store.codes["13800138000"] = "123456"
	exec := func(sql string, args ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, &pgconn.PgError{Code: "23505"}
	}
	h := newTestSmsHandler(enabledSmsScan(), &fakeSmsSender{}, store, nil, exec, false)
	w := httptest.NewRecorder()
	h.Bind(w, testSmsAuthReq("POST", "/v1/auth/sms/bind", `{"phone":"13800138000","code":"123456"}`))
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestSmsUnbind_GuardNoOtherLogin(t *testing.T) {
	phone := "13800138000"
	queryRow := func(sql string, args ...any) pgx.Row {
		if strings.Contains(sql, "ent_sms_config") {
			return &fakeRow{scan: enabledSmsScan()}
		}
		if strings.Contains(sql, "password_set") {
			return &fakeRow{scan: func(dest ...any) error {
				*dest[0].(**string) = &phone
				*dest[1].(*bool) = false   // 鏃犲彛浠ゅ瘑鐮?
				*dest[2].(*int) = 0        // 鏃犱笁鏂硅韩浠?
				return nil
			}}
		}
		return &fakeRow{}
	}
	h := newTestSmsHandler(nil, &fakeSmsSender{}, newMemSmsStore(), queryRow, nil, false)
	w := httptest.NewRecorder()
	h.Unbind(w, testSmsAuthReq("DELETE", "/v1/auth/sms/bind", ""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 guard, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSmsUnbind_Success(t *testing.T) {
	phone := "13800138000"
	queryRow := func(sql string, args ...any) pgx.Row {
		if strings.Contains(sql, "password_set") {
			return &fakeRow{scan: func(dest ...any) error {
				*dest[0].(**string) = &phone
				*dest[1].(*bool) = true
				*dest[2].(*int) = 0
				return nil
			}}
		}
		return &fakeRow{}
	}
	exec := func(sql string, args ...any) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	h := newTestSmsHandler(nil, &fakeSmsSender{}, newMemSmsStore(), queryRow, exec, false)
	w := httptest.NewRecorder()
	h.Unbind(w, testSmsAuthReq("DELETE", "/v1/auth/sms/bind", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSmsGetBind(t *testing.T) {
	phone := "13800138000"
	queryRow := func(sql string, args ...any) pgx.Row {
		if strings.Contains(sql, "SELECT phone FROM users") {
			return &fakeRow{scan: func(dest ...any) error {
				*dest[0].(**string) = &phone
				return nil
			}}
		}
		return &fakeRow{}
	}
	h := newTestSmsHandler(nil, &fakeSmsSender{}, newMemSmsStore(), queryRow, nil, false)
	w := httptest.NewRecorder()
	h.GetBind(w, testSmsAuthReq("GET", "/v1/auth/sms/bind", ""))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "13800138000") {
		t.Fatalf("expected bound phone, got %d %s", w.Code, w.Body.String())
	}
}

// 鈹€鈹€ 绠＄悊绔厤缃?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestSmsGetConfig_Defaults(t *testing.T) {
	h := newTestSmsHandler(nil, &fakeSmsSender{}, newMemSmsStore(), nil, nil, false)
	w := httptest.NewRecorder()
	h.GetConfig(w, testSmsReq("GET", "/v1/ent/sms/config", ""))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"exists":false`) {
		t.Fatalf("expected defaults, got %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "sms-secret") {
		t.Fatal("secret must never leak")
	}
}

func TestSmsUpdateConfig_EnableRequiresSecret(t *testing.T) {
	h := newTestSmsHandler(nil, &fakeSmsSender{}, newMemSmsStore(), nil, nil, false)
	w := httptest.NewRecorder()
	h.UpdateConfig(w, testSmsReq("PUT", "/v1/ent/sms/config",
		`{"provider":"aliyun","sign_name":"S","template_id":"T","enabled":true}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (no secret), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSmsUpdateConfig_CustomRequiresEndpoint(t *testing.T) {
	h := newTestSmsHandler(nil, &fakeSmsSender{}, newMemSmsStore(), nil, nil, false)
	w := httptest.NewRecorder()
	h.UpdateConfig(w, testSmsReq("PUT", "/v1/ent/sms/config",
		`{"provider":"custom","secret":"sk-1","enabled":true}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (custom endpoint), got %d", w.Code)
	}
}

func TestSmsUpdateConfig_LoginRequiresEnabled(t *testing.T) {
	h := newTestSmsHandler(nil, &fakeSmsSender{}, newMemSmsStore(), nil, nil, false)
	w := httptest.NewRecorder()
	h.UpdateConfig(w, testSmsReq("PUT", "/v1/ent/sms/config",
		`{"login_enabled":true}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (login w/o enabled), got %d", w.Code)
	}
}

func TestSmsUpdateConfig_Success(t *testing.T) {
	upserted := false
	exec := func(sql string, args ...any) (pgconn.CommandTag, error) {
		if strings.Contains(sql, "ent_sms_config") {
			upserted = true
		}
		return pgconn.NewCommandTag("INSERT 0 1"), nil
	}
	secretEnc, _ := auth.EncryptAESGCM(testEncKey, "new-secret")
	scan := smsScan(auth.SmsCustom, "", "", "k", secretEnc, "http://sms.example.com/send",
		300, 60, 10, false, false, true)
	queryRow := func(sql string, args ...any) pgx.Row {
		if strings.Contains(sql, "ent_sms_config") {
			if upserted {
				return &fakeRow{scan: scan} // upsert 鍚庡洖璇?
			}
			return &fakeRow{} // 鍒濇 loadConfig 鈫?鏃犻厤缃?
		}
		return &fakeRow{}
	}
	h := newTestSmsHandler(nil, &fakeSmsSender{}, newMemSmsStore(), queryRow, exec, false)

	w := httptest.NewRecorder()
	h.UpdateConfig(w, testSmsReq("PUT", "/v1/ent/sms/config",
		`{"provider":"custom","secret":"new-secret","endpoint":"http://sms.example.com/send","enabled":true}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !upserted {
		t.Fatal("expected config upsert")
	}
	body := w.Body.String()
	if !strings.Contains(body, maskedSecret) || strings.Contains(body, "new-secret") {
		t.Fatalf("secret must be masked in response, got %s", body)
	}
}

