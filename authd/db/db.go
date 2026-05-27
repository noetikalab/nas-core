// Package db 提供 SQLite 数据库连接与嵌入式迁移功能。
// 使用纯 Go 的 modernc.org/sqlite 驱动，无需 CGO，编译和部署更简单。
package db

import (
	"github.com/jmoiron/sqlx"
	// 注册 "sqlite" 驱动到 database/sql，由 modernc.org/sqlite（纯 Go）实现。
	// 不要替换为 mattn/go-sqlite3，那个需要 CGO。
	_ "modernc.org/sqlite"
)

// Connect 打开（或新建）指定路径的 SQLite 数据库，Ping 验证连接。
//
// DSN 参数说明：
//   - _journal_mode=WAL：Write-Ahead Logging，读写并发更好
//   - _busy_timeout=5000：遇到锁等待 5 秒而非立即返回 busy 错误
//
// SQLite 只支持单写者，因此 SetMaxOpenConns(1) 避免并发写冲突。
// 调用方负责在使用完毕后 db.Close()。
func Connect(path string) (*sqlx.DB, error) {
	db, err := sqlx.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	// SQLite 写操作是串行的，限制连接池大小为 1 可避免 "database is locked"
	db.SetMaxOpenConns(1)
	return db, nil
}
