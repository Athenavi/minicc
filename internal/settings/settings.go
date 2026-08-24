// Package settings 提供一个基于 AES-256-GCM 的加密设置存取层。
//
// 功能：
//   - 用部署主密钥（APP_SECRET 派生密钥）对敏感配置（LLM/S3/支付密钥、redis/pg 密码等）
//     做加密后存入 system_settings 表；非敏感配置明文存储。
//   - 提供按分类的 SaveConfig / LoadConfig，供后台「系统设置」与管理端 API 使用。
package settings

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

// 密文格式前缀：`v1:` + base64(nonce || ciphertext)
const (
	cipherVersion = "v1:"
	nonceSize     = 12
)

var (
	// ErrEncryptedKeyNotFound APP_SECRET 为空或无法派生密钥时返回。
	ErrEncryptedKeyNotFound = errors.New("settings encryption key unavailable (APP_SECRET not set)")
)

// Store 为 system_settings 表提供带加密的读写。
type Store struct {
	pool *pgxpool.Pool
	aead cipher.AEAD // 由 APP_SECRET 派生；nil 表示未初始化加密（仅允许非敏感存取）
}

// New 创建 Store。appSecret 为空时返回一个仅支持非敏感存取的 Store（敏感键写入会报错）。
func New(pool *pgxpool.Pool, appSecret string) *Store {
	return &Store{
		pool: pool,
		aead: newAEAD(appSecret),
	}
}

// EncryptEnabled 报告当前主密钥是否可用（可加密敏感配置）。
func (s *Store) EncryptEnabled() bool { return s.aead != nil }

// nullableUser 将空 userID 转为 NULL，避免 uuid 列绑定空字符串报错。
func nullableUser(userID string) interface{} {
	if userID == "" {
		return nil
	}
	return userID
}

// newAEAD 从 APP_SECRET 派生 AES-GCM AEAD；秘密为空返回 nil。
func newAEAD(appSecret string) cipher.AEAD {
	if appSecret == "" {
		return nil
	}
	key := sha256.Sum256([]byte("minicc-settings:" + appSecret))
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

// EncryptString 将明文编码为可落库的密文串。
func (s *Store) EncryptString(plain string) (string, error) {
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

// DecryptString 解密 settings.EncryptString 产生的密文。
func (s *Store) DecryptString(enc string) (string, error) {
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

// IsSensitive 判断某个配置键是否为敏感值（需加密入库）。
// 依据：键名包含 password / secret / private_key / api_key / token / dsn。
func IsSensitive(key string) bool {
	k := strings.ToLower(key)
	for _, needle := range []string{"password", "secret", "private_key", "api_key", "dsn", "token"} {
		if strings.Contains(k, needle) {
			return true
		}
	}
	return false
}

// SaveConfig 将某分类的 config 逐键 upsert 到 system_settings。
//   - val 为 nil：删除该键（回落 env/默认值）
//   - 敏感键且 aead 可用：加密后落库，标记 encrypted=true
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
			// 密文以 JSON 字符串形态入库（value 列为 jsonb，裸串无法强转）
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

// LoadConfig 读取某分类全部配置，敏感值解密后返回（面向管理员，需鉴权）。
func (s *Store) LoadConfig(ctx context.Context, category string) (map[string]interface{}, error) {
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
				continue // 无法解密则不返回该键（避免把密文当明文展示）
			}
			rawStr = strings.Trim(plain, `"`)
		}
		var val interface{}
		// 先尝试按原始 jsonb 解析；失败则按纯字符串返回
		if err := json.Unmarshal([]byte(rawStr), &val); err != nil {
			val = rawStr
		}
		out[key] = val
	}
	return out, nil
}