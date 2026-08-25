package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/enterprise"
)

// RequireEntPerm 浼佷笟鐗堟潈闄愪腑闂翠欢锛岀鍚嶄笌 RequirePermission 淇濇寔涓€鑷淬€?//
// 鍐崇瓥娴佺▼锛?//  1. 浠?auth.GetClaims 鍙?UserID锛岃皟鐢?enterprise.LoadEffectivePerms 鑱氬悎
//     鐢ㄦ埛鐨勪紒涓氱骇鏈夋晥鏉冮檺锛堢洿鎺ヨ鑹?鈭?缇ょ粍鎴愬憳瑙掕壊锛屽甫 Redis 缂撳瓨锛夈€?//  2. 杩斿洖 nil锛堢敤鎴锋棤 ent 瑙掕壊閰嶇疆锛夆啋 鍥為€€鏃ф潈闄愪綋绯?auth.HasPermission銆?//  3. 杩斿洖闈?nil锛堝惈绌哄垏鐗囷級鈫?浠呭綋 perm 鍦ㄥ垏鐗囧唴鏀捐锛屽惁鍒?403锛?//     绌哄垏鐗囪〃绀?鏄庣‘鏃犳潈闄?锛岀姝㈠洖閫€鏃т綋绯伙紙闃茶秺鏉冿級銆?//  4. LoadEffectivePerms 鍑洪敊锛坋nt 鍩虹璁炬柦鏁呴殰锛夆啋 fail-open 鍥為€€
//     auth.HasPermission + slog.Warn锛屼繚璇佹晠闅滀笉闃绘柇绠＄悊闈€?//
// 娉ㄦ剰锛氭湰涓棿浠朵粎渚涘悗缁换鍔℃寕杞藉埌 /v1/ent/* 璺敱浣跨敤锛屽綋鍓嶄笉娉ㄥ唽浠讳綍璺敱銆?func RequireEntPerm(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := auth.GetClaims(r.Context())
			if claims == nil || claims.UserID == "" {
				logAndRespond(w, errors.New("missing auth claims"),
					http.StatusUnauthorized, ErrAuthRequired)
				return
			}

			perms, err := enterprise.LoadEffectivePerms(r.Context(), claims.UserID)
			if err != nil {
				// ent 鍩虹璁炬柦鏁呴殰锛堝 PG 鏌ヨ澶辫触锛夛細fail-open 鍥為€€鏃ф潈闄愪綋绯?				slog.Warn("ent rbac: load effective perms failed, falling back to legacy permissions",
					"user_id", claims.UserID, "perm", perm, "error", err)
				if !auth.HasPermission(claims, perm) {
					logAndRespond(w, errors.New("insufficient permissions"),
						http.StatusForbidden, "insufficient permissions")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if perms == nil {
				// 鐢ㄦ埛鏃?ent 瑙掕壊閰嶇疆锛氬洖閫€鏃ф潈闄愪綋绯?				if !auth.HasPermission(claims, perm) {
					logAndRespond(w, errors.New("insufficient permissions"),
						http.StatusForbidden, "insufficient permissions")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// 鐢ㄦ埛鏈?ent 閰嶇疆锛堝惈绌哄垏鐗?= 鏄庣‘鏃犳潈闄愶級锛氫粎浠ヨ仛鍚堟潈闄愪负鍑嗭紝绂佹鍥為€€
			for _, p := range perms {
				if p == perm {
					next.ServeHTTP(w, r)
					return
				}
			}
			logAndRespond(w, errors.New("insufficient enterprise permissions"),
				http.StatusForbidden, "insufficient permissions")
		})
	}
}
