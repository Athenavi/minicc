package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// builtinRoleScan 妯℃嫙 scanRole 鐨勮鎵弿锛氬唴缃鑹?platform_admin銆?
func builtinRoleScan(dest ...any) error {
	*dest[0].(*string) = "22222222-2222-2222-2222-222222222222"
	*dest[1].(*string) = "platform_admin"
	*dest[2].(**string) = strPtr("骞冲彴绠＄悊鍛?)
	*dest[3].(*bool) = true
	*dest[4].(*[]string) = []string{"ent:manage", "sso:manage"}
	*dest[5].(*int) = 3
	*dest[6].(*time.Time) = time.Now()
	*dest[7].(*time.Time) = time.Now()
	return nil
}

func strPtr(s string) *string { return &s }

// 鈹€鈹€ 鍐呯疆瑙掕壊淇濇姢锛?09锛夆攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestUpdateRole_BuiltinPermissionsChange_409(t *testing.T) {
	store := &fakeQuerier{
		queryRow: func(sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM ent_roles") {
				return &fakeRow{scan: builtinRoleScan}
			}
			return &fakeRow{}
		},
		exec: func(sql string, args ...any) (pgconn.CommandTag, error) {
			t.Fatalf("no write should happen for builtin role protection, got: %s", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	h := &EntIdentityHandler{db: store}

	body := `{"permissions":["ent:manage"]}`
	req := httptest.NewRequest("PUT", "/v1/ent/roles/22222222-2222-2222-2222-222222222222", strings.NewReader(body))
	req.SetPathValue("id", "22222222-2222-2222-2222-222222222222")
	w := httptest.NewRecorder()
	h.UpdateRole(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for builtin permissions change, got %d", w.Code)
	}
}

func TestUpdateRole_BuiltinNameChange_409(t *testing.T) {
	store := &fakeQuerier{
		queryRow: func(sql string, args ...any) pgx.Row {
			return &fakeRow{scan: builtinRoleScan}
		},
	}
	h := &EntIdentityHandler{db: store}

	body := `{"name":"renamed"}`
	req := httptest.NewRequest("PUT", "/v1/ent/roles/22222222-2222-2222-2222-222222222222", strings.NewReader(body))
	req.SetPathValue("id", "22222222-2222-2222-2222-222222222222")
	w := httptest.NewRecorder()
	h.UpdateRole(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for builtin name change, got %d", w.Code)
	}
}

func TestDeleteRole_Builtin_409(t *testing.T) {
	store := &fakeQuerier{
		queryRow: func(sql string, args ...any) pgx.Row {
			return &fakeRow{scan: builtinRoleScan}
		},
		exec: func(sql string, args ...any) (pgconn.CommandTag, error) {
			t.Fatalf("no delete should happen for builtin role, got: %s", sql)
			return pgconn.CommandTag{}, nil
		},
	}
	h := &EntIdentityHandler{db: store}

	req := httptest.NewRequest("DELETE", "/v1/ent/roles/22222222-2222-2222-2222-222222222222", nil)
	req.SetPathValue("id", "22222222-2222-2222-2222-222222222222")
	w := httptest.NewRecorder()
	h.DeleteRole(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for builtin delete, got %d", w.Code)
	}
}

func TestUpdateRole_NotFound_404(t *testing.T) {
	store := &fakeQuerier{
		queryRow: func(sql string, args ...any) pgx.Row {
			return &fakeRow{scan: func(dest ...any) error { return pgx.ErrNoRows }}
		},
	}
	h := &EntIdentityHandler{db: store}

	body := `{"display_name":"x"}`
	req := httptest.NewRequest("PUT", "/v1/ent/roles/33333333-3333-3333-3333-333333333333", strings.NewReader(body))
	req.SetPathValue("id", "33333333-3333-3333-3333-333333333333")
	w := httptest.NewRecorder()
	h.UpdateRole(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// 鈹€鈹€ 璇锋眰鏍￠獙锛?00锛孌B 鏃犲叧锛夆攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestSetUserRoles_InvalidBody_400(t *testing.T) {
	h := &EntIdentityHandler{db: &fakeQuerier{}}

	req := httptest.NewRequest("PUT", "/v1/ent/users/44444444-4444-4444-4444-444444444444/roles",
		strings.NewReader("not-json"))
	req.SetPathValue("id", "44444444-4444-4444-4444-444444444444")
	w := httptest.NewRecorder()
	h.SetUserRoles(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetUserRoles_InvalidUserID_400(t *testing.T) {
	h := &EntIdentityHandler{db: &fakeQuerier{}}

	req := httptest.NewRequest("PUT", "/v1/ent/users/not-a-uuid/roles", strings.NewReader(`{"role_ids":[]}`))
	req.SetPathValue("id", "not-a-uuid")
	w := httptest.NewRecorder()
	h.SetUserRoles(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid user id, got %d", w.Code)
	}
}

func TestSetUserRoles_InvalidRoleID_400(t *testing.T) {
	h := &EntIdentityHandler{db: &fakeQuerier{}}

	req := httptest.NewRequest("PUT", "/v1/ent/users/44444444-4444-4444-4444-444444444444/roles",
		strings.NewReader(`{"role_ids":["not-a-uuid"]}`))
	req.SetPathValue("id", "44444444-4444-4444-4444-444444444444")
	w := httptest.NewRecorder()
	h.SetUserRoles(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid role_ids, got %d", w.Code)
	}
}

// 鈹€鈹€ 鎸傝浇杈呭姪鏂规硶锛氫腑闂翠欢閾剧敓鏁?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestRegisterRoutes_RequiresAuth(t *testing.T) {
	h := NewEntIdentityHandler()
	mux := http.NewServeMux()
	// pass-through authMW锛涙潈闄愭牎楠岀敱 RequireEntPerm 閾惧畬鎴?
	h.RegisterRoutes(mux, func(next http.Handler) http.Handler { return next })

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 鏃犺璇?claims 鈫?RequireEntPerm 杩斿洖 401锛堜笉渚濊禆 DB/Redis锛?
	resp, err := http.Get(ts.URL + "/v1/ent/users")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims, got %d", resp.StatusCode)
	}
}

// 鈹€鈹€ 鍒嗛〉鍙傛暟瑙ｆ瀽 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestParsePageQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/ent/users", nil)
	if page, size := parsePageQuery(req); page != 1 || size != 20 {
		t.Fatalf("defaults: got page=%d size=%d", page, size)
	}

	req = httptest.NewRequest("GET", "/v1/ent/users?page=3&page_size=50", nil)
	if page, size := parsePageQuery(req); page != 3 || size != 50 {
		t.Fatalf("explicit: got page=%d size=%d", page, size)
	}

	req = httptest.NewRequest("GET", "/v1/ent/users?page=-1&page_size=9999", nil)
	if page, size := parsePageQuery(req); page != 1 || size != 100 {
		t.Fatalf("clamped: got page=%d size=%d", page, size)
	}
}
