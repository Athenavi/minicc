package db

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedMarketCatalog 骞傜瓑鎾甯傚満鐩綍绀轰緥鏉＄洰锛堟妧鑳?Agent/MCP锛夛紝浠呭綋鐩綍涓虹┖鏃舵墽琛屻€?// 鐢熶骇鍙垹闄ゆ垨鐢辩鐞嗙鑷鍙戝竷锛涙挱绉嶅唴瀹逛负婕旂ず绾с€?func SeedMarketCatalog(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ent_catalog_items`).Scan(&n); err != nil || n > 0 {
		return err // 宸叉湁鍐呭鎴栨煡璇㈠け璐ワ細璺宠繃锛堟煡璇㈠け璐ヤ笉闃诲鍚姩锛?	}

	items := []struct {
		typ, name, version, manifest, status string
	}{
		{
			"skill", "web_research", "1.0.0",
			`{"name":"web_research","description":"鑱旂綉鎼滅储涓庣綉椤垫姄鍙栵紝鍩轰簬鐪熷疄璧勬枡鍥炵瓟","exec":{"type":"prompt","source":"鎼滅储缃戠粶骞舵姄鍙栫浉鍏崇綉椤碉紝鍩轰簬鐪熷疄鏉ユ簮鍥炵瓟鐢ㄦ埛闂锛屾爣娉ㄥ紩鐢ㄣ€?},"parameters":[]}`,
			"published",
		},
		{
			"skill", "data_analyzer", "1.0.0",
			`{"name":"data_analyzer","description":"鏁版嵁鍒嗘瀽涓庡彲瑙嗗寲锛氳鍙栨枃浠躲€佹墽琛屽垎鏋愩€佽緭鍑虹粨璁?,"exec":{"type":"prompt","source":"璇诲彇鐢ㄦ埛鎻愪緵鐨勬枃浠讹紝鎵ц鏁版嵁鍒嗘瀽锛堝彲鐢?Python 宸ュ叿锛夛紝缁欏嚭缁撹涓庡缓璁€?},"parameters":[]}`,
			"published",
		},
		{
			"agent", "research_agent", "1.0.0",
			`{"name":"research_agent","description":"鐮旂┒鍨?Agent锛氭绱€佹姄鍙栥€佸綊绾?,"system_prompt":"浣犳槸鐮旂┒鍔╂墜銆傛绱㈠苟鎶撳彇缃戦〉锛屽綊绾宠鐐癸紝缁欏嚭甯﹀紩鐢ㄧ殑缁撹銆?,"tools":[{"name":"web_fetch","description":"鎶撳彇缃戦〉"},{"name":"read_file","description":"璇诲彇鏂囦欢"}],"llm_config":{"model":"deepseek-chat","max_tokens":4096,"temperature":0.4},"max_turns":6,"timeout_seconds":120}`,
			"published",
		},
		{
			"agent", "code_reviewer", "1.0.0",
			`{"name":"code_reviewer","description":"浠ｇ爜瀹℃煡 Agent锛氳浠ｇ爜銆佹壘闂銆佺粰寤鸿","system_prompt":"浣犳槸璧勬繁浠ｇ爜瀹℃煡鍛樸€傞槄璇讳唬鐮侊紝鎸囧嚭瀹夊叏銆佹€ц兘涓庢纭€ч棶棰橈紝缁欏嚭淇寤鸿銆?,"tools":[{"name":"read_file","description":"璇诲彇鏂囦欢"},{"name":"grep_files","description":"鎼滅储鏂囨湰"}],"llm_config":{"model":"deepseek-chat","max_tokens":4096,"temperature":0.3},"max_turns":6,"timeout_seconds":120}`,
			"published",
		},
		{
			"mcp", "filesystem", "1.0.0",
			`{"name":"filesystem","description":"MCP 鏂囦欢绯荤粺鏈嶅姟锛氬畨鍏ㄧ殑鏂囦欢璇诲啓","command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","/tmp"],"env":{}}`,
			"published",
		},
		{
			"mcp", "fetch", "1.0.0",
			`{"name":"fetch","description":"MCP 缃戦〉鎶撳彇鏈嶅姟","command":"npx","args":["-y","@modelcontextprotocol/server-fetch"],"env":{}}`,
			"published",
		},
	}

	for _, it := range items {
		if _, err := pool.Exec(ctx,
			`INSERT INTO ent_catalog_items (type, name, version, manifest, status)
			 VALUES ($1, $2, $3, $4::jsonb, $5)`,
			it.typ, it.name, it.version, it.manifest, it.status); err != nil {
			slog.Warn("seed market item", "name", it.name, "error", err)
		}
	}
	slog.Info("seeded market catalog", "items", len(items))
	return nil
}
