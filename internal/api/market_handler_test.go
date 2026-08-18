package api

import (
	"context"
	"errors"
	"testing"
)

// withFakeMarketLookup 临时替换市场查询实现，返回恢复函数。
func withFakeMarketLookup(t *testing.T, fn func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error)) {
	t.Helper()
	orig := marketItemLookup
	marketItemLookup = fn
	t.Cleanup(func() { marketItemLookup = orig })
}

// TestIsItemEnabledForTenant_NoPublishedItem 市场无同名 published 条目 → 放行，
// 保证未上架能力不受市场管控影响。
func TestIsItemEnabledForTenant_NoPublishedItem(t *testing.T) {
	withFakeMarketLookup(t, func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error) {
		return false, false, nil
	})
	enabled, err := IsItemEnabledForTenant(context.Background(), "plugin", "local-tool", "t1")
	if err != nil || !enabled {
		t.Fatalf("expected enabled=true without published item, got enabled=%v err=%v", enabled, err)
	}
}

// TestIsItemEnabledForTenant_PublishedGranted 已发布且租户已安装启用 → 放行。
func TestIsItemEnabledForTenant_PublishedGranted(t *testing.T) {
	withFakeMarketLookup(t, func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error) {
		return true, true, nil
	})
	enabled, err := IsItemEnabledForTenant(context.Background(), "plugin", "crm", "t1")
	if err != nil || !enabled {
		t.Fatalf("expected enabled=true for granted tenant, got enabled=%v err=%v", enabled, err)
	}
}

// TestIsItemEnabledForTenant_PublishedNotGranted 已发布但租户未安装/禁用 → 拦截。
func TestIsItemEnabledForTenant_PublishedNotGranted(t *testing.T) {
	withFakeMarketLookup(t, func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error) {
		return true, false, nil
	})
	enabled, err := IsItemEnabledForTenant(context.Background(), "plugin", "crm", "t1")
	if err != nil || enabled {
		t.Fatalf("expected enabled=false for non-granted tenant, got enabled=%v err=%v", enabled, err)
	}
}

// TestIsItemEnabledForTenant_FailOpen 查询失败 → fail-open 放行。
func TestIsItemEnabledForTenant_FailOpen(t *testing.T) {
	withFakeMarketLookup(t, func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error) {
		return false, false, errors.New("pg down")
	})
	enabled, err := IsItemEnabledForTenant(context.Background(), "skill", "x", "t1")
	if err != nil || !enabled {
		t.Fatalf("expected fail-open on lookup error, got enabled=%v err=%v", enabled, err)
	}
}

// TestCanCatalogTransition 状态机：仅 draft→published→retired；
// retired 为终态，不可回 published（非法迁移由 handler 返回 409）。
func TestCanCatalogTransition(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{"draft", "published", true},
		{"published", "retired", true},
		{"draft", "retired", false},
		{"published", "draft", false},
		{"retired", "published", false},
		{"retired", "draft", false},
		{"published", "published", false},
	}
	for _, c := range cases {
		if got := canCatalogTransition(c.from, c.to); got != c.want {
			t.Errorf("canCatalogTransition(%q,%q) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

// TestFilterSkillsByMarket discover 响应过滤：仅移除市场已发布且未授权的条目。
func TestFilterSkillsByMarket(t *testing.T) {
	withFakeMarketLookup(t, func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error) {
		// 仅 "premium-skill" 在市场已发布且未对该租户授权
		if itemName == "premium-skill" {
			return true, false, nil
		}
		return false, false, nil // 其余未上架 → 放行
	})
	list := []interface{}{
		map[string]interface{}{"name": "csv_summarize"},
		map[string]interface{}{"name": "premium-skill"},
		map[string]interface{}{"no_name_field": true}, // 无名条目原样保留
		"not-a-map", // 非对象条目原样保留
	}
	kept := filterSkillsByMarket(context.Background(), list, "t1")
	if len(kept) != 3 {
		t.Fatalf("expected 3 entries kept, got %d", len(kept))
	}
	for _, raw := range kept {
		if m, ok := raw.(map[string]interface{}); ok {
			if m["name"] == "premium-skill" {
				t.Fatal("premium-skill should have been filtered out")
			}
		}
	}
}
