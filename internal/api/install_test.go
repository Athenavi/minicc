package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/athenavi/minicc/config"
)

func testConfig(t *testing.T, appSecret string) *config.Config {
	t.Helper()
	t.Setenv("APP_SECRET", appSecret)
	return config.LoadAllowUnconfigured()
}

// TestInitInstallToken_Derived 意图：APP_SECRET 有效时安装令牌必须为确定性派生
// （重启后不变，部署者可在启动日志中拿到同一令牌）。
func TestInitInstallToken_Derived(t *testing.T) {
	installToken = ""
	defer func() { installToken = "" }()

	cfg := testConfig(t, "test-app-secret-32-bytes-long-for-testing!")
	tok1 := InitInstallToken(cfg)
	tok2 := InitInstallToken(cfg) // 幂等：重复调用返回同一令牌
	if tok1 == "" || tok1 != tok2 {
		t.Fatalf("derived install token must be stable and non-empty: %q vs %q", tok1, tok2)
	}
	// 与独立派生结果一致（确定性）
	if tok1 != deriveInstallToken(cfg.AppSecret) {
		t.Fatalf("InitInstallToken != deriveInstallToken: %q vs %q", tok1, deriveInstallToken(cfg.AppSecret))
	}
}

// TestInitInstallToken_RandomFallback 意图：APP_SECRET 缺失（首次部署）时令牌
// 为进程内随机值（Jenkins 模式），两次独立初始化不得相同。
func TestInitInstallToken_RandomFallback(t *testing.T) {
	installToken = ""
	defer func() { installToken = "" }()

	// 用「非空但弱值」模拟 APP_SECRET 缺失：loadDotEnv 只注入 env 为空的键，
	// 弱值（<32 字符）不满足 ValidateAppSecret → 走随机 fallback 分支。
	cfg := testConfig(t, "weak")
	tok1 := InitInstallToken(cfg)
	installToken = ""
	tok2 := InitInstallToken(cfg)
	if tok1 == "" || tok1 == tok2 {
		t.Fatalf("random fallback token must be unique per init: %q vs %q", tok1, tok2)
	}
}

// TestEncryptSecret_Roundtrip 意图：DSN/Redis 密码以 AES-256-GCM 加密后必须可解密还原，
// 且密文不得泄露明文；错误密钥（APP_SECRET 变更）解密必须失败。
func TestEncryptSecret_Roundtrip(t *testing.T) {
	secret := "test-app-secret-32-bytes-long-for-testing!"
	plain := "postgres://user:pass@host:5432/minicc?sslmode=disable"

	enc, err := encryptSecret(secret, plain)
	if err != nil {
		t.Fatalf("encryptSecret: %v", err)
	}
	if enc == "" || enc == plain {
		t.Fatalf("ciphertext must differ from plaintext")
	}
	dec, err := decryptSecret(secret, enc)
	if err != nil {
		t.Fatalf("decryptSecret: %v", err)
	}
	if dec != plain {
		t.Fatalf("roundtrip mismatch: %q", dec)
	}

	// 错误密钥解密失败
	if _, err := decryptSecret("wrong-app-secret-32-bytes-long-value!!", enc); err == nil {
		t.Fatalf("decrypt with wrong key must fail")
	}
	// 空值往返保持空
	if encEmpty, _ := encryptSecret(secret, ""); encEmpty != "" {
		t.Fatalf("empty plaintext should stay empty, got %q", encEmpty)
	}
}

