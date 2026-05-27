package db

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations 从内嵌的 migrations 目录读取 *.up.sql 文件，按文件名排序后顺序执行。
//
// 迁移追踪表 schema_migrations 记录已应用的迁移版本号，已应用的不会重复执行。
// 使用 sqlx 直连执行 SQL，不依赖 golang-migrate 等外部迁移框架，
// 避免引入 mattn/go-sqlite3（CGO）依赖。
//
// dbPath: SQLite 文件路径，会调用 Connect 打开（自动创建数据库文件）。
// 返回第一个执行失败的迁移的错误，如果全部成功或没有新迁移则返回 nil。
func RunMigrations(dbPath string) error {
	// 打开数据库连接（如果文件不存在会自动创建）
	conn, err := Connect(dbPath)
	if err != nil {
		return fmt.Errorf("migrate connect: %w", err)
	}
	defer conn.Close()

	// 创建迁移追踪表（如果不存在）
	conn.MustExec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		dirty INTEGER NOT NULL DEFAULT 0
	)`)

	// 查询已应用的迁移版本号集合
	applied := appliedVersions(conn)

	// 从嵌入文件系统读取 migrations 目录中的 .up.sql 文件
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// 筛选出 .up.sql 文件，按文件名排序（文件名格式：001_xxx.up.sql）
	var upFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	// 按顺序执行未应用的迁移
	for _, name := range upFiles {
		ver := parseVersion(name)
		if ver == 0 || applied[ver] {
			continue // 版本号无效或已应用，跳过
		}
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := conn.Exec(string(data)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		// 记录该迁移已应用
		conn.MustExec("INSERT INTO schema_migrations (version) VALUES (?)", ver)
	}
	return nil
}

// appliedVersions 返回 schema_migrations 表中已记录的迁移版本号集合。
// 如果表不存在（首次运行），返回空 map。
func appliedVersions(db *sqlx.DB) map[int]bool {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return map[int]bool{}
	}
	defer rows.Close()
	m := map[int]bool{}
	for rows.Next() {
		var v int
		rows.Scan(&v)
		m[v] = true
	}
	return m
}

// parseVersion 从迁移文件名开头提取数字版本号。
// 文件名格式约定为 "001_description.up.sql"，其中 "001" 就是版本号。
func parseVersion(filename string) int {
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) == 0 {
		return 0
	}
	var v int
	fmt.Sscanf(parts[0], "%d", &v)
	return v
}
