package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/athenavi/chiron/config"
)

func testConfig(t *testing.T, appSecret string) *config.Config {
	t.Helper()
	t.Setenv("APP_SECRET", appSecret)
	return config.LoadAllowUnconfigured()
}

// TestInitInstallToken_Derived 鎰忓浘锛欰PP_SECRET 鏈夋晥鏃跺畨瑁呬护鐗屽繀椤讳负纭畾鎬ф淳鐢?// 锛堥噸鍚悗涓嶅彉锛岄儴缃茶€呭彲鍦ㄥ惎鍔ㄦ棩蹇椾腑鎷垮埌鍚屼竴浠ょ墝锛夈€?func TestInitInstallToken_Derived(t *testing.T) {
	installToken = ""
	defer func() { installToken = "" }()

	cfg := testConfig(t, "test-app-secret-32-bytes-long-for-testing!")
	tok1 := InitInstallToken(cfg)
	tok2 := InitInstallToken(cfg) // 骞傜瓑锛氶噸澶嶈皟鐢ㄨ繑鍥炲悓涓€浠ょ墝
	if tok1 == "" || tok1 != tok2 {
		t.Fatalf("derived install token must be stable and non-empty: %q vs %q", tok1, tok2)
	}
	// 涓庣嫭绔嬫淳鐢熺粨鏋滀竴鑷达紙纭畾鎬э級
	if tok1 != deriveInstallToken(cfg.AppSecret) {
		t.Fatalf("InitInstallToken != deriveInstallToken: %q vs %q", tok1, deriveInstallToken(cfg.AppSecret))
	}
}

// TestInitInstallToken_RandomFallback 鎰忓浘锛欰PP_SECRET 缂哄け锛堥娆￠儴缃诧級鏃朵护鐗?// 涓鸿繘绋嬪唴闅忔満鍊硷紙Jenkins 妯″紡锛夛紝涓ゆ鐙珛鍒濆鍖栦笉寰楃浉鍚屻€?func TestInitInstallToken_RandomFallback(t *testing.T) {
	installToken = ""
	defer func() { installToken = "" }()

	// 鐢ㄣ€岄潪绌轰絾寮卞€笺€嶆ā鎷?APP_SECRET 缂哄け锛歭oadDotEnv 鍙敞鍏?env 涓虹┖鐨勯敭锛?	// 寮卞€硷紙<32 瀛楃锛変笉婊¤冻 ValidateAppSecret 鈫?璧伴殢鏈?fallback 鍒嗘敮銆?	cfg := testConfig(t, "weak")
	tok1 := InitInstallToken(cfg)
	installToken = ""
	tok2 := InitInstallToken(cfg)
	if tok1 == "" || tok1 == tok2 {
		t.Fatalf("random fallback token must be unique per init: %q vs %q", tok1, tok2)
	}
}

// TestEncryptSecret_Roundtrip 鎰忓浘锛欴SN/Redis 瀵嗙爜浠?AES-256-GCM 鍔犲瘑鍚庡繀椤诲彲瑙ｅ瘑杩樺師锛?// 涓斿瘑鏂囦笉寰楁硠闇叉槑鏂囷紱閿欒瀵嗛挜锛圓PP_SECRET 鍙樻洿锛夎В瀵嗗繀椤诲け璐ャ€?func TestEncryptSecret_Roundtrip(t *testing.T) {
	secret := "test-app-secret-32-bytes-long-for-testing!"
	plain := "postgres://user:pass@host:5432/chiron?sslmode=disable"

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

	// 閿欒瀵嗛挜瑙ｅ瘑澶辫触
	if _, err := decryptSecret("wrong-app-secret-32-bytes-long-value!!", enc); err == nil {
		t.Fatalf("decrypt with wrong key must fail")
	}
	// 绌哄€煎線杩斾繚鎸佺┖
	if encEmpty, _ := encryptSecret(secret, ""); encEmpty != "" {
		t.Fatalf("empty plaintext should stay empty, got %q", encEmpty)
	}
}

