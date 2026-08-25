package api

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/engine"
)

// SkillHandler proxies skill requests to the Python AI engine.
type SkillHandler struct {
	python *engine.PythonClient
}

func NewSkillHandler(python *engine.PythonClient) *SkillHandler {
	return &SkillHandler{python: python}
}

var validSkillName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// RegisterRoutes 娉ㄥ唽鎶€鑳借矾鐢便€傚畨鍏ㄤ慨澶嶏紙P0-S2锛夛細鎵€鏈夎矾鐢卞繀椤荤粡杩?authMW
// 锛堟妧鑳藉嵆浠ｇ爜锛屾湭璁よ瘉鍙?install/run = 鏈璇?RCE锛夛紝骞舵寕 rlMW 涓?sanitizeMW銆?func (h *SkillHandler) RegisterRoutes(mux *http.ServeMux, authMW, rlMW, sanitizeMW routeMiddleware) {
	// 鍚屾椂娉ㄥ唽绮剧‘鍜屽熬鏂滄潬鍙樹綋锛欸o ServeMux 涓?"/v1/skills/" 鍙尮閰嶅瓙鏍戯紙涓嶅尮閰?"/v1/skills"锛夛紝
	// 鑰屽墠绔皟鐢ㄧ殑鏄?GET /v1/skills锛孭ython 绔敞鍐岀殑涔熸槸 /v1/skills銆?	mux.Handle("GET /v1/skills", authMW(rlMW(http.HandlerFunc(h.proxy))))
	mux.Handle("GET /v1/skills/", authMW(rlMW(http.HandlerFunc(h.proxy))))
	mux.Handle("POST /v1/skills/install", authMW(rlMW(sanitizeMW(http.HandlerFunc(h.proxy)))))
	mux.Handle("POST /v1/skills/generate", authMW(rlMW(sanitizeMW(http.HandlerFunc(h.proxy)))))
	mux.Handle("DELETE /v1/skills/{name}", authMW(rlMW(http.HandlerFunc(h.proxyDelete))))
	mux.Handle("GET /v1/skills/discover", authMW(rlMW(http.HandlerFunc(h.proxy))))
	// 鍚仠锛圥UT锛変笌杩愯锛圥OST run锛夆€斺€旀妧鑳藉伐浣滃彴涓婚摼璺?	mux.Handle("PUT /v1/skills/{name}", authMW(rlMW(sanitizeMW(http.HandlerFunc(h.proxy)))))
	mux.Handle("POST /v1/skills/{name}/run", authMW(rlMW(sanitizeMW(http.HandlerFunc(h.proxy)))))
	// 甯傚満鎶€鑳芥敞鍐岋紙P1 淇锛氬墠绔?SkillMarketCard 璋冪敤 /v1/skills/{id}/register锛?	// 姝ゅ墠涓ょ鍧囨棤姝よ矾鐢卞鑷存敞鍐?404锛?	mux.Handle("POST /v1/skills/{name}/register", authMW(rlMW(http.HandlerFunc(h.register))))
}

// register 灏嗚兘鍔涙敞鍐屼腑蹇冪殑鎶€鑳芥敞鍐屼负鏈湴鎶€鑳斤紙杞彂 Python /v1/skills/{name}/register锛夈€?func (h *SkillHandler) register(w http.ResponseWriter, r *http.Request) {
	if h.python == nil {
		InternalError(w, "python engine not available")
		return
	}
	name := r.PathValue("name")
	if name == "" {
		BadRequest(w, "name is required")
		return
	}
	if !validSkillName.MatchString(name) {
		BadRequest(w, "invalid skill name")
		return
	}
	claims := auth.GetClaims(r.Context())
	target := "/v1/skills/" + name + "/register"
	if claims != nil {
		target += "?user_id=" + claims.UserID + "&tenant_id=" + claims.TenantID
	}
	var body map[string]interface{}
	if r.ContentLength > 0 {
		if err := DecodeJSON(w, r, &body); err != nil {
			BadRequest(w, ErrInvalidReq)
			return
		}
	} else {
		body = map[string]interface{}{}
	}
	var result map[string]interface{}
	if err := h.python.PostJSON(r.Context(), target, body, &result); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "python engine error")
		return
	}
	OK(w, result)
}

