package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/db"
	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database management",
	Long:  `Manage MiniCC database.`,
}

var dbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show database status",
	RunE:  runDBStatus,
}

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	RunE:  runDBMigrate,
}

func init() {
	dbCmd.AddCommand(dbStatusCmd)
	dbCmd.AddCommand(dbMigrateCmd)
}

// getDSN 读取 POSTGRES_DSN，默认与 config 一致
func getDSN() string {
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://minicc:minicc@localhost:5432/minicc?sslmode=disable"
}

// sanitizeDSN 隐藏连接串中的密码，避免打印泄露
func sanitizeDSN(dsn string) string {
	const marker = "://"
	i := 0
	if idx := strings.Index(dsn, marker); idx >= 0 {
		i = idx + len(marker)
	}
	rest := dsn[i:]
	// 密码可能含 @，host 前的最后一个 @ 才是 userinfo 分隔
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return dsn
	}
	userinfo := rest[:at]
	if colon := strings.Index(userinfo, ":"); colon >= 0 {
		userinfo = userinfo[:colon] + ":*****"
	}
	return dsn[:i] + userinfo + rest[at:]
}

func runDBStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dsn := getDSN()
	if err := db.ConnectPostgres(ctx, dsn, 2, 1); err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}
	defer db.ClosePostgres()

	if err := db.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("数据库不可达: %w", err)
	}

	fmt.Println("Database Status")
	fmt.Println("===============")
	fmt.Printf("DSN:       %s\n", sanitizeDSN(dsn))
	fmt.Printf("Connected: yes\n")

	// 查询已应用的迁移（表可能尚未创建 → 视为无迁移）
	rows, err := db.Pool.Query(ctx,
		"SELECT version, name, checksum, applied_at FROM schema_migrations ORDER BY version DESC")
	if err != nil {
		fmt.Println("Migrations: (schema_migrations 表不存在或不可读，可能尚未执行过迁移)")
		return nil
	}
	defer rows.Close()

	fmt.Println("\nApplied migrations:")
	count := 0
	for rows.Next() {
		var version int64
		var name, checksum string
		var appliedAt time.Time
		if err := rows.Scan(&version, &name, &checksum, &appliedAt); err != nil {
			continue
		}
		fmt.Printf("  %d  %s  %s  %s\n", version, name, appliedAt.Format(time.RFC3339), checksum)
		count++
	}
	if count == 0 {
		fmt.Println("  (none)")
	}
	fmt.Printf("\nTotal: %d migrations applied\n", count)
	return nil
}

func runDBMigrate(cmd *cobra.Command, args []string) error {
	// 用户决策：调用 atlas 二进制执行迁移
	atlasBin, err := exec.LookPath("atlas")
	if err != nil {
		return fmt.Errorf(
			"atlas CLI 未找到，请先安装（https://atlasgo.io）或使用 minicc-migrate 命令。"+
				"安装后可执行: minicc db migrate。atlas 安装参考: https://atlasgo.io/getting-started/installation")
	}

	dsn := getDSN()
	fmt.Printf("Running database migrations (atlas: %s)\n", atlasBin)
	fmt.Printf("DSN: %s\n", sanitizeDSN(dsn))

	// 检测迁移目录是否混有内部迁移格式（.up.sql/.down.sql），atlas 无法正确处理
	if hasInternalMigrationFiles("migrations") {
		fmt.Println("警告: migrations/ 目录包含 .up.sql/.down.sql（内部迁移器格式），" +
			"atlas 可能无法正确应用全部迁移；建议使用 minicc-migrate 命令。")
	}

	// atlas SQL 格式迁移：--dir 指向 migrations/，--url 连接数据库（5 分钟超时）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	migrateCmd := exec.CommandContext(
		ctx, atlasBin, "migrate", "apply",
		"--dir", "file://migrations",
		"--url", dsn,
	)
	migrateCmd.Stdout = os.Stdout
	migrateCmd.Stderr = os.Stderr
	if err := migrateCmd.Run(); err != nil {
		return fmt.Errorf("atlas migrate apply 失败: %w", err)
	}
	fmt.Println("Database migrations completed")
	return nil
}

// hasInternalMigrationFiles 检测目录下是否存在内部迁移器格式（.up.sql/.down.sql）文件
func hasInternalMigrationFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".up.sql") || strings.HasSuffix(name, ".down.sql") {
			return true
		}
	}
	return false
}
