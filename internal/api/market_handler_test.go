package api

import (
	"context"
	"errors"
	"testing"
)

// withFakeMarketLookup 涓存椂鏇挎崲甯傚満鏌ヨ瀹炵幇锛岃繑鍥炴仮澶嶅嚱鏁般€?
func withFakeMarketLookup(t *testing.T, fn func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error)) {
	t.Helper()
	orig := marketItemLookup
	marketItemLookup = fn
	t.Cleanup(func() { marketItemLookup = orig })
}

// TestIsItemEnabledForTenant_NoPublishedItem 甯傚満鏃犲悓鍚?published 鏉＄洰 鈫?鏀捐锛?
// 淇濊瘉鏈笂鏋惰兘鍔涗笉鍙楀競鍦虹鎺у奖鍝嶃€?
func TestIsItemEnabledForTenant_NoPublishedItem(t *testing.T) {
	withFakeMarketLookup(t, func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error) {
		return false, false, nil
	})
	enabled, err := IsItemEnabledForTenant(context.Background(), "plugin", "local-tool", "t1")
	if err != nil || !enabled {
		t.Fatalf("expected enabled=true without published item, got enabled=%v err=%v", enabled, err)
	}
}

// TestIsItemEnabledForTenant_PublishedGranted 宸插彂甯冧笖绉熸埛宸插畨瑁呭惎鐢?鈫?鏀捐銆?
func TestIsItemEnabledForTenant_PublishedGranted(t *testing.T) {
	withFakeMarketLookup(t, func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error) {
		return true, true, nil
	})
	enabled, err := IsItemEnabledForTenant(context.Background(), "plugin", "crm", "t1")
	if err != nil || !enabled {
		t.Fatalf("expected enabled=true for granted tenant, got enabled=%v err=%v", enabled, err)
	}
}

// TestIsItemEnabledForTenant_PublishedNotGranted 宸插彂甯冧絾绉熸埛鏈畨瑁?绂佺敤 鈫?鎷︽埅銆?
func TestIsItemEnabledForTenant_PublishedNotGranted(t *testing.T) {
	withFakeMarketLookup(t, func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error) {
		return true, false, nil
	})
	enabled, err := IsItemEnabledForTenant(context.Background(), "plugin", "crm", "t1")
	if err != nil || enabled {
		t.Fatalf("expected enabled=false for non-granted tenant, got enabled=%v err=%v", enabled, err)
	}
}

// TestIsItemEnabledForTenant_FailOpen 鏌ヨ澶辫触 鈫?fail-open 鏀捐銆?
func TestIsItemEnabledForTenant_FailOpen(t *testing.T) {
	withFakeMarketLookup(t, func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error) {
		return false, false, errors.New("pg down")
	})
	enabled, err := IsItemEnabledForTenant(context.Background(), "skill", "x", "t1")
	if err != nil || !enabled {
		t.Fatalf("expected fail-open on lookup error, got enabled=%v err=%v", enabled, err)
	}
}

// TestCanCatalogTransition 鐘舵€佹満锛氫粎 draft鈫抪ublished鈫抮etired锛?
// retired 涓虹粓鎬侊紝涓嶅彲鍥?published锛堥潪娉曡縼绉荤敱 handler 杩斿洖 409锛夈€?
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

// TestFilterSkillsByMarket discover 鍝嶅簲杩囨护锛氫粎绉婚櫎甯傚満宸插彂甯冧笖鏈巿鏉冪殑鏉＄洰銆?
func TestFilterSkillsByMarket(t *testing.T) {
	withFakeMarketLookup(t, func(ctx context.Context, itemType, itemName, tenantID string) (bool, bool, error) {
		// 浠?"premium-skill" 鍦ㄥ競鍦哄凡鍙戝竷涓旀湭瀵硅绉熸埛鎺堟潈
		if itemName == "premium-skill" {
			return true, false, nil
		}
		return false, false, nil // 鍏朵綑鏈笂鏋?鈫?鏀捐
	})
	list := []interface{}{
		map[string]interface{}{"name": "csv_summarize"},
		map[string]interface{}{"name": "premium-skill"},
		map[string]interface{}{"no_name_field": true}, // 鏃犲悕鏉＄洰鍘熸牱淇濈暀
		"not-a-map", // 闈炲璞℃潯鐩師鏍蜂繚鐣?
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