// proxy forwards the request to the Python engine.
func (h *SkillHandler) proxy(w http.ResponseWriter, r *http.Request) {
	if h.python == nil {
		InternalError(w, "python engine not available")
		return
	}

	var result map[string]interface{}
	var err error

	switch r.Method {
	case "GET":
		// 瑙勮寖鍖栬浆鍙戣矾寰勶細Python 绔敞鍐岀殑鏄?/v1/skills锛堟棤灏炬枩鏉狅級
		path := strings.TrimSuffix(r.URL.Path, "/")
		err = h.python.GetJSON(r.Context(), path, &result)
		if err == nil && strings.HasSuffix(path, "/discover") {
			filterDiscoverByMarket(r.Context(), result, skillTenantID(r))
		}
	case "POST", "PUT":
		var body map[string]interface{}
		if err := DecodeJSON(w, r, &body); err != nil {
			BadRequest(w, ErrInvalidReq)
			return
		}
		// 韬唤娉ㄥ叆锛堢綉鍏充负鍞竴鍙俊杈圭晫锛夛細寮曟搸鏃犻壌鏉冿紝蹇呴』鐢辩綉鍏虫敞鍏ョ鎴?鐢ㄦ埛韬唤銆?		if tid := skillTenantID(r); tid != "" {
			body["tenant_id"] = tid
		}
		if uid := auth.GetClaims(r.Context()); uid != nil && uid.UserID != "" {
			body["user_id"] = uid.UserID
		}
		if r.Method == "PUT" {
			err = h.python.PutJSON(r.Context(), r.URL.Path, body, &result)
		} else {
			err = h.python.PostJSON(r.Context(), r.URL.Path, body, &result)
		}
	default:
		BadRequest(w, "unsupported method")
		return
	}

	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "python engine error")
		return
	}
	OK(w, result)
}

// proxyDelete forwards DELETE requests to the Python engine.
func (h *SkillHandler) proxyDelete(w http.ResponseWriter, r *http.Request) {
	if h.python == nil {
		InternalError(w, "python engine not available")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		BadRequest(w, "name is required")
		return
	}
	if !validSkillName.MatchString(name) {
		BadRequest(w, "invalid skill name")
		return
	}

	var result map[string]interface{}
	if err := h.python.DeleteJSON(r.Context(), "/v1/skills/"+name, &result); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "python engine error")
		return
	}
	OK(w, result)
}

// skillTenantID 鍙栧綋鍓嶇鎴?ID锛歝laims 浼樺厛锛岀己鐪佸洖閫€榛樿绉熸埛銆?func skillTenantID(r *http.Request) string {
	if claims := auth.GetClaims(r.Context()); claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return DefaultTenantID
}

// filterDiscoverByMarket 瀵?discover 浠ｇ悊鍝嶅簲鍋氬競鍦虹櫧鍚嶅崟杩囨护锛?// 浠呭綋甯傚満瀛樺湪鍚屽悕 published 鏉＄洰鏃舵寜绉熸埛鎺堟潈杩囨护锛圛sItemEnabledForTenant
// 鍐呴儴淇濊瘉鏈笂鏋惰兘鍔涗笌鏌ヨ鏁呴殰鍧囨斁琛岋級銆侾G 涓嶅彲鐢ㄦ椂鏁翠綋璺宠繃锛岄伩鍏嶉€愭潯 warn銆?func filterDiscoverByMarket(ctx context.Context, result map[string]interface{}, tenantID string) {
	if db.ReadPool() == nil {
		return
	}
	// 鍏煎澶氱鍝嶅簲缁撴瀯锛氬湪甯歌鍒楄〃閿腑瀹氫綅鎶€鑳芥暟缁?	var list []interface{}
	var listKey string
	for _, key := range []string{"skills", "items", "data", "list"} {
		if arr, ok := result[key].([]interface{}); ok {
			list, listKey = arr, key
			break
		}
	}
	if listKey == "" {
		return
	}
	result[listKey] = filterSkillsByMarket(ctx, list, tenantID)
}

// filterSkillsByMarket 瀵规妧鑳藉垪琛ㄩ€愰」鍋氬競鍦洪棬鎺ц繃婊わ紙绾€昏緫锛岀嫭绔嬪彲娴嬶級銆?func filterSkillsByMarket(ctx context.Context, list []interface{}, tenantID string) []interface{} {
	kept := make([]interface{}, 0, len(list))
	for _, raw := range list {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			kept = append(kept, raw)
			continue
		}
		name, _ := entry["name"].(string)
		if name == "" {
			kept = append(kept, raw)
			continue
		}
		if enabled, _ := IsItemEnabledForTenant(ctx, "skill", name, tenantID); enabled {
			kept = append(kept, raw)
		}
	}
	return kept
}