// TestInstallMW 鎰忓浘锛氬畨瑁呯鐐瑰繀椤绘牎楠?X-Install-Token锛坔eader 鎴??token= 鏌ヨ鍙傛暟锛夛紝
// 鏈惡甯︽垨閿欒浠ょ墝涓€寰?401锛涗护鐗屽尮閰嶆椂鏀捐銆?func TestInstallMW(t *testing.T) {
	installToken = ""
	defer func() { installToken = "" }()

	cfg := testConfig(t, "test-app-secret-32-bytes-long-for-testing!")
	tok := InitInstallToken(cfg)

	ok := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ok = true })
	h := installMW(next)

	// 鏃犱护鐗?鈫?401
	req := httptest.NewRequest(http.MethodGet, "/v1/install/step1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || ok {
		t.Fatalf("no token: got %d, ok=%v", w.Code, ok)
	}

	// 閿欒浠ょ墝 鈫?401
	ok = false
	req = httptest.NewRequest(http.MethodGet, "/v1/install/step1", nil)
	req.Header.Set("X-Install-Token", "wrong")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || ok {
		t.Fatalf("wrong token: got %d, ok=%v", w.Code, ok)
	}

	// header 姝ｇ‘浠ょ墝 鈫?鏀捐
	ok = false
	req = httptest.NewRequest(http.MethodGet, "/v1/install/step1", nil)
	req.Header.Set("X-Install-Token", tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !ok {
		t.Fatalf("header token: got %d, ok=%v", w.Code, ok)
	}

	// ?token= 鏌ヨ鍙傛暟 鈫?鏀捐锛堝墠绔粠 URL 璇诲彇鍚庤浆 header锛宑url 渚挎嵎璺緞锛?	ok = false
	req = httptest.NewRequest(http.MethodGet, "/v1/install/step1?token="+tok, nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !ok {
		t.Fatalf("query token: got %d, ok=%v", w.Code, ok)
	}

	// 浠ょ墝鏈垵濮嬪寲锛堥潪 setup 妯″紡锛夋椂鎭掓嫆缁?	installToken = ""
	ok = false
	req = httptest.NewRequest(http.MethodGet, "/v1/install/step1", nil)
	req.Header.Set("X-Install-Token", tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || ok {
		t.Fatalf("uninitialized token: got %d, ok=%v", w.Code, ok)
	}
}

// TestInstallLockPersistence 鎰忓浘锛歩nstall.lock 鍔犲瘑瀛楁蹇呴』鑳藉師瀛愯惤鐩樺苟鍘熸牱璇诲洖
// 锛圫tep 2 淇濆瓨 鈫?閲嶅惎鍚?ApplyInstallLockConfig 璇诲彇瑙ｅ瘑鐨勫熀纭€锛夈€?func TestInstallLockPersistence(t *testing.T) {
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

	// 浜屾鍐欏叆锛圫tep 2 鈫?Step 3 杩炵画鏇存柊锛夛細Windows 涓?os.Rename 涓嶈鐩栧凡瀛樺湪鐩爣锛屽繀椤诲吋瀹?	original.Step3Done = true
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

// TestApplyInstallLockConfig 鎰忓浘锛氬畨瑁呭畬鎴愬悗閲嶅惎锛宮ain 鐢?lock 涓姞瀵嗙殑 DSN/Redis
// 閰嶇疆瑕嗙洊寮曞鍊硷紱env 鏄惧紡璁剧疆鐨?POSTGRES_DSN 浼樺厛浜?lock銆?func TestApplyInstallLockConfig(t *testing.T) {
	secret := "test-app-secret-32-bytes-long-for-testing!"
	dsnEnc, _ := encryptSecret(secret, "postgres://install:user@db.internal:5432/chiron")
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

	// env 鏈缃?POSTGRES_DSN 鈫?lock 鍏滃簳锛汻edis 浠?lock 涓哄噯
	cfg := testConfig(t, secret)
	cfg.PostgresDSN = ""
	ApplyInstallLockConfig(cfg)
	if cfg.PostgresDSN != "postgres://install:user@db.internal:5432/chiron" {
		t.Fatalf("cfg.PostgresDSN = %q, want lock value", cfg.PostgresDSN)
	}
	if cfg.RedisAddr != "redis.internal:6379" || cfg.RedisPassword != "rp" || cfg.RedisDB != 1 {
		t.Fatalf("cfg redis = %q/%q/%d, want lock values", cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	}

	// env 鏄惧紡璁剧疆 POSTGRES_DSN 鈫?浠?env 涓哄噯
	cfg2 := testConfig(t, secret)
	cfg2.PostgresDSN = "postgres://env:user@db.env:5432/chiron"
	ApplyInstallLockConfig(cfg2)
	if cfg2.PostgresDSN != "postgres://env:user@db.env:5432/chiron" {
		t.Fatalf("cfg.PostgresDSN = %q, want env value (env wins)", cfg2.PostgresDSN)
	}
}

// TestStep3_RequiresPool 鎰忓浘锛歋tep 3 鍦ㄦ暟鎹簱鏈厤缃紙db.Pool 涓?nil锛夋椂蹇呴』鏄庣‘鎷掔粷锛?// 鑰屼笉鏄?panic 鎴栭潤榛樻垚鍔熴€?func TestStep3_RequiresPool(t *testing.T) {
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
