// Package settings 鎻愪緵涓€涓熀浜?AES-256-GCM 鐨勫姞瀵嗚缃瓨鍙栧眰銆?//
// 鍔熻兘锛?//   - 鐢ㄩ儴缃蹭富瀵嗛挜锛圓PP_SECRET 娲剧敓瀵嗛挜锛夊鏁忔劅閰嶇疆锛圠LM/S3/鏀粯瀵嗛挜銆乺edis/pg 瀵嗙爜绛夛級
//     鍋氬姞瀵嗗悗瀛樺叆 system_settings 琛紱闈炴晱鎰熼厤缃槑鏂囧瓨鍌ㄣ€?//   - 鎻愪緵鎸夊垎绫荤殑 SaveConfig / LoadConfig锛屼緵鍚庡彴銆岀郴缁熻缃€嶄笌绠＄悊绔?API 浣跨敤銆?package settings

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 瀵嗘枃鏍煎紡鍓嶇紑锛歚v1:` + base64(nonce || ciphertext)
const (
	cipherVersion = "v1:"
	nonceSize     = 12
)

var (
	// ErrEncryptedKeyNotFound APP_SECRET 涓虹┖鎴栨棤娉曟淳鐢熷瘑閽ユ椂杩斿洖銆?	ErrEncryptedKeyNotFound = errors.New("settings encryption key unavailable (APP_SECRET not set)")
)

// Store 涓?system_settings 琛ㄦ彁渚涘甫鍔犲瘑鐨勮鍐欍€?type Store struct {
	pool *pgxpool.Pool
	aead cipher.AEAD // 鐢?APP_SECRET 娲剧敓锛沶il 琛ㄧず鏈垵濮嬪寲鍔犲瘑锛堜粎鍏佽闈炴晱鎰熷瓨鍙栵級
}

// New 鍒涘缓 Store銆俛ppSecret 涓虹┖鏃惰繑鍥炰竴涓粎鏀寔闈炴晱鎰熷瓨鍙栫殑 Store锛堟晱鎰熼敭鍐欏叆浼氭姤閿欙級銆?func New(pool *pgxpool.Pool, appSecret string) *Store {
	return &Store{
		pool: pool,
		aead: newAEAD(appSecret),
	}
}

// EncryptEnabled 鎶ュ憡褰撳墠涓诲瘑閽ユ槸鍚﹀彲鐢紙鍙姞瀵嗘晱鎰熼厤缃級銆?func (s *Store) EncryptEnabled() bool { return s.aead != nil }

// nullableUser 灏嗙┖ userID 杞负 NULL锛岄伩鍏?uuid 鍒楃粦瀹氱┖瀛楃涓叉姤閿欍€?func nullableUser(userID string) interface{} {
	if userID == "" {
		return nil
	}
	return userID
}

// newAEAD 浠?APP_SECRET 娲剧敓 AES-GCM AEAD锛涚瀵嗕负绌鸿繑鍥?nil銆?func newAEAD(appSecret string) cipher.AEAD {
	if appSecret == "" {
		return nil
	}
	key := sha256.Sum256([]byte("chiron-settings:" + appSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil
	}
	return aead
}

// EncryptString 灏嗘槑鏂囩紪鐮佷负鍙惤搴撶殑瀵嗘枃涓层€?func (s *Store) EncryptString(plain string) (string, error) {
	if s.aead == nil {
		return "", ErrEncryptedKeyNotFound
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := s.aead.Seal(nil, nonce, []byte(plain), nil)
	raw := append(nonce, sealed...)
	return cipherVersion + base64.StdEncoding.EncodeToString(raw), nil
}

// DecryptString 瑙ｅ瘑 settings.EncryptString 浜х敓鐨勫瘑鏂囥€?func (s *Store) DecryptString(enc string) (string, error) {
	if s.aead == nil {
		return "", ErrEncryptedKeyNotFound
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, cipherVersion))
	if err != nil {
		return "", err
	}
	if len(raw) < nonceSize {
		return "", errors.New("invalid ciphertext")
	}
	nonce, sealed := raw[:nonceSize], raw[nonceSize:]
	plain, err := s.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// IsSensitive 鍒ゆ柇鏌愪釜閰嶇疆閿槸鍚︿负鏁忔劅鍊硷紙闇€鍔犲瘑鍏ュ簱锛夈€?// 渚濇嵁锛氶敭鍚嶅寘鍚?password / secret / private_key / api_key / token / dsn銆?func IsSensitive(key string) bool {
	k := strings.ToLower(key)
	for _, needle := range []string{"password", "secret", "private_key", "api_key", "dsn", "token"} {
		if strings.Contains(k, needle) {
			return true
		}
	}
	return false
}

// SaveConfig 灏嗘煇鍒嗙被鐨?config 閫愰敭 upsert 鍒?system_settings銆?//   - val 涓?nil锛氬垹闄よ閿紙鍥炶惤 env/榛樿鍊硷級
//   - 鏁忔劅閿笖 aead 鍙敤锛氬姞瀵嗗悗钀藉簱锛屾爣璁?encrypted=true
func (s *Store) SaveConfig(ctx context.Context, category string, config map[string]interface{}, userID string) error {
	if s.pool == nil {
		return errors.New("database unavailable")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for key, val := range config {
		if val == nil {
			if _, err := tx.Exec(ctx,
				`DELETE FROM system_settings WHERE category=$1 AND key=$2`,
				category, key); err != nil {
				return err
			}
			continue
		}
		valueJSON, err := json.Marshal(val)
		if err != nil {
			return err
		}
		encrypted := false
		value := string(valueJSON)
		if sensitive := IsSensitive(key); sensitive {
			if s.aead == nil {
				return ErrEncryptedKeyNotFound
			}
			enc, err := s.EncryptString(value)
			if err != nil {
				return err
			}
			// 瀵嗘枃浠?JSON 瀛楃涓插舰鎬佸叆搴擄紙value 鍒椾负 jsonb锛岃８涓叉棤娉曞己杞級
			encJSON, err := json.Marshal(enc)
			if err != nil {
				return err
			}
			value = string(encJSON)
			encrypted = true
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO system_settings (category, key, value, encrypted, updated_by, updated_at)
			 VALUES ($1, $2, $3::jsonb, $4, $5, NOW())
			 ON CONFLICT (category, key)
			 DO UPDATE SET value=EXCLUDED.value, encrypted=EXCLUDED.encrypted,
			               updated_by=EXCLUDED.updated_by, updated_at=NOW()`,
			category, key, value, encrypted, nullableUser(userID)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// LoadConfig 璇诲彇鏌愬垎绫诲叏閮ㄩ厤缃紝鏁忔劅鍊艰В瀵嗗悗杩斿洖锛堥潰鍚戠鐞嗗憳锛岄渶閴存潈锛夈€?func (s *Store) LoadConfig(ctx context.Context, category string) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	if s.pool == nil {
		return out, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT key, value, encrypted FROM system_settings WHERE category=$1`, category)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var raw json.RawMessage
		var encrypted bool
		if err := rows.Scan(&key, &raw, &encrypted); err != nil {
			continue
		}
		rawStr := strings.Trim(string(raw), `"`)
		if encrypted {
			plain, err := s.DecryptString(rawStr)
			if err != nil {
				slog.Warn("settings decrypt failed", "category", category, "key", key, "error", err)
				continue // 鏃犳硶瑙ｅ瘑鍒欎笉杩斿洖璇ラ敭锛堥伩鍏嶆妸瀵嗘枃褰撴槑鏂囧睍绀猴級
			}
			rawStr = strings.Trim(plain, `"`)
		}
		var val interface{}
		// 鍏堝皾璇曟寜鍘熷 jsonb 瑙ｆ瀽锛涘け璐ュ垯鎸夌函瀛楃涓茶繑鍥?		if err := json.Unmarshal([]byte(rawStr), &val); err != nil {
			val = rawStr
		}
		out[key] = val
	}
	return out, nil
}
