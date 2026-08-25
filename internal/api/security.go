package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

// InputSanitizer 杈撳叆鍑€鍖栧櫒锛岄槻姝?Prompt Injection
type InputSanitizer struct {
	injectionPatterns []*regexp.Regexp
	homoglyphPattern  *regexp.Regexp
}

// NewInputSanitizer 鍒涘缓杈撳叆鍑€鍖栧櫒
func NewInputSanitizer() *InputSanitizer {
	patterns := []string{
		`(?i)ignore\s+(all\s+)?(previous|above|prior|earlier)\s+instructions?`,
		`(?i)forget\s+(everything|all|previous|prior)`,
		`(?i)you\s+are\s+(now|free|not\s+bound|unrestricted|no\s+longer)`,
		`(?i)system\s+prompt`,
		`(?i)new\s+instructions?`,
		`(?i)disregard\s+(all|previous|above|prior)`,
		`(?i)override\s+(safety|instructions?|rules?|policy)`,
		`(?i)act\s+as\s+if\s+you\s+(have|are)\s+no\s+restrictions?`,
		`(?i)pretend\s+you\s+are\s+(not|an?\s+unrestricted)`,
		`(?i)bypass\s+(all|safety|content)\s+(filters?|restrictions?|rules?|checks?)`,
		`(?i)dan\s+(mode|prompt|system)`,
		`(?i)jailbreak`,
		`(?i)developer\s+(mode|instructions?)`,
		`(?i)do\s+not\s+(follow|obey)\s+(the\s+)?(system\s+)?(instructions?|rules?)`,
		`(?i)I\s+am\s+(the\s+)?(system|admin|owner|root|superuser)`,
		`(?i)this\s+is\s+(a\s+)?(test|demo|simulation)`,
		`(?i)repeat\s+(after\s+)?(me|this):`,
		`(?i)translation\s+(mode|system|prompt)`,
		`(?i)markdown\s+(mode|system)`,
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			slog.Warn("缂栬瘧娉ㄥ叆妯″紡澶辫触", "pattern", p, "error", err)
			continue
		}
		compiled = append(compiled, re)
	}

	// 妫€娴嬮潪 ASCII 鍚屽舰寮備箟璇嶏紙Unicode 娣锋穯鏀诲嚮锛氳タ閲屽皵/甯岃厞/闃挎媺浼瓧姣嶄吉瑁呮垚 ASCII锛?	homoglyph := regexp.MustCompile("[\u0400-\u04FF\u0370-\u03FF\u0600-\u06FF]")

	return &InputSanitizer{
		injectionPatterns: compiled,
		homoglyphPattern:  homoglyph,
	}
}

// Sanitize 鍑€鍖栫敤鎴疯緭鍏?// 浣跨敤 XML 鏍囩鍖呰９ + HTML 杞箟锛岄槻姝㈢敤鎴疯緭鍏ヨ LLM 瑙ｉ噴涓烘寚浠?func (s *InputSanitizer) Sanitize(input string) string {
	escaped := htmlEscape(input)
	return fmt.Sprintf("<user_input>\n%s\n</user_input>", escaped)
}

// DetectInjection 妫€娴?Prompt Injection 鏀诲嚮
// 杩斿洖 (鏄惁妫€娴嬪埌, 鍖归厤鐨勬ā寮忔弿杩?
func (s *InputSanitizer) DetectInjection(input string) (bool, string) {
	normalized := normalizeInput(input)

	if s.homoglyphPattern.MatchString(input) {
		return true, "unicode homoglyph / zero-width characters detected"
	}

	for _, pattern := range s.injectionPatterns {
		if pattern.MatchString(normalized) {
			return true, pattern.String()
		}
	}
	return false, ""
}

// normalizeInput 褰掍竴鍖栫敤鎴疯緭鍏ヤ互瑙勯伩娣锋穯鎶€鏈?// 灏嗗叏瑙掆啋鍗婅銆佸幓闄ら浂瀹藉瓧绗︺€佺粺涓€绌虹櫧
func normalizeInput(input string) string {
	var b strings.Builder
	b.Grow(len(input))
	for _, r := range input {
		// 闆跺瀛楃 / 鏍煎紡鎺у埗绗︼紙U+200B-200F, U+FE00-FE0F 鍙樹綋閫夋嫨绗︾瓑锛?		if (r >= '\u200B' && r <= '\u200F') || (r >= '\uFE00' && r <= '\uFE0F') || (r >= '\u2060' && r <= '\u2064') {
			continue
		}
		// 鍏ㄨ ASCII 鈫?鍗婅
		if r >= '\uFF01' && r <= '\uFF5E' {
			b.WriteRune(r - 0xFEE0)
			continue
		}
		b.WriteRune(r)
	}
	lower := strings.ToLower(b.String())
	return strings.Join(strings.Fields(lower), " ")
}