// TestInstallMW 意图：安装端点必须校验 X-Install-Token（header 或 ?token= 查询参数），
// 未携带或错误令牌一律 401；令牌匹配时放行。
func TestInstallMW(t *testing.T) {
	installToken = ""
	defer func() { installToken = "" }()

	cfg := testConfig(t, "test-app-secret-32-bytes-long-for-testing!")
	tok := InitInstallToken(cfg)

	ok := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ok = true })
	h := installMW(next)

	// 无令牌 → 401
	req := httptest.NewRequest(http.MethodGet, "/v1/install/step1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || ok {
		t.Fatalf("no token: got %d, ok=%v", w.Code, ok)
	}

	// 错误令牌 → 401
	ok = false
	req = httptest.NewRequest(http.MethodGet, "/v1/install/step1", nil)
	req.Header.Set("X-Install-Token", "wrong")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || ok {
		t.Fatalf("wrong token: got %d, ok=%v", w.Code, ok)
	}

	// header 正确令牌 → 放行
	ok = false
	req = httptest.NewRequest(http.MethodGet, "/v1/install/step1", nil)
	req.Header.Set("X-Install-Token", tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !ok {
		t.Fatalf("header token: got %d, ok=%v", w.Code, ok)
	}

	// ?token= 查询参数 → 放行（前端从 URL 读取后转 header，curl 便捷路径）
	ok = false
	req = httptest.NewRequest(http.MethodGet, "/v1/install/step1?token="+tok, nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !ok {
		t.Fatalf("query token: got %d, ok=%v", w.Code, ok)
	}

	// 令牌未初始化（非 setup 模式）时恒拒绝
	installToken = ""
	ok = false
	req = httptest.NewRequest(http.MethodGet, "/v1/install/step1", nil)
	req.Header.Set("X-Install-Token", tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || ok {
		t.Fatalf("uninitialized token: got %d, ok=%v", w.Code, ok)
	}
}

// TestInstallLockPersistence 意图：install.lock 加密字段必须能原子落盘并原样读回
// （Step 2 保存 → 重启后 ApplyInstallLockConfig 读取解密的基础）。
func TestInstallLockPersistence(t *testing.T) {
	secret := "test-app-secret-32-bytes-long-for-testing!"
	dsnEnc, _ := encryptSecret(secret, "postgres://u:p@h:5432/db")
	redisEnc, _ := encryptSecret(secret, "localhost:6379")
	pwdEnc, _ := encryptSecret(secret, "s3cr3t")

	original := &InstallLock{
		Completed:     true,
		Step1Done:     true,
		Step2Done:     true,
		Step3Done:     true,
		AppSecretSet:  true,
		DSN:           dsnEnc,
		RedisAddr:     redisEnc,
		RedisPassword: pwdEnc,
		RedisDB:       3,
	}
	if err := SaveInstallLock(original); err != nil {
		t.Fatalf("SaveInstallLock: %v", err)
	}
	defer os.Remove(installLockPath)

	// 二次写入（Step 2 → Step 3 连续更新）：Windows 上 os.Rename 不覆盖已存在目标，必须兼容
	original.Step3Done = true
	original.Completed = true
	if err := SaveInstallLock(original); err != nil {
		t.Fatalf("SaveInstallLock (overwrite): %v", err)
	}

	loaded, err := LoadInstallLock()
	if err != nil {
		t.Fatalf("LoadInstallLock: %v", err)
	}
	if !loaded.Completed || !loaded.Step3Done || loaded.RedisDB != 3 {
		t.Fatalf("loaded lock fields mismatch: %+v", loaded)
	}
	dsn, err := decryptSecret(secret, loaded.DSN)
	if err != nil || dsn != "postgres://u:p@h:5432/db" {
		t.Fatalf("dsn roundtrip failed: %q err=%v", dsn, err)
	}
	addr, _ := decryptSecret(secret, loaded.RedisAddr)
	pwd, _ := decryptSecret(secret, loaded.RedisPassword)
	if addr != "localhost:6379" || pwd != "s3cr3t" {
		t.Fatalf("redis roundtrip failed: addr=%q pwd=%q", addr, pwd)
	}
}

// TestApplyInstallLockConfig 意图：安装完成后重启，main 用 lock 中加密的 DSN/Redis
// 配置覆盖引导值；env 显式设置的 POSTGRES_DSN 优先于 lock。
func TestApplyInstallLockConfig(t *testing.T) {
	secret := "test-app-secret-32-bytes-long-for-testing!"
	dsnEnc, _ := encryptSecret(secret, "postgres://install:user@db.internal:5432/minicc")
	addrEnc, _ := encryptSecret(secret, "redis.internal:6379")
	pwdEnc, _ := encryptSecret(secret, "rp")
	lock := &InstallLock{
		Completed:     true,
		Step1Done:     true,
		Step2Done:     true,
		Step3Done:     true,
		AppSecretSet:  true,
		DSN:           dsnEnc,
		RedisAddr:     addrEnc,
		RedisPassword: pwdEnc,
		RedisDB:       1,
	}
	if err := SaveInstallLock(lock); err != nil {
		t.Fatalf("SaveInstallLock: %v", err)
	}
	defer os.Remove(installLockPath)

	// env 未设置 POSTGRES_DSN → lock 兜底；Redis 以 lock 为准
	cfg := testConfig(t, secret)
	cfg.PostgresDSN = ""
	ApplyInstallLockConfig(cfg)
	if cfg.PostgresDSN != "postgres://install:user@db.internal:5432/minicc" {
		t.Fatalf("cfg.PostgresDSN = %q, want lock value", cfg.PostgresDSN)
	}
	if cfg.RedisAddr != "redis.internal:6379" || cfg.RedisPassword != "rp" || cfg.RedisDB != 1 {
		t.Fatalf("cfg redis = %q/%q/%d, want lock values", cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	}

	// env 显式设置 POSTGRES_DSN → 以 env 为准
	cfg2 := testConfig(t, secret)
	cfg2.PostgresDSN = "postgres://env:user@db.env:5432/minicc"
	ApplyInstallLockConfig(cfg2)
	if cfg2.PostgresDSN != "postgres://env:user@db.env:5432/minicc" {
		t.Fatalf("cfg.PostgresDSN = %q, want env value (env wins)", cfg2.PostgresDSN)
	}
}

// TestStep3_RequiresPool 意图：Step 3 在数据库未配置（db.Pool 为 nil）时必须明确拒绝，
// 而不是 panic 或静默成功。
func TestStep3_RequiresPool(t *testing.T) {
	installToken = ""
	defer func() { installToken = "" }()

	cfg := testConfig(t, "test-app-secret-32-bytes-long-for-testing!")
	h := NewInstallHandler(cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/install/step3",
		http.NoBody).WithContext(context.Background())
	w := httptest.NewRecorder()
	h.Step3(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("step3 without db.Pool: got %d, want 400", w.Code)
	}
}
