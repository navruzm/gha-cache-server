package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"

	"github.com/navruzm/github-actions-cache-server-go/internal/config"
)

type Driver string

const (
	SQLite   Driver = "sqlite"
	Postgres Driver = "postgres"
	MySQL    Driver = "mysql"
)

type DB struct {
	*sql.DB
	Driver Driver
}

func Open(cfg *config.Config) (*DB, error) {
	switch cfg.DBDriver {
	case "sqlite":
		if dir := filepath.Dir(cfg.DBSQLitePath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create sqlite dir: %w", err)
			}
		}
		db, err := sql.Open("sqlite", cfg.DBSQLitePath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1)
		if err := db.Ping(); err != nil {
			return nil, err
		}
		return &DB{DB: db, Driver: SQLite}, nil
	case "postgres":
		dsn := cfg.PostgresURL
		if dsn == "" {
			u := url.URL{
				Scheme: "postgres",
				User:   url.UserPassword(cfg.PostgresUser, cfg.PostgresPassword),
				Host:   fmt.Sprintf("%s:%d", cfg.PostgresHost, cfg.PostgresPort),
				Path:   "/" + cfg.PostgresDatabase,
			}
			q := u.Query()
			q.Set("sslmode", "disable")
			u.RawQuery = q.Encode()
			dsn = u.String()
		}
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(10)
		if err := db.Ping(); err != nil {
			return nil, err
		}
		return &DB{DB: db, Driver: Postgres}, nil
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true",
			cfg.MySQLUser, cfg.MySQLPassword, cfg.MySQLHost, cfg.MySQLPort, cfg.MySQLDatabase)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(10)
		if err := db.Ping(); err != nil {
			return nil, err
		}
		return &DB{DB: db, Driver: MySQL}, nil
	default:
		return nil, fmt.Errorf("unknown driver %q", cfg.DBDriver)
	}
}