// htmlEscape 杞箟鐢ㄦ埛杈撳叆涓殑鐗规畩瀛楃锛岄槻姝?LLM 灏?// 鐢ㄦ埛杈撳叆涓殑 XML/HTML 鏍囩璇В閲婁负鎸囦护
func htmlEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString("&#60;")
		case '>':
			b.WriteString("&#62;")
		case '&':
			b.WriteString("&#38;")
		case '"':
			b.WriteString("&#34;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SanitizeMiddleware 杈撳叆鍑€鍖栦腑闂翠欢
func SanitizeMiddleware(sanitizer *InputSanitizer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 鍙 POST 璇锋眰杩涜鍑€鍖?			if r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}

			// 妫€鏌?Content-Type
			ct := r.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				next.ServeHTTP(w, r)
				return
			}

			// 璇诲彇璇锋眰浣擄紙淇濈暀鎵€鏈夊瓧娈碉級
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				BadRequest(w, "invalid or too large request body")
				return
			}
			defer r.Body.Close()

			// 鍙闈炵┖ content 瀛楁杩涜鍑€鍖?			if content, ok := body["content"].(string); ok && content != "" {
				// 妫€娴嬫敞鍏?				if detected, pattern := sanitizer.DetectInjection(content); detected {
					slog.Warn("妫€娴嬪埌 Prompt Injection 鏀诲嚮",
						"pattern", pattern,
						"content_preview", truncate(content, 100),
						"path", r.URL.Path,
						"ip", r.RemoteAddr,
					)
					BadRequest(w, "杈撳叆鍐呭鍖呭惈涓嶅厑璁哥殑鎸囦护")
					return
				}
				// 鍑€鍖?content
				body["content"] = sanitizer.Sanitize(content)
			}

			// 閲嶅缓璇锋眰浣擄紙淇濈暀鎵€鏈夊瓧娈碉級
			newBody, err := json.Marshal(body)
			if err != nil {
				slog.Error("sanitize: marshal request body", "error", err)
				InternalError(w, "request processing failed")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(newBody))
			r.ContentLength = int64(len(newBody))

			next.ServeHTTP(w, r)
		})
	}
}

// OutputScanner 杈撳嚭鎵弿鍣紝妫€娴?LLM 鍝嶅簲涓殑鏁忔劅淇℃伅
type OutputScanner struct {
	// 绯荤粺鎻愮ず鍏抽敭璇?	systemPromptKeywords []string
	// API Key 妯″紡
	apiKeyPatterns []*regexp.Regexp
}

// NewOutputScanner 鍒涘缓杈撳嚭鎵弿鍣?func NewOutputScanner() *OutputScanner {
	return &OutputScanner{
		systemPromptKeywords: []string{
			"system prompt",
			"SYSTEM:",
			"You are a helpful assistant",
		},
		apiKeyPatterns: []*regexp.Regexp{
			regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`),
			regexp.MustCompile(`(?i)api[_-]?key[=:]\s*["']?[a-zA-Z0-9]{16,}`),
			regexp.MustCompile(`(?i)password[=:]\s*["']?[^\s"]{8,}`),
		},
	}
}

// Scan 鎵弿 LLM 杈撳嚭
func (s *OutputScanner) Scan(response string) (safe bool, reason string) {
	// 妫€鏌ユ槸鍚︽硠闇茬郴缁熸彁绀?	lower := strings.ToLower(response)
	for _, keyword := range s.systemPromptKeywords {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			return false, "response may contain system prompt content"
		}
	}

	// 妫€鏌ユ槸鍚﹀寘鍚?API Key
	for _, pattern := range s.apiKeyPatterns {
		if pattern.MatchString(response) {
			return false, "response may contain API keys or secrets"
		}
	}

	return true, ""
}

// truncate 鎴柇瀛楃涓?func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}


