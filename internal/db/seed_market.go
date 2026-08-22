package db

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedMarketCatalog 幂等播种市场目录示例条目（技能/Agent/MCP），仅当目录为空时执行。
// 生产可删除或由管理端自行发布；播种内容为演示级。
func SeedMarketCatalog(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM ent_catalog_items`).Scan(&n); err != nil || n > 0 {
		return err // 已有内容或查询失败：跳过（查询失败不阻塞启动）
	}

	items := []struct {
		typ, name, version, manifest, status string
	}{
		{
			"skill", "web_research", "1.0.0",
			`{"name":"web_research","description":"联网搜索与网页抓取，基于真实资料回答","exec":{"type":"prompt","source":"搜索网络并抓取相关网页，基于真实来源回答用户问题，标注引用。"},"parameters":[]}`,
			"published",
		},
		{
			"skill", "data_analyzer", "1.0.0",
			`{"name":"data_analyzer","description":"数据分析与可视化：读取文件、执行分析、输出结论","exec":{"type":"prompt","source":"读取用户提供的文件，执行数据分析（可用 Python 工具），给出结论与建议。"},"parameters":[]}`,
			"published",
		},
		{
			"agent", "research_agent", "1.0.0",
			`{"name":"research_agent","description":"研究型 Agent：检索、抓取、归纳","system_prompt":"你是研究助手。检索并抓取网页，归纳要点，给出带引用的结论。","tools":[{"name":"web_fetch","description":"抓取网页"},{"name":"read_file","description":"读取文件"}],"llm_config":{"model":"deepseek-chat","max_tokens":4096,"temperature":0.4},"max_turns":6,"timeout_seconds":120}`,
			"published",
		},
		{
			"agent", "code_reviewer", "1.0.0",
			`{"name":"code_reviewer","description":"代码审查 Agent：读代码、找问题、给建议","system_prompt":"你是资深代码审查员。阅读代码，指出安全、性能与正确性问题，给出修复建议。","tools":[{"name":"read_file","description":"读取文件"},{"name":"grep_files","description":"搜索文本"}],"llm_config":{"model":"deepseek-chat","max_tokens":4096,"temperature":0.3},"max_turns":6,"timeout_seconds":120}`,
			"published",
		},
		{
			"mcp", "filesystem", "1.0.0",
			`{"name":"filesystem","description":"MCP 文件系统服务：安全的文件读写","command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","/tmp"],"env":{}}`,
			"published",
		},
		{
			"mcp", "fetch", "1.0.0",
			`{"name":"fetch","description":"MCP 网页抓取服务","command":"npx","args":["-y","@modelcontextprotocol/server-fetch"],"env":{}}`,
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
