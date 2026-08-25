package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/athenavi/chiron/internal/db"
	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database management",
	Long:  `Manage Chiron database.`,
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

// getDSN 璇诲彇 POSTGRES_DSN锛岄粯璁や笌 config 涓€鑷?func getDSN() string {
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		return dsn
	}
	return "postgres://chiron:chiron@localhost:5432/chiron?sslmode=disable"
}

// sanitizeDSN 闅愯棌杩炴帴涓蹭腑鐨勫瘑鐮侊紝閬垮厤鎵撳嵃娉勯湶
func sanitizeDSN(dsn string) string {
	const marker = "://"
	i := 0
	if idx := strings.Index(dsn, marker); idx >= 0 {
		i = idx + len(marker)
	}
	rest := dsn[i:]
	// 瀵嗙爜鍙兘鍚?@锛宧ost 鍓嶇殑鏈€鍚庝竴涓?@ 鎵嶆槸 userinfo 鍒嗛殧
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
		return fmt.Errorf("杩炴帴鏁版嵁搴撳け璐? %w", err)
	}
	defer db.ClosePostgres()

	if err := db.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("鏁版嵁搴撲笉鍙揪: %w", err)
	}

	fmt.Println("Database Status")
	fmt.Println("===============")
	fmt.Printf("DSN:       %s\n", sanitizeDSN(dsn))
	fmt.Printf("Connected: yes\n")

	// 鏌ヨ宸插簲鐢ㄧ殑杩佺Щ锛堣〃鍙兘灏氭湭鍒涘缓 鈫?瑙嗕负鏃犺縼绉伙級
	rows, err := db.Pool.Query(ctx,
		"SELECT version, name, checksum, applied_at FROM schema_migrations ORDER BY version DESC")
	if err != nil {
		fmt.Println("Migrations: (schema_migrations 琛ㄤ笉瀛樺湪鎴栦笉鍙锛屽彲鑳藉皻鏈墽琛岃繃杩佺Щ)")
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
	// 鐢ㄦ埛鍐崇瓥锛氳皟鐢?atlas 浜岃繘鍒舵墽琛岃縼绉?	atlasBin, err := exec.LookPath("atlas")
	if err != nil {
		return fmt.Errorf(
			"atlas CLI 鏈壘鍒帮紝璇峰厛瀹夎锛坔ttps://atlasgo.io锛夋垨浣跨敤 chiron-migrate 鍛戒护銆?+
				"瀹夎鍚庡彲鎵ц: chiron db migrate銆俛tlas 瀹夎鍙傝€? https://atlasgo.io/getting-started/installation")

	}

	dsn := getDSN()
	fmt.Printf("Running database migrations (atlas: %s)\n", atlasBin)
	fmt.Printf("DSN: %s\n", sanitizeDSN(dsn))

	// 妫€娴嬭縼绉荤洰褰曟槸鍚︽贩鏈夊唴閮ㄨ縼绉绘牸寮忥紙.up.sql/.down.sql锛夛紝atlas 鏃犳硶姝ｇ‘澶勭悊
	if hasInternalMigrationFiles("migrations") {
		fmt.Println("璀﹀憡: migrations/ 鐩綍鍖呭惈 .up.sql/.down.sql锛堝唴閮ㄨ縼绉诲櫒鏍煎紡锛夛紝" +
			"atlas 鍙兘鏃犳硶姝ｇ‘搴旂敤鍏ㄩ儴杩佺Щ锛涘缓璁娇鐢?chiron-migrate 鍛戒护銆?)
	}

	// atlas SQL 鏍煎紡杩佺Щ锛?-dir 鎸囧悜 migrations/锛?-url 杩炴帴鏁版嵁搴擄紙5 鍒嗛挓瓒呮椂锛?	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	migrateCmd := exec.CommandContext(
		ctx, atlasBin, "migrate", "apply",
		"--dir", "file://migrations",
		"--url", dsn,
	)
	migrateCmd.Stdout = os.Stdout
	migrateCmd.Stderr = os.Stderr
	if err := migrateCmd.Run(); err != nil {
		return fmt.Errorf("atlas migrate apply 澶辫触: %w", err)
	}
	fmt.Println("Database migrations completed")
	return nil
}

// hasInternalMigrationFiles 妫€娴嬬洰褰曚笅鏄惁瀛樺湪鍐呴儴杩佺Щ鍣ㄦ牸寮忥紙.up.sql/.down.sql锛夋枃浠?func hasInternalMigrationFiles(dir string) bool {
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


