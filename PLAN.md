# GitHub Actions Cache Server — Go Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the falcondev-oss/github-actions-cache-server (TypeScript/Nitro) to Go, producing a single static binary that is wire-compatible with `actions/cache@v4` (Twirp v2 + Azure-style block-blob upload) and feature-equivalent for filesystem/S3/GCS storage and SQLite/Postgres/MySQL databases.

**Architecture:** Single Go process exposing an HTTP server on `:3000` with three logical surfaces: (1) the GitHub cache Twirp v2 protocol at `/twirp/github.actions.results.api.v1.CacheService/*`, (2) the Azure block-blob emulation at `/devstoreaccount1/upload/{id}` plus a direct streaming download at `/download/{id}`, (3) a REST management API at `/management-api/*`. Storage is abstracted behind a small `Adapter` interface (filesystem, S3, GCS); persistence is `database/sql` against SQLite/Postgres/MySQL. Background cleanup runs on `time.Ticker`s.

**Tech Stack:**
- **Language:** Go 1.22+ (uses `net/http` ServeMux pattern routing)
- **HTTP:** `net/http` standard library (no framework)
- **DB:** `database/sql` + `modernc.org/sqlite` (pure-Go), `github.com/lib/pq`, `github.com/go-sql-driver/mysql`
- **JWT/JWKS:** hand-rolled from `crypto/rsa`, `crypto/ecdsa`, `crypto/x509`, `encoding/json` (RS256 + ES256)
- **Cloud storage:** stdlib `net/http` + `crypto/hmac` SigV4 for S3; stdlib + RSA-signed JWT bearer for GCS
- **Logging:** `log/slog`
- **Scheduling:** `time.Ticker` (cron patterns are fixed: every 5min, hourly, daily — no general cron parser needed)
- **Testing:** stdlib `testing` + `net/http/httptest`; integration via `testcontainers-go` (only third-party test dep)

**Reference repo:** `https://github.com/falcondev-oss/github-actions-cache-server` (commit at the time of writing produces ~2,765 lines of TypeScript across `lib/`, `routes/`, `tasks/`, `plugins/`).

---

## Working Conventions

- **TDD throughout.** Every task starts with a failing test, then implementation, then a passing run, then commit.
- **Frequent commits.** One commit per task. Conventional Commits: `feat:`, `fix:`, `refactor:`, `test:`, `chore:`, `docs:`.
- **DRY.** Common helpers live in one place and are imported.
- **YAGNI.** Don't build cron-pattern parsers when the only patterns we need are `*/5 * * * *`, `0 * * * *`, `0 0 * * *`.
- **stdlib first.** A third-party dep is only added when stdlib is genuinely insufficient (DB drivers, testcontainers).
- **Testing strategy.** Unit tests use the in-memory filesystem adapter and `:memory:` SQLite. Integration/e2e tests use real `actions/cache` semantics with httptest servers.
- **Run all tests after every task** with `go test ./...` from the repo root, and verify they pass before committing.
- **No comments unless WHY is non-obvious.** Code blocks below follow this rule — do not embellish them.

---

## File Structure

```
github-actions-cache-server/
├── PLAN.md
├── README.md
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── go.sum
├── cmd/
│   └── cache-server/
│       └── main.go
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── logging/
│   │   └── logging.go
│   ├── ids/
│   │   ├── ids.go
│   │   └── ids_test.go
│   ├── db/
│   │   ├── db.go
│   │   ├── models.go
│   │   ├── queries.go
│   │   ├── queries_test.go
│   │   ├── migrations.go
│   │   └── migrations_test.go
│   ├── storage/
│   │   ├── adapter.go
│   │   ├── filesystem.go
│   │   ├── filesystem_test.go
│   │   ├── s3.go
│   │   ├── s3_test.go
│   │   ├── gcs.go
│   │   ├── gcs_test.go
│   │   ├── sigv4.go
│   │   ├── sigv4_test.go
│   │   └── service.go              # Storage service: upload/download/match
│   ├── auth/
│   │   ├── jwks.go
│   │   ├── jwks_test.go
│   │   ├── jwt.go
│   │   ├── jwt_test.go
│   │   ├── scope.go
│   │   └── scope_test.go
│   ├── server/
│   │   ├── server.go
│   │   ├── middleware.go
│   │   ├── health.go
│   │   ├── twirp.go
│   │   ├── twirp_test.go
│   │   ├── upload.go
│   │   ├── upload_test.go
│   │   ├── download.go
│   │   ├── download_test.go
│   │   ├── management.go
│   │   ├── management_test.go
│   │   └── proxy.go
│   ├── cron/
│   │   ├── cron.go
│   │   └── cron_test.go
│   └── tasks/
│       ├── tasks.go
│       ├── cleanup_uploads.go
│       ├── cleanup_uploads_test.go
│       ├── cleanup_cache_entries.go
│       ├── cleanup_storage_locations.go
│       ├── cleanup_parts.go
│       └── cleanup_merges.go
└── tests/
    └── e2e/
        └── e2e_test.go
```

**Responsibilities:**
- `cmd/cache-server`: composition root only — parses config, wires everything up, runs `http.Server`.
- `internal/config`: env parsing & validation; one struct, one `Load()` function.
- `internal/logging`: slog handler factory.
- `internal/ids`: UUID v4 + numeric ID generation (10-digit numeric for upload IDs).
- `internal/db`: `*sql.DB` open + ping, models, all queries (no ORM), migrations.
- `internal/storage`: `Adapter` interface + 3 implementations + `Service` (upload/download/match logic).
- `internal/auth`: JWKS cache, RS256/ES256 verifier, `getCacheScope` equivalent.
- `internal/server`: HTTP handlers, middleware, ServeMux wiring.
- `internal/cron`: minimal scheduler (specific patterns only).
- `internal/tasks`: cleanup task functions registered with cron.

---

## Phase 0 — Repository Bootstrap

### Task 0.1: Initialise Go module and skeleton

**Files:**
- Create: `go.mod`
- Create: `cmd/cache-server/main.go`
- Create: `.gitignore`

- [ ] **Step 1: Initialise the module**

Run:
```bash
cd /home/mustafa/Projects/Personal/github-actions-cache-server
go mod init github.com/falcondev-oss/github-actions-cache-server-go
git init
```

- [ ] **Step 2: Write `.gitignore`**

```
/cache-server
/cache-server.exe
.data/
*.test
*.out
coverage.txt
.idea/
.vscode/
tests/temp/
```

- [ ] **Step 3: Write a placeholder `cmd/cache-server/main.go`**

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "cache-server: not implemented yet")
	os.Exit(1)
}
```

- [ ] **Step 4: Verify it builds**

Run: `go build ./...`
Expected: exits 0, produces no output.

- [ ] **Step 5: Commit**

```bash
git add go.mod cmd .gitignore
git commit -m "chore: bootstrap Go module skeleton"
```

---

## Phase 1 — Configuration

The TS version uses `arkenv` to validate a discriminated union of env vars. We replicate that with a plain Go struct + a `Load()` function that returns typed errors when required vars are missing or invalid.

### Task 1.1: Config struct and loader

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("API_BASE_URL", "http://localhost:3000")
	cfg, err := Load(envFunc(map[string]string{
		"API_BASE_URL": "http://localhost:3000",
	}))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.StorageDriver != "filesystem" {
		t.Errorf("default storage driver = %q, want filesystem", cfg.StorageDriver)
	}
	if cfg.DBDriver != "sqlite" {
		t.Errorf("default DB driver = %q, want sqlite", cfg.DBDriver)
	}
	if cfg.StorageFilesystemPath != ".data/storage/filesystem" {
		t.Errorf("default fs path = %q", cfg.StorageFilesystemPath)
	}
	if cfg.DBSQLitePath != ".data/sqlite.db" {
		t.Errorf("default sqlite path = %q", cfg.DBSQLitePath)
	}
	if cfg.CacheCleanupOlderThanDays != 90 {
		t.Errorf("default cleanup days = %d", cfg.CacheCleanupOlderThanDays)
	}
	if cfg.DefaultActionsResultsURL == "" {
		t.Error("expected DefaultActionsResultsURL default")
	}
}

func TestLoad_RequiresAPIBaseURL(t *testing.T) {
	_, err := Load(envFunc(map[string]string{}))
	if err == nil {
		t.Fatal("expected error when API_BASE_URL missing")
	}
}

func TestLoad_S3Variant(t *testing.T) {
	cfg, err := Load(envFunc(map[string]string{
		"API_BASE_URL":      "http://localhost:3000",
		"STORAGE_DRIVER":    "s3",
		"STORAGE_S3_BUCKET": "test-bucket",
		"AWS_REGION":        "eu-west-1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StorageDriver != "s3" || cfg.S3Bucket != "test-bucket" || cfg.AWSRegion != "eu-west-1" {
		t.Errorf("got %+v", cfg)
	}
}

func TestLoad_PostgresFromURL(t *testing.T) {
	cfg, err := Load(envFunc(map[string]string{
		"API_BASE_URL":     "http://localhost:3000",
		"DB_DRIVER":        "postgres",
		"DB_POSTGRES_URL":  "postgres://u:p@h:5432/db",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBDriver != "postgres" || cfg.PostgresURL != "postgres://u:p@h:5432/db" {
		t.Errorf("got %+v", cfg)
	}
}

func envFunc(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}
```

- [ ] **Step 2: Run the test (expect failure)**

Run: `go test ./internal/config/...`
Expected: build error — `Load` undefined.

- [ ] **Step 3: Implement `Load`**

Write `internal/config/config.go`:

```go
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
)

type Config struct {
	APIBaseURL                string
	DefaultActionsResultsURL  string
	CacheCleanupOlderThanDays int
	DisableCleanupJobs        bool
	Debug                     bool
	EnableDirectDownloads     bool
	SkipTokenValidation       bool
	ManagementAPIKey          string
	ListenAddr                string

	StorageDriver string

	StorageFilesystemPath string

	S3Bucket           string
	AWSRegion          string
	AWSEndpointURL     string
	AWSAccessKeyID     string
	AWSSecretAccessKey string

	GCSBucket               string
	GCSServiceAccountKey    string
	GCSEndpoint             string

	DBDriver string

	DBSQLitePath string

	PostgresDatabase string
	PostgresHost     string
	PostgresPort     int
	PostgresUser     string
	PostgresPassword string
	PostgresURL      string

	MySQLDatabase string
	MySQLHost     string
	MySQLPort     int
	MySQLUser     string
	MySQLPassword string
}

type EnvFunc func(string) string

func Load(env EnvFunc) (*Config, error) {
	if env == nil {
		env = os.Getenv
	}
	c := &Config{
		ListenAddr:                ":3000",
		DefaultActionsResultsURL:  "https://results-receiver.actions.githubusercontent.com",
		CacheCleanupOlderThanDays: 90,
		StorageDriver:             "filesystem",
		StorageFilesystemPath:     ".data/storage/filesystem",
		DBDriver:                  "sqlite",
		DBSQLitePath:              ".data/sqlite.db",
		AWSRegion:                 "us-east-1",
	}

	c.APIBaseURL = env("API_BASE_URL")
	if c.APIBaseURL == "" {
		return nil, errors.New("API_BASE_URL is required")
	}
	if _, err := url.Parse(c.APIBaseURL); err != nil {
		return nil, fmt.Errorf("API_BASE_URL invalid: %w", err)
	}

	if v := env("DEFAULT_ACTIONS_RESULTS_URL"); v != "" {
		c.DefaultActionsResultsURL = v
	}
	if v := env("CACHE_CLEANUP_OLDER_THAN_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("CACHE_CLEANUP_OLDER_THAN_DAYS invalid: %q", v)
		}
		c.CacheCleanupOlderThanDays = n
	}
	c.DisableCleanupJobs = parseBool(env("DISABLE_CLEANUP_JOBS"))
	c.Debug = parseBool(env("DEBUG"))
	c.EnableDirectDownloads = parseBool(env("ENABLE_DIRECT_DOWNLOADS"))
	c.SkipTokenValidation = parseBool(env("SKIP_TOKEN_VALIDATION"))
	c.ManagementAPIKey = env("MANAGEMENT_API_KEY")
	if v := env("LISTEN_ADDR"); v != "" {
		c.ListenAddr = v
	}

	if v := env("STORAGE_DRIVER"); v != "" {
		c.StorageDriver = v
	}
	switch c.StorageDriver {
	case "filesystem":
		if v := env("STORAGE_FILESYSTEM_PATH"); v != "" {
			c.StorageFilesystemPath = v
		}
	case "s3":
		c.S3Bucket = env("STORAGE_S3_BUCKET")
		if c.S3Bucket == "" {
			return nil, errors.New("STORAGE_S3_BUCKET is required when STORAGE_DRIVER=s3")
		}
		if v := env("AWS_REGION"); v != "" {
			c.AWSRegion = v
		}
		c.AWSEndpointURL = env("AWS_ENDPOINT_URL")
		c.AWSAccessKeyID = env("AWS_ACCESS_KEY_ID")
		c.AWSSecretAccessKey = env("AWS_SECRET_ACCESS_KEY")
	case "gcs":
		c.GCSBucket = env("STORAGE_GCS_BUCKET")
		if c.GCSBucket == "" {
			return nil, errors.New("STORAGE_GCS_BUCKET is required when STORAGE_DRIVER=gcs")
		}
		c.GCSServiceAccountKey = env("STORAGE_GCS_SERVICE_ACCOUNT_KEY")
		c.GCSEndpoint = env("STORAGE_GCS_ENDPOINT")
	default:
		return nil, fmt.Errorf("STORAGE_DRIVER must be filesystem|s3|gcs, got %q", c.StorageDriver)
	}

	if v := env("DB_DRIVER"); v != "" {
		c.DBDriver = v
	}
	switch c.DBDriver {
	case "sqlite":
		if v := env("DB_SQLITE_PATH"); v != "" {
			c.DBSQLitePath = v
		}
	case "postgres":
		c.PostgresURL = env("DB_POSTGRES_URL")
		if c.PostgresURL == "" {
			c.PostgresDatabase = env("DB_POSTGRES_DATABASE")
			c.PostgresHost = env("DB_POSTGRES_HOST")
			c.PostgresUser = env("DB_POSTGRES_USER")
			c.PostgresPassword = env("DB_POSTGRES_PASSWORD")
			port, err := strconv.Atoi(env("DB_POSTGRES_PORT"))
			if err != nil {
				return nil, fmt.Errorf("DB_POSTGRES_PORT invalid: %q", env("DB_POSTGRES_PORT"))
			}
			c.PostgresPort = port
			if c.PostgresDatabase == "" || c.PostgresHost == "" || c.PostgresUser == "" {
				return nil, errors.New("postgres requires DB_POSTGRES_URL or DATABASE/HOST/PORT/USER/PASSWORD")
			}
		}
	case "mysql":
		c.MySQLDatabase = env("DB_MYSQL_DATABASE")
		c.MySQLHost = env("DB_MYSQL_HOST")
		c.MySQLUser = env("DB_MYSQL_USER")
		c.MySQLPassword = env("DB_MYSQL_PASSWORD")
		port, err := strconv.Atoi(env("DB_MYSQL_PORT"))
		if err != nil {
			return nil, fmt.Errorf("DB_MYSQL_PORT invalid: %q", env("DB_MYSQL_PORT"))
		}
		c.MySQLPort = port
		if c.MySQLDatabase == "" || c.MySQLHost == "" || c.MySQLUser == "" {
			return nil, errors.New("mysql requires DB_MYSQL_DATABASE/HOST/PORT/USER/PASSWORD")
		}
	default:
		return nil, fmt.Errorf("DB_DRIVER must be sqlite|postgres|mysql, got %q", c.DBDriver)
	}

	return c, nil
}

func parseBool(v string) bool {
	switch v {
	case "1", "true", "TRUE", "True", "yes", "on":
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests (expect pass)**

Run: `go test ./internal/config/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat(config): typed env loader with discriminated storage/db drivers"
```

---

## Phase 2 — Logging

The TS uses `consola`. We use `log/slog`. One file, no tests (delegated to stdlib).

### Task 2.1: Logging factory

**Files:**
- Create: `internal/logging/logging.go`

- [ ] **Step 1: Write the file**

```go
package logging

import (
	"log/slog"
	"os"
)

func New(debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(h)
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./internal/logging/...`
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add internal/logging
git commit -m "feat(logging): slog factory with debug toggle"
```

---

## Phase 3 — IDs

Two ID types: UUID v4 (for cache_entries / storage_locations IDs), and 10-digit numeric (for upload IDs, matching the TS `nanoid('0123456789', 10)`).

### Task 3.1: ID generators

**Files:**
- Create: `internal/ids/ids.go`
- Test: `internal/ids/ids_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ids

import (
	"regexp"
	"testing"
)

func TestUUIDv4_Format(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for i := 0; i < 100; i++ {
		got := UUIDv4()
		if !re.MatchString(got) {
			t.Fatalf("UUIDv4 not v4-shaped: %q", got)
		}
	}
}

func TestUUIDv4_Unique(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 1000; i++ {
		u := UUIDv4()
		if _, dup := seen[u]; dup {
			t.Fatalf("duplicate uuid: %s", u)
		}
		seen[u] = struct{}{}
	}
}

func TestNumericID(t *testing.T) {
	re := regexp.MustCompile(`^[1-9][0-9]{9}$`)
	for i := 0; i < 100; i++ {
		got := NumericID()
		if !re.MatchString(formatInt64(got)) {
			t.Fatalf("NumericID not 10-digit: %d", got)
		}
	}
}

func formatInt64(n int64) string {
	if n < 0 {
		return "-"
	}
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{digits[n%10]}, buf...)
		n /= 10
	}
	return string(buf)
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/ids/...`
Expected: build error.

- [ ] **Step 3: Implement**

```go
package ids

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

func UUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	dst := make([]byte, 36)
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst)
}

func NumericID() int64 {
	const min = int64(1_000_000_000)
	const span = int64(9_000_000_000)
	n, err := rand.Int(rand.Reader, big.NewInt(span))
	if err != nil {
		panic(err)
	}
	return min + n.Int64()
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ids/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ids
git commit -m "feat(ids): UUIDv4 and 10-digit numeric IDs from crypto/rand"
```

---

## Phase 4 — Database Layer

### Task 4.1: Models and `*sql.DB` open

**Files:**
- Create: `internal/db/models.go`
- Create: `internal/db/db.go`

- [ ] **Step 1: Write `internal/db/models.go`**

```go
package db

type CacheEntry struct {
	ID         string
	Key        string
	Version    string
	Scope      string
	RepoID     string
	UpdatedAt  int64
	LocationID string
}

type StorageLocation struct {
	ID               string
	FolderName       string
	PartCount        int
	MergeStartedAt   *int64
	MergedAt         *int64
	PartsDeletedAt   *int64
	LastDownloadedAt *int64
}

type Upload struct {
	ID                      int64
	Key                     string
	Version                 string
	Scope                   string
	RepoID                  string
	CreatedAt               int64
	LastPartUploadedAt      *int64
	StartedPartUploadCount  int
	FinishedPartUploadCount int
	FolderName              string
}
```

- [ ] **Step 2: Write `internal/db/db.go`**

```go
package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
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
```

- [ ] **Step 3: Add SQLite driver to go.mod**

Run:
```bash
go get modernc.org/sqlite
go get github.com/lib/pq
go get github.com/go-sql-driver/mysql
```

- [ ] **Step 4: Add blank imports**

Append to top of `internal/db/db.go` import block:

```go
import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
)
```

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add internal/db go.mod go.sum
git commit -m "feat(db): models and *sql.DB factory for sqlite/postgres/mysql"
```

---

### Task 4.2: Migrations

The TS has 4 sequential migrations. We replicate them as ordered Go SQL. SQLite/Postgres use `text` for IDs; MySQL uses `varchar(36)` for IDs and varchar for keys.

**Files:**
- Create: `internal/db/migrations.go`
- Test: `internal/db/migrations_test.go`

- [ ] **Step 1: Write the failing test**

```go
package db

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openSQLite(t *testing.T) *DB {
	t.Helper()
	raw, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	raw.SetMaxOpenConns(1)
	return &DB{DB: raw, Driver: SQLite}
}

func TestMigrate_AppliesAndIsIdempotent(t *testing.T) {
	d := openSQLite(t)
	ctx := context.Background()
	if err := Migrate(ctx, d); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(ctx, d); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var n int
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM cache_entries`).Scan(&n); err != nil {
		t.Fatalf("count cache_entries: %v", err)
	}
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM storage_locations`).Scan(&n); err != nil {
		t.Fatalf("count storage_locations: %v", err)
	}
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM uploads`).Scan(&n); err != nil {
		t.Fatalf("count uploads: %v", err)
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/db/...`
Expected: build error — `Migrate` undefined.

- [ ] **Step 3: Implement migrations**

Write `internal/db/migrations.go`:

```go
package db

import (
	"context"
	"errors"
	"fmt"
)

type migration struct {
	name string
	fn   func(ctx context.Context, d *DB) error
}

var migrations = []migration{
	{name: "0_init", fn: m0Init},
	{name: "1_upload_part_counts", fn: m1UploadPartCounts},
	{name: "2_scopes", fn: m2Scopes},
	{name: "3_repoId", fn: m3RepoID},
}

func Migrate(ctx context.Context, d *DB) error {
	if err := ensureMigrationsTable(ctx, d); err != nil {
		return err
	}
	applied, err := loadApplied(ctx, d)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if applied[m.name] {
			continue
		}
		if err := m.fn(ctx, d); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if _, err := d.ExecContext(ctx, `INSERT INTO _migrations(name) VALUES (`+placeholder(d.Driver, 1)+`)`, m.name); err != nil {
			return fmt.Errorf("record migration %s: %w", m.name, err)
		}
	}
	return nil
}

func ensureMigrationsTable(ctx context.Context, d *DB) error {
	stmt := `CREATE TABLE IF NOT EXISTS _migrations (name TEXT PRIMARY KEY)`
	if d.Driver == MySQL {
		stmt = `CREATE TABLE IF NOT EXISTS _migrations (name VARCHAR(64) PRIMARY KEY)`
	}
	_, err := d.ExecContext(ctx, stmt)
	return err
}

func loadApplied(ctx context.Context, d *DB) (map[string]bool, error) {
	rows, err := d.QueryContext(ctx, `SELECT name FROM _migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = true
	}
	return out, rows.Err()
}

func idType(d *DB) string {
	if d.Driver == MySQL {
		return "VARCHAR(36)"
	}
	return "TEXT"
}

func textType(d *DB, max int) string {
	if d.Driver == MySQL {
		return fmt.Sprintf("VARCHAR(%d)", max)
	}
	return "TEXT"
}

func placeholder(drv Driver, n int) string {
	if drv == Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

func m0Init(ctx context.Context, d *DB) error {
	stmts := []string{
		fmt.Sprintf(`CREATE TABLE storage_locations (
			id %s PRIMARY KEY,
			folderName TEXT NOT NULL,
			partCount INTEGER NOT NULL,
			mergeStartedAt BIGINT,
			mergedAt BIGINT,
			partsDeletedAt BIGINT,
			lastDownloadedAt BIGINT
		)`, idType(d)),
		fmt.Sprintf(`CREATE TABLE cache_entries (
			id %s PRIMARY KEY,
			"key" %s NOT NULL,
			version %s NOT NULL,
			updatedAt BIGINT NOT NULL,
			locationId %s NOT NULL REFERENCES storage_locations(id) ON DELETE CASCADE
		)`, idType(d), textType(d, 512), textType(d, 255), idType(d)),
		`CREATE TABLE uploads (
			id BIGINT PRIMARY KEY,
			"key" ` + textType(d, 512) + ` NOT NULL,
			version ` + textType(d, 255) + ` NOT NULL,
			createdAt BIGINT NOT NULL,
			lastPartUploadedAt BIGINT,
			folderName TEXT NOT NULL
		)`,
		`CREATE INDEX idx_cache_entries_key_version ON cache_entries("key", version)`,
		`CREATE INDEX idx_uploads_key_version ON uploads("key", version)`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func m1UploadPartCounts(ctx context.Context, d *DB) error {
	stmts := []string{
		`ALTER TABLE uploads ADD COLUMN finishedPartUploadCount INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE uploads ADD COLUMN startedPartUploadCount INTEGER NOT NULL DEFAULT 0`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func m2Scopes(ctx context.Context, d *DB) error {
	scope := textType(d, 255)
	stmts := []string{
		`DELETE FROM cache_entries`,
		`DELETE FROM storage_locations`,
		`DELETE FROM uploads`,
		`ALTER TABLE cache_entries ADD COLUMN scope ` + scope + ` NOT NULL DEFAULT ''`,
		`CREATE INDEX idx_cache_entries_scope ON cache_entries(scope)`,
		`ALTER TABLE uploads ADD COLUMN scope ` + scope + ` NOT NULL DEFAULT ''`,
		`CREATE INDEX idx_uploads_scope ON uploads(scope)`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func m3RepoID(ctx context.Context, d *DB) error {
	repo := textType(d, 255)
	stmts := []string{
		`DELETE FROM cache_entries`,
		`DELETE FROM storage_locations`,
		`DELETE FROM uploads`,
		`ALTER TABLE cache_entries ADD COLUMN repoId ` + repo + ` NOT NULL DEFAULT ''`,
		`CREATE INDEX idx_cache_entries_repoId ON cache_entries(repoId)`,
		`ALTER TABLE uploads ADD COLUMN repoId ` + repo + ` NOT NULL DEFAULT ''`,
		`CREATE INDEX idx_uploads_repoId ON uploads(repoId)`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

var ErrNoRowsAffected = errors.New("no rows affected")
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/db/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations.go internal/db/migrations_test.go
git commit -m "feat(db): sequential migrations with idempotent applier"
```

---

### Task 4.3: Queries

All DB access lives here. The TS code uses Kysely; we use parameterised SQL. Postgres uses `$1`, others use `?`.

**Files:**
- Create: `internal/db/queries.go`
- Test: `internal/db/queries_test.go`

- [ ] **Step 1: Write the failing test**

```go
package db

import (
	"context"
	"testing"
)

func TestQueries_UploadLifecycle(t *testing.T) {
	d := openSQLite(t)
	ctx := context.Background()
	if err := Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	q := New(d)

	if err := q.InsertUpload(ctx, Upload{
		ID: 1234567890, Key: "k", Version: "v", Scope: "s", RepoID: "r",
		CreatedAt: 1, FolderName: "1234567890",
	}); err != nil {
		t.Fatalf("InsertUpload: %v", err)
	}

	got, err := q.FindUploadByKey(ctx, "k", "v", "s", "r")
	if err != nil || got == nil || got.ID != 1234567890 {
		t.Fatalf("FindUploadByKey: %+v err=%v", got, err)
	}

	if err := q.IncStartedPartCount(ctx, 1234567890); err != nil {
		t.Fatalf("IncStartedPartCount: %v", err)
	}
	if err := q.IncFinishedPartCount(ctx, 1234567890, 42); err != nil {
		t.Fatalf("IncFinishedPartCount: %v", err)
	}

	got, _ = q.FindUploadByID(ctx, 1234567890)
	if got.StartedPartUploadCount != 1 || got.FinishedPartUploadCount != 1 {
		t.Errorf("counts: %+v", got)
	}
}

func TestQueries_CacheEntryMatch(t *testing.T) {
	d := openSQLite(t)
	ctx := context.Background()
	if err := Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	q := New(d)

	if err := q.InsertStorageLocation(ctx, StorageLocation{ID: "loc1", FolderName: "f1", PartCount: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertCacheEntry(ctx, CacheEntry{
		ID: "e1", Key: "deps-abc", Version: "v1", Scope: "main", RepoID: "1", UpdatedAt: 100, LocationID: "loc1",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := q.FindExactCacheEntry(ctx, "deps-abc", "v1", "main", "1")
	if err != nil || got == nil {
		t.Fatalf("FindExactCacheEntry: %+v err=%v", got, err)
	}

	got, err = q.FindPrefixedCacheEntry(ctx, "deps-", "v1", "main", "1")
	if err != nil || got == nil || got.ID != "e1" {
		t.Fatalf("FindPrefixedCacheEntry: %+v err=%v", got, err)
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/db/...`
Expected: build error.

- [ ] **Step 3: Implement queries**

Write `internal/db/queries.go`:

```go
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Queries struct {
	d *DB
}

func New(d *DB) *Queries { return &Queries{d: d} }

func (q *Queries) ph(n int) string { return placeholder(q.d.Driver, n) }

func (q *Queries) phs(n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = q.ph(i + 1)
	}
	return strings.Join(parts, ",")
}

func (q *Queries) InsertUpload(ctx context.Context, u Upload) error {
	stmt := fmt.Sprintf(`INSERT INTO uploads
		(id, "key", version, scope, repoId, createdAt, lastPartUploadedAt, folderName,
		 startedPartUploadCount, finishedPartUploadCount)
		VALUES (%s)`, q.phs(10))
	_, err := q.d.ExecContext(ctx, stmt,
		u.ID, u.Key, u.Version, u.Scope, u.RepoID, u.CreatedAt, u.LastPartUploadedAt, u.FolderName,
		u.StartedPartUploadCount, u.FinishedPartUploadCount)
	return err
}

func (q *Queries) FindUploadByKey(ctx context.Context, key, version, scope, repoID string) (*Upload, error) {
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, createdAt, lastPartUploadedAt,
		startedPartUploadCount, finishedPartUploadCount, folderName
		FROM uploads WHERE "key"=%s AND version=%s AND scope=%s AND repoId=%s`,
		q.ph(1), q.ph(2), q.ph(3), q.ph(4))
	row := q.d.QueryRowContext(ctx, stmt, key, version, scope, repoID)
	return scanUpload(row)
}

func (q *Queries) FindUploadByID(ctx context.Context, id int64) (*Upload, error) {
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, createdAt, lastPartUploadedAt,
		startedPartUploadCount, finishedPartUploadCount, folderName FROM uploads WHERE id=%s`, q.ph(1))
	row := q.d.QueryRowContext(ctx, stmt, id)
	return scanUpload(row)
}

func (q *Queries) IncStartedPartCount(ctx context.Context, id int64) error {
	stmt := fmt.Sprintf(`UPDATE uploads SET startedPartUploadCount = startedPartUploadCount + 1 WHERE id=%s`, q.ph(1))
	_, err := q.d.ExecContext(ctx, stmt, id)
	return err
}

func (q *Queries) IncFinishedPartCount(ctx context.Context, id int64, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE uploads SET finishedPartUploadCount = finishedPartUploadCount + 1, lastPartUploadedAt=%s WHERE id=%s`, q.ph(1), q.ph(2))
	_, err := q.d.ExecContext(ctx, stmt, nowMillis, id)
	return err
}

func (q *Queries) DeleteUpload(ctx context.Context, id int64) error {
	stmt := fmt.Sprintf(`DELETE FROM uploads WHERE id=%s`, q.ph(1))
	_, err := q.d.ExecContext(ctx, stmt, id)
	return err
}

func (q *Queries) InsertStorageLocation(ctx context.Context, l StorageLocation) error {
	stmt := fmt.Sprintf(`INSERT INTO storage_locations
		(id, folderName, partCount, mergeStartedAt, mergedAt, partsDeletedAt, lastDownloadedAt)
		VALUES (%s)`, q.phs(7))
	_, err := q.d.ExecContext(ctx, stmt,
		l.ID, l.FolderName, l.PartCount, l.MergeStartedAt, l.MergedAt, l.PartsDeletedAt, l.LastDownloadedAt)
	return err
}

func (q *Queries) GetStorageLocation(ctx context.Context, id string) (*StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT id, folderName, partCount, mergeStartedAt, mergedAt, partsDeletedAt, lastDownloadedAt
		FROM storage_locations WHERE id=%s`, q.ph(1))
	row := q.d.QueryRowContext(ctx, stmt, id)
	return scanLocation(row)
}

func (q *Queries) DeleteStorageLocation(ctx context.Context, id string) error {
	stmt := fmt.Sprintf(`DELETE FROM storage_locations WHERE id=%s`, q.ph(1))
	_, err := q.d.ExecContext(ctx, stmt, id)
	return err
}

func (q *Queries) UpdateStorageLastDownloaded(ctx context.Context, id string, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET lastDownloadedAt=%s WHERE id=%s`, q.ph(1), q.ph(2))
	_, err := q.d.ExecContext(ctx, stmt, nowMillis, id)
	return err
}

func (q *Queries) UpdateStorageMergeStarted(ctx context.Context, id string, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET mergeStartedAt=%s WHERE id=%s`, q.ph(1), q.ph(2))
	_, err := q.d.ExecContext(ctx, stmt, nowMillis, id)
	return err
}

func (q *Queries) UpdateStorageMerged(ctx context.Context, id string, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET mergedAt=%s WHERE id=%s`, q.ph(1), q.ph(2))
	_, err := q.d.ExecContext(ctx, stmt, nowMillis, id)
	return err
}

func (q *Queries) UpdateStoragePartsDeleted(ctx context.Context, id string, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET partsDeletedAt=%s WHERE id=%s`, q.ph(1), q.ph(2))
	_, err := q.d.ExecContext(ctx, stmt, nowMillis, id)
	return err
}

func (q *Queries) ResetStorageMerge(ctx context.Context, id string) error {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET mergeStartedAt=NULL, mergedAt=NULL WHERE id=%s`, q.ph(1))
	_, err := q.d.ExecContext(ctx, stmt, id)
	return err
}

func (q *Queries) InsertCacheEntry(ctx context.Context, e CacheEntry) error {
	stmt := fmt.Sprintf(`INSERT INTO cache_entries (id, "key", version, scope, repoId, updatedAt, locationId)
		VALUES (%s)`, q.phs(7))
	_, err := q.d.ExecContext(ctx, stmt, e.ID, e.Key, e.Version, e.Scope, e.RepoID, e.UpdatedAt, e.LocationID)
	return err
}

func (q *Queries) UpdateCacheEntryLocation(ctx context.Context, id, locationID string, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE cache_entries SET locationId=%s, updatedAt=%s WHERE id=%s`,
		q.ph(1), q.ph(2), q.ph(3))
	_, err := q.d.ExecContext(ctx, stmt, locationID, nowMillis, id)
	return err
}

func (q *Queries) GetCacheEntry(ctx context.Context, id string) (*CacheEntry, error) {
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, updatedAt, locationId
		FROM cache_entries WHERE id=%s`, q.ph(1))
	row := q.d.QueryRowContext(ctx, stmt, id)
	return scanCacheEntry(row)
}

func (q *Queries) DeleteCacheEntry(ctx context.Context, id string) error {
	stmt := fmt.Sprintf(`DELETE FROM cache_entries WHERE id=%s`, q.ph(1))
	_, err := q.d.ExecContext(ctx, stmt, id)
	return err
}

func (q *Queries) FindExactCacheEntry(ctx context.Context, key, version, scope, repoID string) (*CacheEntry, error) {
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, updatedAt, locationId
		FROM cache_entries
		WHERE "key"=%s AND version=%s AND scope=%s AND repoId=%s
		ORDER BY updatedAt DESC LIMIT 1`,
		q.ph(1), q.ph(2), q.ph(3), q.ph(4))
	row := q.d.QueryRowContext(ctx, stmt, key, version, scope, repoID)
	return scanCacheEntry(row)
}

func (q *Queries) FindPrefixedCacheEntry(ctx context.Context, keyPrefix, version, scope, repoID string) (*CacheEntry, error) {
	pattern := escapeLike(keyPrefix) + "%"
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, updatedAt, locationId
		FROM cache_entries
		WHERE "key" LIKE %s ESCAPE '\' AND version=%s AND scope=%s AND repoId=%s
		ORDER BY updatedAt DESC LIMIT 1`,
		q.ph(1), q.ph(2), q.ph(3), q.ph(4))
	row := q.d.QueryRowContext(ctx, stmt, pattern, version, scope, repoID)
	return scanCacheEntry(row)
}

func (q *Queries) FindCacheEntryWithLocation(ctx context.Context, id string) (*CacheEntry, *StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT c.id, c."key", c.version, c.scope, c.repoId, c.updatedAt, c.locationId,
		s.id, s.folderName, s.partCount, s.mergeStartedAt, s.mergedAt, s.partsDeletedAt, s.lastDownloadedAt
		FROM cache_entries c JOIN storage_locations s ON s.id = c.locationId
		WHERE c.id=%s`, q.ph(1))
	row := q.d.QueryRowContext(ctx, stmt, id)
	var e CacheEntry
	var l StorageLocation
	err := row.Scan(&e.ID, &e.Key, &e.Version, &e.Scope, &e.RepoID, &e.UpdatedAt, &e.LocationID,
		&l.ID, &l.FolderName, &l.PartCount, &l.MergeStartedAt, &l.MergedAt, &l.PartsDeletedAt, &l.LastDownloadedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &e, &l, nil
}

func (q *Queries) FindExistingCacheEntryWithLocation(ctx context.Context, key, version, scope, repoID string) (*CacheEntry, *StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT c.id, c."key", c.version, c.scope, c.repoId, c.updatedAt, c.locationId,
		s.id, s.folderName, s.partCount, s.mergeStartedAt, s.mergedAt, s.partsDeletedAt, s.lastDownloadedAt
		FROM cache_entries c JOIN storage_locations s ON s.id = c.locationId
		WHERE c."key"=%s AND c.version=%s AND c.scope=%s AND c.repoId=%s`,
		q.ph(1), q.ph(2), q.ph(3), q.ph(4))
	row := q.d.QueryRowContext(ctx, stmt, key, version, scope, repoID)
	var e CacheEntry
	var l StorageLocation
	err := row.Scan(&e.ID, &e.Key, &e.Version, &e.Scope, &e.RepoID, &e.UpdatedAt, &e.LocationID,
		&l.ID, &l.FolderName, &l.PartCount, &l.MergeStartedAt, &l.MergedAt, &l.PartsDeletedAt, &l.LastDownloadedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &e, &l, nil
}

func (q *Queries) ListExpiredStorageLocations(ctx context.Context, beforeMillis int64, limit, offset int) ([]StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT id, folderName, partCount, mergeStartedAt, mergedAt, partsDeletedAt, lastDownloadedAt
		FROM storage_locations WHERE lastDownloadedAt < %s
		LIMIT %s OFFSET %s`,
		q.ph(1), q.ph(2), q.ph(3))
	return q.queryLocations(ctx, stmt, beforeMillis, limit, offset)
}

func (q *Queries) ListOrphanedStorageLocations(ctx context.Context, limit, offset int) ([]StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT id, folderName, partCount, mergeStartedAt, mergedAt, partsDeletedAt, lastDownloadedAt
		FROM storage_locations s
		WHERE NOT EXISTS (SELECT 1 FROM cache_entries c WHERE c.locationId = s.id)
		LIMIT %s OFFSET %s`, q.ph(1), q.ph(2))
	return q.queryLocations(ctx, stmt, limit, offset)
}

func (q *Queries) ListMergedNotCleanedStorageLocations(ctx context.Context, limit, offset int) ([]StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT id, folderName, partCount, mergeStartedAt, mergedAt, partsDeletedAt, lastDownloadedAt
		FROM storage_locations WHERE mergedAt IS NOT NULL AND partsDeletedAt IS NULL
		LIMIT %s OFFSET %s`, q.ph(1), q.ph(2))
	return q.queryLocations(ctx, stmt, limit, offset)
}

func (q *Queries) ResetStalledMerges(ctx context.Context, beforeMillis int64) (int64, error) {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET mergeStartedAt=NULL, mergedAt=NULL
		WHERE mergeStartedAt < %s AND mergedAt IS NULL`, q.ph(1))
	res, err := q.d.ExecContext(ctx, stmt, beforeMillis)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (q *Queries) ListStaleUploads(ctx context.Context, beforeMillis int64, limit, offset int) ([]Upload, error) {
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, createdAt, lastPartUploadedAt,
		startedPartUploadCount, finishedPartUploadCount, folderName
		FROM uploads
		WHERE (lastPartUploadedAt IS NULL OR lastPartUploadedAt < %s) AND createdAt < %s
		LIMIT %s OFFSET %s`, q.ph(1), q.ph(2), q.ph(3), q.ph(4))
	rows, err := q.d.QueryContext(ctx, stmt, beforeMillis, beforeMillis, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Upload
	for rows.Next() {
		u, err := scanUploadRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (q *Queries) queryLocations(ctx context.Context, stmt string, args ...any) ([]StorageLocation, error) {
	rows, err := q.d.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StorageLocation
	for rows.Next() {
		var l StorageLocation
		if err := rows.Scan(&l.ID, &l.FolderName, &l.PartCount, &l.MergeStartedAt, &l.MergedAt, &l.PartsDeletedAt, &l.LastDownloadedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

type Tx struct {
	*sql.Tx
	q *Queries
}

func (q *Queries) BeginTx(ctx context.Context) (*Tx, error) {
	t, err := q.d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: t, q: q}, nil
}

func scanUpload(row *sql.Row) (*Upload, error) {
	var u Upload
	err := row.Scan(&u.ID, &u.Key, &u.Version, &u.Scope, &u.RepoID, &u.CreatedAt, &u.LastPartUploadedAt,
		&u.StartedPartUploadCount, &u.FinishedPartUploadCount, &u.FolderName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

func scanUploadRows(rows *sql.Rows) (*Upload, error) {
	var u Upload
	err := rows.Scan(&u.ID, &u.Key, &u.Version, &u.Scope, &u.RepoID, &u.CreatedAt, &u.LastPartUploadedAt,
		&u.StartedPartUploadCount, &u.FinishedPartUploadCount, &u.FolderName)
	return &u, err
}

func scanLocation(row *sql.Row) (*StorageLocation, error) {
	var l StorageLocation
	err := row.Scan(&l.ID, &l.FolderName, &l.PartCount, &l.MergeStartedAt, &l.MergedAt, &l.PartsDeletedAt, &l.LastDownloadedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &l, err
}

func scanCacheEntry(row *sql.Row) (*CacheEntry, error) {
	var e CacheEntry
	err := row.Scan(&e.ID, &e.Key, &e.Version, &e.Scope, &e.RepoID, &e.UpdatedAt, &e.LocationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &e, err
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/db/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries.go internal/db/queries_test.go
git commit -m "feat(db): typed query helpers covering uploads/locations/entries"
```

---

## Phase 5 — Storage Adapter Interface and Filesystem

### Task 5.1: Adapter interface

**Files:**
- Create: `internal/storage/adapter.go`

- [ ] **Step 1: Write the interface**

```go
package storage

import (
	"context"
	"errors"
	"io"
)

var ErrObjectNotFound = errors.New("object not found in storage")

type Adapter interface {
	CreateDownloadStream(ctx context.Context, objectName string) (io.ReadCloser, error)
	UploadStream(ctx context.Context, objectName string, body io.Reader) error
	DeleteFolder(ctx context.Context, folderName string) error
	CountFilesInFolder(ctx context.Context, folderName string) (int, error)
	CreateDownloadURL(ctx context.Context, objectName string) (string, bool, error)
	Clear(ctx context.Context) error
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/storage/...`
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add internal/storage/adapter.go
git commit -m "feat(storage): Adapter interface"
```

---

### Task 5.2: Filesystem adapter

**Files:**
- Create: `internal/storage/filesystem.go`
- Test: `internal/storage/filesystem_test.go`

- [ ] **Step 1: Write the failing test**

```go
package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystem_UploadDownloadDelete(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a, err := NewFilesystemAdapter(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := a.UploadStream(ctx, "1234/parts/0", strings.NewReader("hello")); err != nil {
		t.Fatalf("UploadStream: %v", err)
	}
	r, err := a.CreateDownloadStream(ctx, "1234/parts/0")
	if err != nil {
		t.Fatalf("CreateDownloadStream: %v", err)
	}
	defer r.Close()
	got, _ := io.ReadAll(r)
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}

	n, err := a.CountFilesInFolder(ctx, "1234/parts")
	if err != nil || n != 1 {
		t.Errorf("CountFiles=%d err=%v", n, err)
	}

	if err := a.DeleteFolder(ctx, "1234"); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}
	if _, err := a.CreateDownloadStream(ctx, "1234/parts/0"); !errors.Is(err, ErrObjectNotFound) {
		t.Errorf("expected ErrObjectNotFound, got %v", err)
	}
}

func TestFilesystem_PathTraversalRejected(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a, _ := NewFilesystemAdapter(dir)
	err := a.UploadStream(ctx, "../escape", bytes.NewReader([]byte("x")))
	if err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestFilesystem_Clear(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a, _ := NewFilesystemAdapter(dir)
	_ = a.UploadStream(ctx, "a/b", strings.NewReader("x"))
	_ = a.Clear(ctx)
	entries, _ := os.ReadDir(filepath.Clean(dir))
	if len(entries) != 0 {
		t.Errorf("expected empty after Clear, got %d entries", len(entries))
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/storage/...`
Expected: build error.

- [ ] **Step 3: Implement filesystem adapter**

```go
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type FilesystemAdapter struct {
	root string
}

func NewFilesystemAdapter(root string) (*FilesystemAdapter, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &FilesystemAdapter{root: abs}, nil
}

func (a *FilesystemAdapter) safe(name string) (string, error) {
	cleaned := filepath.Clean(name)
	if cleaned == "." {
		return a.root, nil
	}
	resolved := filepath.Join(a.root, cleaned)
	rel, err := filepath.Rel(a.root, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid object name: %q", name)
	}
	return resolved, nil
}

func (a *FilesystemAdapter) CreateDownloadStream(_ context.Context, objectName string) (io.ReadCloser, error) {
	p, err := a.safe(objectName)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrObjectNotFound
	}
	return f, err
}

func (a *FilesystemAdapter) UploadStream(_ context.Context, objectName string, body io.Reader) error {
	p, err := a.safe(objectName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, body); err != nil {
		return err
	}
	return f.Sync()
}

func (a *FilesystemAdapter) DeleteFolder(_ context.Context, folderName string) error {
	p, err := a.safe(folderName)
	if err != nil {
		return err
	}
	return os.RemoveAll(p)
}

func (a *FilesystemAdapter) CountFilesInFolder(_ context.Context, folderName string) (int, error) {
	p, err := a.safe(folderName)
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(p)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n, nil
}

func (a *FilesystemAdapter) CreateDownloadURL(_ context.Context, _ string) (string, bool, error) {
	return "", false, nil
}

func (a *FilesystemAdapter) Clear(_ context.Context) error {
	if err := os.RemoveAll(a.root); err != nil {
		return err
	}
	return os.MkdirAll(a.root, 0o755)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/storage/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/filesystem.go internal/storage/filesystem_test.go
git commit -m "feat(storage): filesystem adapter with path-traversal guard"
```

---

## Phase 6 — Storage Service (uploads, downloads, match)

This is the heart of the system: it owns the multi-part upload lifecycle, the merge-on-first-download streaming, and the cache-key matching. Logic mirrors `lib/storage.ts` `Storage` class.

### Task 6.1: `Service` skeleton & `CreateUpload`

**Files:**
- Create: `internal/storage/service.go`
- Test: `internal/storage/service_test.go`

- [ ] **Step 1: Write the failing test**

```go
package storage

import (
	"context"
	"testing"

	dbpkg "github.com/falcondev-oss/github-actions-cache-server-go/internal/db"
)

func newServiceForTest(t *testing.T) (*Service, *dbpkg.Queries) {
	t.Helper()
	d := openTestDB(t)
	if err := dbpkg.Migrate(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	q := dbpkg.New(d)
	dir := t.TempDir()
	a, err := NewFilesystemAdapter(dir)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(q, a, ServiceConfig{APIBaseURL: "http://localhost:3000"}), q
}

func TestCreateUpload_NewAndIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	u, err := svc.CreateUpload(ctx, "k", "v", "s", "r")
	if err != nil || u == nil {
		t.Fatalf("CreateUpload: %+v err=%v", u, err)
	}
	dup, err := svc.CreateUpload(ctx, "k", "v", "s", "r")
	if err != nil {
		t.Fatalf("dup CreateUpload: %v", err)
	}
	if dup != nil {
		t.Errorf("expected nil for existing upload, got %+v", dup)
	}
}
```

- [ ] **Step 2: Add a tiny test helper for SQLite** (in same package)

Append to bottom of `service_test.go`:

```go
import (
	"database/sql"
	"testing"

	dbpkg "github.com/falcondev-oss/github-actions-cache-server-go/internal/db"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *dbpkg.DB {
	t.Helper()
	raw, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	raw.SetMaxOpenConns(1)
	return &dbpkg.DB{DB: raw, Driver: dbpkg.SQLite}
}
```

(Single import block — fold it into the file's imports rather than duplicating.)

- [ ] **Step 3: Run test (expect failure)**

Run: `go test ./internal/storage/...`
Expected: build error — `Service` undefined.

- [ ] **Step 4: Implement `Service` constructor + `CreateUpload`**

```go
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"time"

	dbpkg "github.com/falcondev-oss/github-actions-cache-server-go/internal/db"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/ids"
)

type ServiceConfig struct {
	APIBaseURL            string
	EnableDirectDownloads bool
	Logger                *slog.Logger
}

type Service struct {
	q       *dbpkg.Queries
	adapter Adapter
	cfg     ServiceConfig

	mergesMu sync.Mutex
	merges   map[string]chan struct{}
}

func NewService(q *dbpkg.Queries, a Adapter, cfg ServiceConfig) *Service {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Service{q: q, adapter: a, cfg: cfg, merges: map[string]chan struct{}{}}
}

func (s *Service) Adapter() Adapter { return s.adapter }

func (s *Service) WaitForOngoingMerges(ctx context.Context) {
	s.mergesMu.Lock()
	chans := make([]chan struct{}, 0, len(s.merges))
	for _, c := range s.merges {
		chans = append(chans, c)
	}
	s.mergesMu.Unlock()
	for _, c := range chans {
		select {
		case <-c:
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) CreateUpload(ctx context.Context, key, version, scope, repoID string) (*dbpkg.Upload, error) {
	existing, err := s.q.FindUploadByKey(ctx, key, version, scope, repoID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, nil
	}
	id := ids.NumericID()
	u := dbpkg.Upload{
		ID:         id,
		Key:        key,
		Version:    version,
		Scope:      scope,
		RepoID:     repoID,
		CreatedAt:  time.Now().UnixMilli(),
		FolderName: strconv.FormatInt(id, 10),
	}
	if err := s.q.InsertUpload(ctx, u); err != nil {
		return nil, fmt.Errorf("insert upload: %w", err)
	}
	return &u, nil
}

var ErrInvalidUpload = errors.New("invalid upload")

type _ = io.Reader
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/storage/...`
Expected: PASS for `TestCreateUpload_NewAndIdempotent`.

- [ ] **Step 6: Commit**

```bash
git add internal/storage/service.go internal/storage/service_test.go
git commit -m "feat(storage): Service.CreateUpload"
```

---

### Task 6.2: `UploadPart`

**Files:**
- Modify: `internal/storage/service.go`
- Modify: `internal/storage/service_test.go`

- [ ] **Step 1: Append the failing test**

```go
func TestUploadPart_StoresPartAndUpdatesCounters(t *testing.T) {
	ctx := context.Background()
	svc, q := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")

	if err := svc.UploadPart(ctx, u.ID, 0, strings.NewReader("part-zero")); err != nil {
		t.Fatalf("UploadPart: %v", err)
	}

	got, _ := q.FindUploadByID(ctx, u.ID)
	if got.StartedPartUploadCount != 1 || got.FinishedPartUploadCount != 1 {
		t.Errorf("counters: %+v", got)
	}

	r, err := svc.adapter.CreateDownloadStream(ctx, fmt.Sprintf("%s/parts/0", u.FolderName))
	if err != nil {
		t.Fatalf("download part: %v", err)
	}
	defer r.Close()
	body, _ := io.ReadAll(r)
	if string(body) != "part-zero" {
		t.Errorf("got %q", body)
	}
}

func TestUploadPart_UnknownUploadIsNoop(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	if err := svc.UploadPart(ctx, 9999999999, 0, strings.NewReader("x")); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
```

(Add `"fmt"`, `"io"`, `"strings"` to imports.)

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/storage/...`
Expected: build error.

- [ ] **Step 3: Add `UploadPart`** to `service.go`

```go
func (s *Service) UploadPart(ctx context.Context, uploadID int64, partIndex int, body io.Reader) error {
	u, err := s.q.FindUploadByID(ctx, uploadID)
	if err != nil {
		return err
	}
	if u == nil {
		return nil
	}
	if err := s.q.IncStartedPartCount(ctx, uploadID); err != nil {
		return err
	}
	if err := s.adapter.UploadStream(ctx, fmt.Sprintf("%s/parts/%d", u.FolderName, partIndex), body); err != nil {
		return err
	}
	return s.q.IncFinishedPartCount(ctx, uploadID, time.Now().UnixMilli())
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/storage/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage
git commit -m "feat(storage): Service.UploadPart"
```

---

### Task 6.3: `CompleteUpload`

**Files:**
- Modify: `internal/storage/service.go`
- Modify: `internal/storage/service_test.go`

- [ ] **Step 1: Append the failing test**

```go
func TestCompleteUpload_NewEntry(t *testing.T) {
	ctx := context.Background()
	svc, q := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("hello"))
	_ = svc.UploadPart(ctx, u.ID, 1, strings.NewReader("world"))

	got, err := svc.CompleteUpload(ctx, "k", "v", "s", "r")
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	if got == nil {
		t.Fatal("expected upload returned")
	}

	entry, _ := q.FindExactCacheEntry(ctx, "k", "v", "s", "r")
	if entry == nil {
		t.Fatal("expected cache entry")
	}
	loc, _ := q.GetStorageLocation(ctx, entry.LocationID)
	if loc == nil || loc.PartCount != 2 {
		t.Errorf("loc=%+v", loc)
	}
}

func TestCompleteUpload_ReplacesExisting(t *testing.T) {
	ctx := context.Background()
	svc, q := newServiceForTest(t)

	u1, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u1.ID, 0, strings.NewReader("v1"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "s", "r")

	u2, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u2.ID, 0, strings.NewReader("v2"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "s", "r")

	entry, _ := q.FindExactCacheEntry(ctx, "k", "v", "s", "r")
	if entry == nil {
		t.Fatal("expected cache entry")
	}
}

func TestCompleteUpload_ZeroPartsRejected(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	_, _ = svc.CreateUpload(ctx, "k", "v", "s", "r")
	_, err := svc.CompleteUpload(ctx, "k", "v", "s", "r")
	if err == nil {
		t.Fatal("expected error when 0 parts uploaded")
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/storage/...`
Expected: build error.

- [ ] **Step 3: Implement `CompleteUpload`**

```go
func (s *Service) CompleteUpload(ctx context.Context, key, version, scope, repoID string) (*dbpkg.Upload, error) {
	u, err := s.q.FindUploadByKey(ctx, key, version, scope, repoID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}

	if u.FinishedPartUploadCount == 0 {
		_ = s.q.DeleteUpload(ctx, u.ID)
		return nil, errors.New("no parts have been uploaded")
	}
	if u.StartedPartUploadCount != u.FinishedPartUploadCount {
		_ = s.q.DeleteUpload(ctx, u.ID)
		return nil, fmt.Errorf("not all parts uploaded (%d of %d)", u.FinishedPartUploadCount, u.StartedPartUploadCount)
	}

	partsFolder := fmt.Sprintf("%s/parts", u.FolderName)
	partCount, err := s.adapter.CountFilesInFolder(ctx, partsFolder)
	if err != nil {
		return nil, fmt.Errorf("count parts: %w", err)
	}
	if partCount != u.FinishedPartUploadCount {
		_ = s.q.DeleteUpload(ctx, u.ID)
		return nil, fmt.Errorf("part count mismatch: db=%d storage=%d", u.FinishedPartUploadCount, partCount)
	}

	tx, err := s.q.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	locID := ids.UUIDv4()
	if err := s.q.InsertStorageLocation(ctx, dbpkg.StorageLocation{
		ID: locID, FolderName: u.FolderName, PartCount: partCount,
	}); err != nil {
		return nil, err
	}

	existing, existingLoc, err := s.q.FindExistingCacheEntryWithLocation(ctx, key, version, scope, repoID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if existing != nil {
		if err := s.q.UpdateCacheEntryLocation(ctx, existing.ID, locID, now); err != nil {
			return nil, err
		}
		if err := s.q.DeleteStorageLocation(ctx, existingLoc.ID); err != nil {
			return nil, err
		}
		if err := s.adapter.DeleteFolder(ctx, existingLoc.FolderName); err != nil {
			return nil, err
		}
	} else {
		if err := s.q.InsertCacheEntry(ctx, dbpkg.CacheEntry{
			ID: ids.UUIDv4(), Key: key, Version: version, Scope: scope, RepoID: repoID,
			UpdatedAt: now, LocationID: locID,
		}); err != nil {
			return nil, err
		}
	}
	if err := s.q.DeleteUpload(ctx, u.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return u, nil
}
```

> **Note:** the `BeginTx`/`Commit` here is for the DB rows; the adapter calls run outside the transaction. This matches the TS source where adapter.deleteFolder is awaited inside the transaction callback but the underlying DB tx only governs the SQL — losing storage and DB consistency on a crash mid-replace is an existing issue we don't introduce.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/storage/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage
git commit -m "feat(storage): Service.CompleteUpload with replace semantics"
```

---

### Task 6.4: `Download` with merge-on-first-read

The TS code does a clever trick: on the first download for a not-yet-merged entry, it streams the parts to the response **and** to a writer that uploads a `merged` blob in the background. Subsequent downloads can then use the merged blob (and a presigned URL when the adapter supports it).

We replicate this with `io.Pipe` + `io.MultiWriter`. The merge runs in a goroutine; we register it in `s.merges` so `WaitForOngoingMerges` can drain it on shutdown.

**Files:**
- Modify: `internal/storage/service.go`
- Modify: `internal/storage/service_test.go`

- [ ] **Step 1: Append the failing test**

```go
func TestDownload_FirstReadStreamsAndMerges(t *testing.T) {
	ctx := context.Background()
	svc, q := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("AAA"))
	_ = svc.UploadPart(ctx, u.ID, 1, strings.NewReader("BBB"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "s", "r")

	entry, _ := q.FindExactCacheEntry(ctx, "k", "v", "s", "r")
	r, err := svc.Download(ctx, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	body, _ := io.ReadAll(r)
	if string(body) != "AAABBB" {
		t.Errorf("got %q", body)
	}
	svc.WaitForOngoingMerges(ctx)
	merged, err := svc.adapter.CreateDownloadStream(ctx, u.FolderName+"/merged")
	if err != nil {
		t.Fatalf("merged not present: %v", err)
	}
	defer merged.Close()
	mb, _ := io.ReadAll(merged)
	if string(mb) != "AAABBB" {
		t.Errorf("merged got %q", mb)
	}
}

func TestDownload_NotFoundReturnsNil(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	r, err := svc.Download(ctx, "nope")
	if err != nil {
		t.Fatalf("expected nil error for missing entry, got %v", err)
	}
	if r != nil {
		t.Error("expected nil reader for missing entry")
	}
}

func TestDownload_StaleParts_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	svc, q := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("X"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "s", "r")
	_ = svc.adapter.Clear(ctx)

	entry, _ := q.FindExactCacheEntry(ctx, "k", "v", "s", "r")
	r, err := svc.Download(ctx, entry.ID)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if r != nil {
		t.Error("expected nil reader for stale entry")
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/storage/...`
Expected: failures.

- [ ] **Step 3: Implement `Download`**

```go
func (s *Service) Download(ctx context.Context, cacheEntryID string) (io.ReadCloser, error) {
	_, loc, err := s.q.FindCacheEntryWithLocation(ctx, cacheEntryID)
	if err != nil {
		return nil, err
	}
	if loc == nil {
		return nil, nil
	}
	go func() {
		_ = s.q.UpdateStorageLastDownloaded(context.Background(), loc.ID, time.Now().UnixMilli())
	}()

	if loc.MergedAt != nil {
		r, err := s.adapter.CreateDownloadStream(ctx, loc.FolderName+"/merged")
		if errors.Is(err, ErrObjectNotFound) {
			s.cfg.Logger.Warn("stale cache entry: merged blob missing", "id", cacheEntryID)
			return nil, nil
		}
		return r, err
	}
	if loc.MergeStartedAt != nil {
		return s.streamPartsReader(ctx, loc), nil
	}

	if err := s.ensurePartsExist(ctx, loc); err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			s.cfg.Logger.Warn("stale cache entry: parts missing", "id", cacheEntryID)
			return nil, nil
		}
		return nil, err
	}

	if err := s.q.UpdateStorageMergeStarted(ctx, loc.ID, time.Now().UnixMilli()); err != nil {
		return nil, err
	}

	respR, respW := io.Pipe()
	mergeR, mergeW := io.Pipe()
	done := make(chan struct{})

	s.mergesMu.Lock()
	s.merges[loc.ID] = done
	s.mergesMu.Unlock()

	go func() {
		defer close(done)
		defer func() {
			s.mergesMu.Lock()
			delete(s.merges, loc.ID)
			s.mergesMu.Unlock()
		}()
		bgCtx := context.Background()
		if err := s.adapter.UploadStream(bgCtx, loc.FolderName+"/merged", mergeR); err != nil {
			s.cfg.Logger.Error("merge upload failed", "id", loc.ID, "err", err)
			_ = s.q.ResetStorageMerge(bgCtx, loc.ID)
			return
		}
		if err := s.q.UpdateStorageMerged(bgCtx, loc.ID, time.Now().UnixMilli()); err != nil {
			s.cfg.Logger.Error("mark merged failed", "id", loc.ID, "err", err)
			return
		}
		_ = s.q.UpdateStoragePartsDeleted(bgCtx, loc.ID, time.Now().UnixMilli())
		if err := s.adapter.DeleteFolder(bgCtx, loc.FolderName+"/parts"); err != nil {
			s.cfg.Logger.Error("delete parts after merge failed", "id", loc.ID, "err", err)
		}
	}()

	go func() {
		defer respW.Close()
		defer mergeW.Close()
		mw := io.MultiWriter(respW, mergeW)
		if err := s.streamPartsTo(ctx, loc, mw); err != nil {
			respW.CloseWithError(err)
			mergeW.CloseWithError(err)
		}
	}()

	return respR, nil
}

func (s *Service) ensurePartsExist(ctx context.Context, loc *dbpkg.StorageLocation) error {
	n, err := s.adapter.CountFilesInFolder(ctx, loc.FolderName+"/parts")
	if err != nil {
		return err
	}
	if n < loc.PartCount {
		return ErrObjectNotFound
	}
	return nil
}

func (s *Service) streamPartsTo(ctx context.Context, loc *dbpkg.StorageLocation, w io.Writer) error {
	for i := 0; i < loc.PartCount; i++ {
		r, err := s.adapter.CreateDownloadStream(ctx, fmt.Sprintf("%s/parts/%d", loc.FolderName, i))
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(w, r)
		_ = r.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func (s *Service) streamPartsReader(ctx context.Context, loc *dbpkg.StorageLocation) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		if err := s.streamPartsTo(ctx, loc, pw); err != nil {
			pw.CloseWithError(err)
		}
	}()
	return pr
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/storage/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage
git commit -m "feat(storage): Service.Download with merge-on-first-read"
```

---

### Task 6.5: `MatchCacheEntry` and `GetCacheEntryWithDownloadURL`

**Files:**
- Modify: `internal/storage/service.go`
- Modify: `internal/storage/service_test.go`

- [ ] **Step 1: Append the failing test**

```go
func TestMatchCacheEntry_PrefersExactPrimary(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	mustSave := func(key, val string) {
		t.Helper()
		u, _ := svc.CreateUpload(ctx, key, "v", "main", "r")
		_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader(val))
		_, err := svc.CompleteUpload(ctx, key, "v", "main", "r")
		if err != nil {
			t.Fatal(err)
		}
	}
	mustSave("deps-abc", "1")
	mustSave("deps-xyz", "2")

	got, err := svc.MatchCacheEntry(ctx, MatchInput{
		Keys: []string{"deps-abc"}, Version: "v", Scopes: []string{"main"}, RepoID: "r",
	})
	if err != nil || got == nil {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got.Type != MatchExactPrimary || got.Entry.Key != "deps-abc" {
		t.Errorf("got %+v", got)
	}
}

func TestMatchCacheEntry_PrefixedPrimary(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "deps-abc", "v", "main", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("x"))
	_, _ = svc.CompleteUpload(ctx, "deps-abc", "v", "main", "r")

	got, _ := svc.MatchCacheEntry(ctx, MatchInput{
		Keys: []string{"deps-"}, Version: "v", Scopes: []string{"main"}, RepoID: "r",
	})
	if got == nil || got.Type != MatchPrefixedPrimary {
		t.Errorf("got %+v", got)
	}
}

func TestMatchCacheEntry_FallsBackToRestoreKey(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "deps-abc", "v", "main", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("x"))
	_, _ = svc.CompleteUpload(ctx, "deps-abc", "v", "main", "r")

	got, _ := svc.MatchCacheEntry(ctx, MatchInput{
		Keys: []string{"missing-key", "deps-abc"}, Version: "v", Scopes: []string{"main"}, RepoID: "r",
	})
	if got == nil || got.Type != MatchExactRestore {
		t.Errorf("got %+v", got)
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/storage/...`
Expected: build error.

- [ ] **Step 3: Implement match logic**

```go
type MatchType string

const (
	MatchExactPrimary    MatchType = "exact-primary"
	MatchPrefixedPrimary MatchType = "prefixed-primary"
	MatchExactRestore    MatchType = "exact-restore"
	MatchPrefixedRestore MatchType = "prefixed-restore"
)

type MatchInput struct {
	Keys    []string
	Version string
	Scopes  []string
	RepoID  string
}

type Match struct {
	Entry dbpkg.CacheEntry
	Type  MatchType
}

func (s *Service) MatchCacheEntry(ctx context.Context, in MatchInput) (*Match, error) {
	if len(in.Keys) == 0 {
		return nil, errors.New("at least one key required")
	}
	primary, restore := in.Keys[0], in.Keys[1:]
	for _, scope := range in.Scopes {
		if e, err := s.q.FindExactCacheEntry(ctx, primary, in.Version, scope, in.RepoID); err != nil {
			return nil, err
		} else if e != nil {
			return &Match{Entry: *e, Type: MatchExactPrimary}, nil
		}
		if e, err := s.q.FindPrefixedCacheEntry(ctx, primary, in.Version, scope, in.RepoID); err != nil {
			return nil, err
		} else if e != nil {
			return &Match{Entry: *e, Type: MatchPrefixedPrimary}, nil
		}
		for _, k := range restore {
			if e, err := s.q.FindExactCacheEntry(ctx, k, in.Version, scope, in.RepoID); err != nil {
				return nil, err
			} else if e != nil {
				return &Match{Entry: *e, Type: MatchExactRestore}, nil
			}
			if e, err := s.q.FindPrefixedCacheEntry(ctx, k, in.Version, scope, in.RepoID); err != nil {
				return nil, err
			} else if e != nil {
				return &Match{Entry: *e, Type: MatchPrefixedRestore}, nil
			}
		}
	}
	return nil, nil
}

type ResolvedDownload struct {
	Entry       dbpkg.CacheEntry
	DownloadURL string
}

func (s *Service) GetCacheEntryWithDownloadURL(ctx context.Context, in MatchInput) (*ResolvedDownload, error) {
	m, err := s.MatchCacheEntry(ctx, in)
	if err != nil || m == nil {
		return nil, err
	}
	defaultURL := fmt.Sprintf("%s/download/%s", s.cfg.APIBaseURL, m.Entry.ID)
	if !s.cfg.EnableDirectDownloads {
		return &ResolvedDownload{Entry: m.Entry, DownloadURL: defaultURL}, nil
	}
	loc, err := s.q.GetStorageLocation(ctx, m.Entry.LocationID)
	if err != nil {
		return nil, err
	}
	if loc == nil {
		return nil, errors.New("storage location not found")
	}
	if loc.MergedAt == nil {
		return &ResolvedDownload{Entry: m.Entry, DownloadURL: defaultURL}, nil
	}
	url, ok, err := s.adapter.CreateDownloadURL(ctx, loc.FolderName+"/merged")
	if err != nil {
		return nil, err
	}
	if !ok {
		return &ResolvedDownload{Entry: m.Entry, DownloadURL: defaultURL}, nil
	}
	return &ResolvedDownload{Entry: m.Entry, DownloadURL: url}, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/storage/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage
git commit -m "feat(storage): MatchCacheEntry + GetCacheEntryWithDownloadURL"
```

---

## Phase 7 — Auth (JWKS + JWT verification)

GitHub Actions runners present a JWT issued by `https://token.actions.githubusercontent.com`. We verify it against that issuer's JWKS endpoint. The token's `ac` claim is a JSON-encoded string of `[{"Scope":"refs/heads/main","Permission":3}, ...]`; `repository_id` is a string. Permissions: 1=read, 2=write, 3=read+write.

We support RS256 (the issuer uses 2048-bit RSA) and ES256 (so we're future-proof). All from stdlib crypto.

### Task 7.1: JWT parser & verifier

**Files:**
- Create: `internal/auth/jwt.go`
- Test: `internal/auth/jwt_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func sign(t *testing.T, alg string, kid string, claims map[string]any, key any) string {
	t.Helper()
	hdr := map[string]any{"alg": alg, "typ": "JWT", "kid": kid}
	hb, _ := json.Marshal(hdr)
	cb, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding.EncodeToString
	signing := enc(hb) + "." + enc(cb)
	var sig []byte
	switch k := key.(type) {
	case *rsa.PrivateKey:
		s, err := signRS256(signing, k)
		if err != nil {
			t.Fatal(err)
		}
		sig = s
	case *ecdsa.PrivateKey:
		s, err := signES256(signing, k)
		if err != nil {
			t.Fatal(err)
		}
		sig = s
	}
	return signing + "." + enc(sig)
}

func TestVerify_RS256(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwt := sign(t, "RS256", "k1", map[string]any{"iss": "issuer", "sub": "x"}, k)
	keyset := &Keyset{keys: map[string]any{"k1": &k.PublicKey}}
	claims, err := Verify(jwt, keyset, "issuer")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims["sub"] != "x" {
		t.Errorf("claims=%+v", claims)
	}
}

func TestVerify_ES256(t *testing.T) {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	jwt := sign(t, "ES256", "e1", map[string]any{"iss": "issuer"}, k)
	keyset := &Keyset{keys: map[string]any{"e1": &k.PublicKey}}
	if _, err := Verify(jwt, keyset, "issuer"); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerify_BadIssuerRejected(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwt := sign(t, "RS256", "k1", map[string]any{"iss": "wrong"}, k)
	keyset := &Keyset{keys: map[string]any{"k1": &k.PublicKey}}
	if _, err := Verify(jwt, keyset, "issuer"); err == nil {
		t.Fatal("expected error for bad issuer")
	}
}

func TestDecodeUnverified(t *testing.T) {
	parts := []string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"x"}`)),
		"",
	}
	c, err := DecodeUnverified(strings.Join(parts, "."))
	if err != nil || c["sub"] != "x" {
		t.Errorf("DecodeUnverified: %v %+v", err, c)
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/auth/...`
Expected: build error.

- [ ] **Step 3: Implement**

Write `internal/auth/jwt.go`:

```go
package auth

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

type Claims map[string]any

type Keyset struct {
	keys map[string]any
}

func (k *Keyset) Get(kid string) any { return k.keys[kid] }

var (
	ErrMalformedToken    = errors.New("malformed token")
	ErrUnsupportedAlg    = errors.New("unsupported algorithm")
	ErrInvalidSignature  = errors.New("invalid signature")
	ErrIssuerMismatch    = errors.New("issuer mismatch")
	ErrTokenExpired      = errors.New("token expired")
	ErrUnknownKey        = errors.New("unknown signing key")
)

func DecodeUnverified(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrMalformedToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	return c, nil
}

func Verify(token string, ks *Keyset, expectedIssuer string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrMalformedToken
	}
	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		return nil, fmt.Errorf("unmarshal header: %w", err)
	}
	key := ks.Get(hdr.Kid)
	if key == nil {
		return nil, fmt.Errorf("%w: kid=%q", ErrUnknownKey, hdr.Kid)
	}
	signed := []byte(parts[0] + "." + parts[1])
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	switch hdr.Alg {
	case "RS256":
		pk, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%w: kid %q is not RSA", ErrUnsupportedAlg, hdr.Kid)
		}
		h := sha256.Sum256(signed)
		if err := rsa.VerifyPKCS1v15(pk, crypto.SHA256, h[:], sig); err != nil {
			return nil, ErrInvalidSignature
		}
	case "ES256":
		pk, ok := key.(*ecdsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%w: kid %q is not ECDSA", ErrUnsupportedAlg, hdr.Kid)
		}
		if len(sig) != 64 {
			return nil, ErrInvalidSignature
		}
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		h := sha256.Sum256(signed)
		if !ecdsa.Verify(pk, h[:], r, s) {
			return nil, ErrInvalidSignature
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlg, hdr.Alg)
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, err
	}

	if iss, _ := c["iss"].(string); iss != expectedIssuer {
		return nil, fmt.Errorf("%w: got %q want %q", ErrIssuerMismatch, iss, expectedIssuer)
	}
	if expF, ok := c["exp"].(float64); ok {
		if int64(expF) < time.Now().Unix() {
			return nil, ErrTokenExpired
		}
	}
	return c, nil
}

func signRS256(input string, k *rsa.PrivateKey) ([]byte, error) {
	h := sha256.Sum256([]byte(input))
	return rsa.SignPKCS1v15(nil, k, crypto.SHA256, h[:])
}

func signES256(input string, k *ecdsa.PrivateKey) ([]byte, error) {
	h := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(nilReader{}, k, h[:])
	if err != nil {
		return nil, err
	}
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	s.FillBytes(out[32:])
	return out, nil
}

type nilReader struct{}

func (nilReader) Read(b []byte) (int, error) {
	return cryptoRand(b)
}
```

Add `internal/auth/rand_compat.go`:

```go
package auth

import "crypto/rand"

func cryptoRand(b []byte) (int, error) { return rand.Read(b) }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/auth/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/jwt.go internal/auth/jwt_test.go internal/auth/rand_compat.go
git commit -m "feat(auth): RS256/ES256 JWT verification with stdlib crypto"
```

---

### Task 7.2: JWKS fetcher

JWKS responses look like:
```json
{ "keys": [ { "kty":"RSA", "kid":"...", "n":"...", "e":"AQAB", "alg":"RS256" }, ... ] }
```

We fetch and cache for 10 minutes. On unknown kid, force-refetch (handles key rotation).

**Files:**
- Create: `internal/auth/jwks.go`
- Test: `internal/auth/jwks_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func jwksWithRSA(t *testing.T, kid string, k *rsa.PublicKey) string {
	t.Helper()
	body := map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"kid": kid,
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01}),
		}},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

func TestFetcher_LoadsAndCaches(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jwksWithRSA(t, "k1", &k.PublicKey)))
	}))
	defer srv.Close()

	f := NewJWKSFetcher(srv.URL)
	ks, err := f.Fetch(t.Context())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if ks.Get("k1") == nil {
		t.Error("expected k1")
	}
	_, _ = f.Fetch(t.Context())
	if calls != 1 {
		t.Errorf("expected 1 HTTP call (cache hit), got %d", calls)
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/auth/...`
Expected: build error.

- [ ] **Step 3: Implement**

Write `internal/auth/jwks.go`:

```go
package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

type JWKSFetcher struct {
	url    string
	client *http.Client
	ttl    time.Duration

	mu      sync.Mutex
	current *Keyset
	expires time.Time
}

func NewJWKSFetcher(url string) *JWKSFetcher {
	return &JWKSFetcher{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
		ttl:    10 * time.Minute,
	}
}

func (f *JWKSFetcher) Fetch(ctx context.Context) (*Keyset, error) {
	f.mu.Lock()
	if f.current != nil && time.Now().Before(f.expires) {
		ks := f.current
		f.mu.Unlock()
		return ks, nil
	}
	f.mu.Unlock()
	return f.refresh(ctx)
}

func (f *JWKSFetcher) ForceRefresh(ctx context.Context) (*Keyset, error) {
	return f.refresh(ctx)
}

func (f *JWKSFetcher) refresh(ctx context.Context) (*Keyset, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks fetch: %s", resp.Status)
	}
	var raw struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("jwks decode: %w", err)
	}
	keys := map[string]any{}
	for _, k := range raw.Keys {
		switch k.Kty {
		case "RSA":
			n, err := base64.RawURLEncoding.DecodeString(k.N)
			if err != nil {
				continue
			}
			e, err := base64.RawURLEncoding.DecodeString(k.E)
			if err != nil {
				continue
			}
			eInt := 0
			for _, b := range e {
				eInt = eInt<<8 | int(b)
			}
			keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: eInt}
		case "EC":
			if k.Crv != "P-256" {
				continue
			}
			x, _ := base64.RawURLEncoding.DecodeString(k.X)
			y, _ := base64.RawURLEncoding.DecodeString(k.Y)
			keys[k.Kid] = &ecdsa.PublicKey{
				Curve: elliptic.P256(),
				X:     new(big.Int).SetBytes(x),
				Y:     new(big.Int).SetBytes(y),
			}
		}
	}
	ks := &Keyset{keys: keys}
	f.mu.Lock()
	f.current = ks
	f.expires = time.Now().Add(f.ttl)
	f.mu.Unlock()
	return ks, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/auth/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/jwks.go internal/auth/jwks_test.go
git commit -m "feat(auth): JWKS fetcher with TTL cache"
```

---

### Task 7.3: `getCacheScope` equivalent

**Files:**
- Create: `internal/auth/scope.go`
- Test: `internal/auth/scope_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractScopes_FromValidToken(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(jwksWithRSA(t, "k1", &k.PublicKey)))
	}))
	defer srv.Close()
	v := NewVerifier(NewJWKSFetcher(srv.URL), "issuer", false)
	ac, _ := json.Marshal([]Scope{{Scope: "refs/heads/main", Permission: 3}})
	tok := sign(t, "RS256", "k1", map[string]any{
		"iss": "issuer", "ac": string(ac), "repository_id": "42",
	}, k)
	res, err := v.Authorize(context.Background(), "Bearer "+tok)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if res.RepoID != "42" || len(res.Scopes) != 1 || res.Scopes[0].Permission != 3 {
		t.Errorf("got %+v", res)
	}
}

func TestExtractScopes_SkipsValidation(t *testing.T) {
	v := NewVerifier(nil, "issuer", true)
	ac, _ := json.Marshal([]Scope{{Scope: "x", Permission: 2}})
	parts := []string{
		"e30",
		mustB64Json(map[string]any{"ac": string(ac), "repository_id": "1"}),
		"",
	}
	res, err := v.Authorize(context.Background(), "Bearer "+joinDots(parts))
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if res.RepoID != "1" || len(res.Scopes) != 1 {
		t.Errorf("got %+v", res)
	}
}
```

Add helpers in same test file:

```go
import (
	"encoding/base64"
	"strings"
)

func mustB64Json(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

func joinDots(parts []string) string { return strings.Join(parts, ".") }
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/auth/...`
Expected: build error.

- [ ] **Step 3: Implement**

Write `internal/auth/scope.go`:

```go
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Scope struct {
	Scope      string `json:"Scope"`
	Permission int    `json:"Permission"`
}

type AuthResult struct {
	RepoID string
	Scopes []Scope
}

type Verifier struct {
	fetcher  *JWKSFetcher
	issuer   string
	skipAuth bool
}

func NewVerifier(f *JWKSFetcher, issuer string, skip bool) *Verifier {
	return &Verifier{fetcher: f, issuer: issuer, skipAuth: skip}
}

var ErrUnauthorized = errors.New("unauthorized")

func (v *Verifier) Authorize(ctx context.Context, authzHeader string) (*AuthResult, error) {
	if !strings.HasPrefix(authzHeader, "Bearer ") {
		return nil, fmt.Errorf("%w: missing or malformed Authorization header", ErrUnauthorized)
	}
	token := strings.TrimPrefix(authzHeader, "Bearer ")

	var claims Claims
	var err error
	if v.skipAuth {
		claims, err = DecodeUnverified(token)
	} else {
		ks, ferr := v.fetcher.Fetch(ctx)
		if ferr != nil {
			return nil, fmt.Errorf("jwks fetch: %w", ferr)
		}
		claims, err = Verify(token, ks, v.issuer)
		if errors.Is(err, ErrUnknownKey) {
			ks, ferr = v.fetcher.ForceRefresh(ctx)
			if ferr == nil {
				claims, err = Verify(token, ks, v.issuer)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}

	acRaw, _ := claims["ac"].(string)
	if acRaw == "" {
		return nil, fmt.Errorf("%w: token missing cache scopes", ErrUnauthorized)
	}
	var scopes []Scope
	if err := json.Unmarshal([]byte(acRaw), &scopes); err != nil {
		return nil, fmt.Errorf("%w: invalid scopes JSON", ErrUnauthorized)
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("%w: empty scopes", ErrUnauthorized)
	}
	repoID, _ := claims["repository_id"].(string)
	if repoID == "" {
		if f, ok := claims["repository_id"].(float64); ok {
			repoID = fmt.Sprintf("%d", int64(f))
		}
	}
	if repoID == "" {
		return nil, fmt.Errorf("%w: token missing repository_id", ErrUnauthorized)
	}
	return &AuthResult{RepoID: repoID, Scopes: scopes}, nil
}

func WriteScope(scopes []Scope) (Scope, bool) {
	for _, s := range scopes {
		if s.Permission >= 2 {
			return s, true
		}
	}
	return Scope{}, false
}

func ScopesByPermissionDesc(scopes []Scope) []string {
	cp := append([]Scope(nil), scopes...)
	for i := 1; i < len(cp); i++ {
		for j := i; j > 0 && cp[j].Permission > cp[j-1].Permission; j-- {
			cp[j], cp[j-1] = cp[j-1], cp[j]
		}
	}
	out := make([]string, len(cp))
	for i, s := range cp {
		out[i] = s.Scope
	}
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/auth/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/scope.go internal/auth/scope_test.go
git commit -m "feat(auth): scope extraction + write/read permission helpers"
```

---

## Phase 8 — HTTP Server Skeleton

### Task 8.1: Server, middleware, health, root

The TS server is built on h3/Nitro. We use `net/http`'s ServeMux (Go 1.22+ supports method+pattern routes).

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/middleware.go`
- Create: `internal/server/health.go`

- [ ] **Step 1: Write `health.go`**

```go
package server

import "net/http"

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("healthy"))
}

func handleRoot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
```

- [ ] **Step 2: Write `middleware.go`**

```go
package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(logger *slog.Logger, debugMode bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r)
			elapsed := time.Since(start)
			lvl := slog.LevelInfo
			if rec.status >= 500 {
				lvl = slog.LevelError
			} else if !debugMode && r.URL.Path == "/health" {
				return
			}
			logger.Log(r.Context(), lvl, "http",
				"method", r.Method, "path", r.URL.Path,
				"status", rec.status, "elapsed_ms", elapsed.Milliseconds())
		})
	}
}

func recoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic", "err", rec, "stack", string(debug.Stack()))
					http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
```

- [ ] **Step 3: Write `server.go`**

```go
package server

import (
	"log/slog"
	"net/http"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/auth"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/storage"
)

type Deps struct {
	Cfg      *config.Config
	Logger   *slog.Logger
	Storage  *storage.Service
	Verifier *auth.Verifier
}

func NewHandler(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /{$}", handleRoot)
	registerTwirp(mux, d)
	registerUpload(mux, d)
	registerDownload(mux, d)
	registerManagement(mux, d)
	registerProxy(mux, d)
	return chain(mux,
		recoverMiddleware(d.Logger),
		loggingMiddleware(d.Logger, d.Cfg.Debug),
	)
}
```

- [ ] **Step 4: Add stub registrations** (these will be filled in later phases — use empty handlers so `go build` works now)

Append to `server.go`:

```go
func registerTwirp(_ *http.ServeMux, _ Deps)      {}
func registerUpload(_ *http.ServeMux, _ Deps)     {}
func registerDownload(_ *http.ServeMux, _ Deps)   {}
func registerManagement(_ *http.ServeMux, _ Deps) {}
func registerProxy(_ *http.ServeMux, _ Deps)      {}
```

- [ ] **Step 5: Verify build**

Run: `go build ./...`
Expected: exits 0.

- [ ] **Step 6: Smoke test health**

Add `internal/server/health_test.go`:

```go
package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/logging"
)

func TestHealth(t *testing.T) {
	h := NewHandler(Deps{
		Cfg:    &config.Config{},
		Logger: logging.New(false),
	})
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "healthy" {
		t.Errorf("status=%d body=%q", resp.StatusCode, body)
	}
}
```

Run: `go test ./internal/server/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/server
git commit -m "feat(server): handler skeleton with health, logging, recover middleware"
```

---

## Phase 9 — Twirp Handlers (CreateCacheEntry, GetCacheEntryDownloadURL, FinalizeCacheEntryUpload)

The actions/cache v2 client speaks Twirp **JSON** to these endpoints (Content-Type: `application/json`). Field names are snake_case, return shapes match the TS handlers exactly.

### Task 9.1: Twirp endpoints

**Files:**
- Create: `internal/server/twirp.go`
- Test: `internal/server/twirp_test.go`

- [ ] **Step 1: Write the failing test**

```go
package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/auth"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
	dbpkg "github.com/falcondev-oss/github-actions-cache-server-go/internal/db"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/logging"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/storage"

	_ "modernc.org/sqlite"
	"database/sql"
)

func newTestServer(t *testing.T) (*httptest.Server, *storage.Service, string) {
	t.Helper()
	raw, _ := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	t.Cleanup(func() { raw.Close() })
	raw.SetMaxOpenConns(1)
	d := &dbpkg.DB{DB: raw, Driver: dbpkg.SQLite}
	if err := dbpkg.Migrate(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	q := dbpkg.New(d)
	a, _ := storage.NewFilesystemAdapter(t.TempDir())
	cfg := &config.Config{
		APIBaseURL:          "http://localhost:3000",
		SkipTokenValidation: true,
	}
	svc := storage.NewService(q, a, storage.ServiceConfig{APIBaseURL: cfg.APIBaseURL})
	v := auth.NewVerifier(nil, "issuer", true)
	h := NewHandler(Deps{Cfg: cfg, Logger: logging.New(false), Storage: svc, Verifier: v})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	tok := makeUnsignedToken(t, []auth.Scope{{Scope: "main", Permission: 3}}, "42")
	return srv, svc, tok
}

func makeUnsignedToken(t *testing.T, scopes []auth.Scope, repoID string) string {
	t.Helper()
	_, _ = rsa.GenerateKey(rand.Reader, 2048)
	ac, _ := json.Marshal(scopes)
	header := `{"alg":"none","typ":"JWT"}`
	payload, _ := json.Marshal(map[string]any{
		"ac": string(ac), "repository_id": repoID,
	})
	enc := func(b []byte) string {
		return strings.TrimRight(strings.NewReplacer("+", "-", "/", "_").Replace(b64(b)), "=")
	}
	return enc([]byte(header)) + "." + enc(payload) + "."
}

func b64(b []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	out := make([]byte, 0, ((len(b)+2)/3)*4)
	for i := 0; i < len(b); i += 3 {
		var n uint32
		switch len(b) - i {
		case 1:
			n = uint32(b[i]) << 16
			out = append(out, alphabet[(n>>18)&0x3f], alphabet[(n>>12)&0x3f], '=', '=')
		case 2:
			n = uint32(b[i])<<16 | uint32(b[i+1])<<8
			out = append(out, alphabet[(n>>18)&0x3f], alphabet[(n>>12)&0x3f], alphabet[(n>>6)&0x3f], '=')
		default:
			n = uint32(b[i])<<16 | uint32(b[i+1])<<8 | uint32(b[i+2])
			out = append(out, alphabet[(n>>18)&0x3f], alphabet[(n>>12)&0x3f], alphabet[(n>>6)&0x3f], alphabet[n&0x3f])
		}
	}
	return string(out)
}

func TestTwirp_CreateCacheEntry(t *testing.T) {
	srv, _, tok := newTestServer(t)
	body, _ := json.Marshal(map[string]any{"key": "k", "version": "v"})
	req, _ := http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["ok"] != true {
		t.Errorf("expected ok=true, got %+v", got)
	}
	url, _ := got["signed_upload_url"].(string)
	if !strings.HasPrefix(url, "http://localhost:3000/devstoreaccount1/upload/") {
		t.Errorf("signed_upload_url = %q", url)
	}
}

func TestTwirp_FinalizeAndGet(t *testing.T) {
	srv, svc, tok := newTestServer(t)
	u, _ := svc.CreateUpload(context.Background(), "k", "v", "main", "42")
	_ = svc.UploadPart(context.Background(), u.ID, 0, strings.NewReader("hi"))

	body, _ := json.Marshal(map[string]any{"key": "k", "version": "v"})
	req, _ := http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/FinalizeCacheEntryUpload",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("finalize status=%d", resp.StatusCode)
	}

	body, _ = json.Marshal(map[string]any{"key": "k", "version": "v"})
	req, _ = http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/GetCacheEntryDownloadURL",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("get status=%d", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["ok"] != true || got["matched_key"] != "k" {
		t.Errorf("got %+v", got)
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/server/...`
Expected: failure.

- [ ] **Step 3: Replace `registerTwirp` stub**

Edit `internal/server/server.go` and remove the `func registerTwirp(_ *http.ServeMux, _ Deps) {}` stub.

Write `internal/server/twirp.go`:

```go
package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/auth"
)

const twirpPrefix = "/twirp/github.actions.results.api.v1.CacheService"

func registerTwirp(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("POST "+twirpPrefix+"/CreateCacheEntry", twirpCreate(d))
	mux.HandleFunc("POST "+twirpPrefix+"/GetCacheEntryDownloadURL", twirpGet(d))
	mux.HandleFunc("POST "+twirpPrefix+"/FinalizeCacheEntryUpload", twirpFinalize(d))
}

type twirpKeyVersion struct {
	Key     string `json:"key"`
	Version string `json:"version"`
}

type twirpKeyRestoreVersion struct {
	Key         string   `json:"key"`
	RestoreKeys []string `json:"restore_keys"`
	Version     string   `json:"version"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeTwirpErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"code": code, "msg": msg})
}

func authorize(d Deps, r *http.Request) (*auth.AuthResult, error) {
	return d.Verifier.Authorize(r.Context(), r.Header.Get("Authorization"))
}

func twirpCreate(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := authorize(d, r)
		if err != nil {
			writeTwirpErr(w, http.StatusUnauthorized, "unauthenticated", err.Error())
			return
		}
		var body twirpKeyVersion
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" || body.Version == "" {
			writeTwirpErr(w, http.StatusBadRequest, "invalid_argument", "key and version required")
			return
		}
		write, ok := auth.WriteScope(res.Scopes)
		if !ok {
			writeTwirpErr(w, http.StatusForbidden, "permission_denied", "no scope with write permission")
			return
		}
		u, err := d.Storage.CreateUpload(r.Context(), body.Key, body.Version, write.Scope, res.RepoID)
		if err != nil {
			writeTwirpErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if u == nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true,
			"signed_upload_url": d.Cfg.APIBaseURL + "/devstoreaccount1/upload/" +
				strconvFormatInt(u.ID),
		})
	}
}

func twirpGet(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := authorize(d, r)
		if err != nil {
			writeTwirpErr(w, http.StatusUnauthorized, "unauthenticated", err.Error())
			return
		}
		var body twirpKeyRestoreVersion
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeTwirpErr(w, http.StatusBadRequest, "invalid_argument", err.Error())
			return
		}
		if body.Key == "" || body.Version == "" {
			writeTwirpErr(w, http.StatusBadRequest, "invalid_argument", "key and version required")
			return
		}
		keys := append([]string{body.Key}, body.RestoreKeys...)
		match, err := d.Storage.GetCacheEntryWithDownloadURL(r.Context(), storageMatchInput(keys, body.Version, res))
		if err != nil {
			writeTwirpErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if match == nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true,
			"signed_download_url": match.DownloadURL,
			"matched_key":         match.Entry.Key,
		})
	}
}

func twirpFinalize(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := authorize(d, r)
		if err != nil {
			writeTwirpErr(w, http.StatusUnauthorized, "unauthenticated", err.Error())
			return
		}
		var body twirpKeyVersion
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" || body.Version == "" {
			writeTwirpErr(w, http.StatusBadRequest, "invalid_argument", "key and version required")
			return
		}
		write, ok := auth.WriteScope(res.Scopes)
		if !ok {
			writeTwirpErr(w, http.StatusForbidden, "permission_denied", "no scope with write permission")
			return
		}
		u, err := d.Storage.CompleteUpload(r.Context(), body.Key, body.Version, write.Scope, res.RepoID)
		if err != nil {
			writeTwirpErr(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if u == nil {
			writeTwirpErr(w, http.StatusNotFound, "not_found", "upload not found")
			return
		}
		if errors.Is(err, nil) {
			// fall through
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entry_id": strconvFormatInt(u.ID)})
	}
}
```

Add a tiny helper file `internal/server/util.go`:

```go
package server

import (
	"strconv"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/auth"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/storage"
)

func strconvFormatInt(n int64) string { return strconv.FormatInt(n, 10) }

func storageMatchInput(keys []string, version string, res *auth.AuthResult) storage.MatchInput {
	return storage.MatchInput{
		Keys:    keys,
		Version: version,
		Scopes:  auth.ScopesByPermissionDesc(res.Scopes),
		RepoID:  res.RepoID,
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server
git commit -m "feat(server): twirp v2 cache endpoints (Create, Get, Finalize)"
```

---

## Phase 10 — Azure-style Upload Endpoint

The runner uploads chunks to `PUT /devstoreaccount1/upload/{uploadId}?blockid={base64}` (one PUT per chunk), then commits with `PUT /devstoreaccount1/upload/{uploadId}?comp=blocklist` (returns 201, body ignored).

Two block-id encodings exist:
- 64-byte decoded: docker buildx — chunk index lives at bytes 16..19 (big-endian uint32).
- 48-byte decoded: everything else — `<36-char UUID><integer string>`.

We must also set `x-ms-request-id` to a UUID on every response (works around an EOF bug in `tonistiigi/go-actions-cache`).

### Task 10.1: Block-id parser (unit-tested)

**Files:**
- Create: `internal/server/blockid.go`
- Create: `internal/server/blockid_test.go`

- [ ] **Step 1: Write the failing test**

```go
package server

import (
	"encoding/base64"
	"encoding/binary"
	"testing"
)

func TestBlockID_48Byte(t *testing.T) {
	raw := []byte("00000000-0000-0000-0000-000000000000" + "00000000017")
	if len(raw) != 47 {
		t.Fatalf("setup: %d", len(raw))
	}
	raw = append(raw, '0')
	id := base64.StdEncoding.EncodeToString(raw)
	got, ok := chunkIndexFromBlockID(id)
	if !ok || got != 170 {
		t.Errorf("got %d ok=%v want 170", got, ok)
	}
}

func TestBlockID_64Byte(t *testing.T) {
	b := make([]byte, 64)
	binary.BigEndian.PutUint32(b[16:20], 7)
	id := base64.StdEncoding.EncodeToString(b)
	got, ok := chunkIndexFromBlockID(id)
	if !ok || got != 7 {
		t.Errorf("got %d ok=%v", got, ok)
	}
}

func TestBlockID_BadLength(t *testing.T) {
	id := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, ok := chunkIndexFromBlockID(id); ok {
		t.Error("expected !ok")
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/server/...`
Expected: build error.

- [ ] **Step 3: Implement**

```go
package server

import (
	"encoding/base64"
	"encoding/binary"
	"strconv"
)

func chunkIndexFromBlockID(b64 string) (int, bool) {
	dec, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return 0, false
	}
	switch len(dec) {
	case 64:
		return int(binary.BigEndian.Uint32(dec[16:20])), true
	case 48:
		s := string(dec)
		if len(s) <= 36 {
			return 0, false
		}
		n, err := strconv.Atoi(s[36:])
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/blockid.go internal/server/blockid_test.go
git commit -m "feat(server): block-id parser supporting 48 and 64 byte encodings"
```

---

### Task 10.2: Upload handler

**Files:**
- Create: `internal/server/upload.go`
- Test: `internal/server/upload_test.go`

- [ ] **Step 1: Write the failing test**

```go
package server

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestUpload_PutBlockAndCommit(t *testing.T) {
	srv, svc, _ := newTestServer(t)
	u, _ := svc.CreateUpload(context.Background(), "k", "v", "main", "42")

	b := make([]byte, 64)
	binary.BigEndian.PutUint32(b[16:20], 0)
	blockID := base64.StdEncoding.EncodeToString(b)

	url := srv.URL + "/devstoreaccount1/upload/" + strconv.FormatInt(u.ID, 10) + "?blockid=" + blockID
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader("payload-zero"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if resp.Header.Get("x-ms-request-id") == "" {
		t.Error("missing x-ms-request-id")
	}

	commitURL := srv.URL + "/devstoreaccount1/upload/" + strconv.FormatInt(u.ID, 10) + "?comp=blocklist"
	req, _ = http.NewRequest(http.MethodPut, commitURL, strings.NewReader(""))
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("commit status=%d", resp.StatusCode)
	}
}

func TestUpload_AliasPath(t *testing.T) {
	srv, svc, _ := newTestServer(t)
	u, _ := svc.CreateUpload(context.Background(), "k", "v", "main", "42")
	url := srv.URL + "/upload/" + strconv.FormatInt(u.ID, 10) + "?comp=blocklist"
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader(""))
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/server/...`
Expected: failure.

- [ ] **Step 3: Implement & remove `registerUpload` stub**

Delete the stub from `server.go`. Write `internal/server/upload.go`:

```go
package server

import (
	"net/http"
	"strconv"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/ids"
)

func registerUpload(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("PUT /devstoreaccount1/upload/{uploadId}", uploadHandler(d))
	mux.HandleFunc("PUT /upload/{uploadId}", uploadHandler(d))
}

func uploadHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-ms-request-id", ids.UUIDv4())

		uploadID, err := strconv.ParseInt(r.PathValue("uploadId"), 10, 64)
		if err != nil {
			http.Error(w, "invalid upload id", http.StatusBadRequest)
			return
		}

		q := r.URL.Query()
		if q.Get("comp") == "blocklist" {
			w.WriteHeader(http.StatusCreated)
			return
		}

		blockID := q.Get("blockid")
		index := 0
		if blockID != "" {
			n, ok := chunkIndexFromBlockID(blockID)
			if !ok {
				http.Error(w, "invalid block id", http.StatusBadRequest)
				return
			}
			index = n
		}

		if err := d.Storage.UploadPart(r.Context(), uploadID, index, r.Body); err != nil {
			d.Logger.Error("uploadPart", "err", err)
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/upload.go internal/server/upload_test.go internal/server/server.go
git commit -m "feat(server): Azure-style block upload at /devstoreaccount1/upload/{id}"
```

---

## Phase 11 — Direct Download Endpoint

### Task 11.1: `/download/{cacheEntryId}`

**Files:**
- Create: `internal/server/download.go`
- Test: `internal/server/download_test.go`

- [ ] **Step 1: Write the failing test**

```go
package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDownload_StreamsCacheEntry(t *testing.T) {
	srv, svc, _ := newTestServer(t)
	ctx := context.Background()
	u, _ := svc.CreateUpload(ctx, "k", "v", "main", "42")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("hello "))
	_ = svc.UploadPart(ctx, u.ID, 1, strings.NewReader("world"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "main", "42")

	// look up entry id
	rows, err := svc.Match(ctx, "k", "v", "main", "42")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/download/" + rows.Entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "hello world" {
		t.Errorf("status=%d body=%q", resp.StatusCode, body)
	}
}

func TestDownload_NotFound(t *testing.T) {
	srv, _, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/download/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}
```

> Note: this test references `svc.Match`, a thin convenience wrapper. Add it.

- [ ] **Step 2: Add convenience method to `Service`**

In `internal/storage/service.go`, append:

```go
func (s *Service) Match(ctx context.Context, key, version, scope, repoID string) (*Match, error) {
	return s.MatchCacheEntry(ctx, MatchInput{
		Keys: []string{key}, Version: version, Scopes: []string{scope}, RepoID: repoID,
	})
}
```

- [ ] **Step 3: Implement download handler & remove stub**

Delete `func registerDownload(_ *http.ServeMux, _ Deps) {}` from `server.go`. Write `internal/server/download.go`:

```go
package server

import (
	"io"
	"net/http"
)

func registerDownload(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("GET /download/{cacheEntryId}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("cacheEntryId")
		stream, err := d.Storage.Download(r.Context(), id)
		if err != nil {
			d.Logger.Error("download", "err", err)
			http.Error(w, "download failed", http.StatusInternalServerError)
			return
		}
		if stream == nil {
			http.Error(w, "cache file not found", http.StatusNotFound)
			return
		}
		defer stream.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, stream)
	})
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/download.go internal/server/download_test.go internal/server/server.go internal/storage/service.go
git commit -m "feat(server): direct streaming download endpoint"
```

---

## Phase 12 — Cron Scheduler & Cleanup Tasks

The TS uses Nitro's cron strings:
- `*/5 * * * *` → cleanup:uploads
- `0 0 * * *`   → cleanup:cache-entries, cleanup:storage-locations
- `0 * * * *`   → cleanup:parts, cleanup:merges

These three patterns map cleanly to fixed intervals. We don't need a general cron parser.

### Task 12.1: Scheduler

**Files:**
- Create: `internal/cron/cron.go`
- Test: `internal/cron/cron_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerFiresAndStops(t *testing.T) {
	var n int32
	s := New()
	s.Every(50*time.Millisecond, "tick", func(_ context.Context) error {
		atomic.AddInt32(&n, 1)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	s.Wait()
	if got := atomic.LoadInt32(&n); got < 2 {
		t.Errorf("fired %d times, want >= 2", got)
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/cron/...`
Expected: build error.

- [ ] **Step 3: Implement**

```go
package cron

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Job struct {
	Name     string
	Interval time.Duration
	Fn       func(context.Context) error
}

type Scheduler struct {
	jobs []Job
	wg   sync.WaitGroup
	log  *slog.Logger
}

func New() *Scheduler { return &Scheduler{log: slog.Default()} }

func (s *Scheduler) WithLogger(l *slog.Logger) *Scheduler {
	s.log = l
	return s
}

func (s *Scheduler) Every(d time.Duration, name string, fn func(context.Context) error) {
	s.jobs = append(s.jobs, Job{Name: name, Interval: d, Fn: fn})
}

func (s *Scheduler) Run(ctx context.Context) {
	for _, j := range s.jobs {
		s.wg.Add(1)
		go s.runJob(ctx, j)
	}
}

func (s *Scheduler) Wait() { s.wg.Wait() }

func (s *Scheduler) runJob(ctx context.Context, j Job) {
	defer s.wg.Done()
	t := time.NewTicker(j.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			func() {
				cctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
				defer cancel()
				if err := j.Fn(cctx); err != nil {
					s.log.Error("scheduled task failed", "name", j.Name, "err", err)
				}
			}()
		}
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/cron/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cron
git commit -m "feat(cron): minimal interval-based scheduler"
```

---

### Task 12.2: Cleanup tasks

**Files:**
- Create: `internal/tasks/tasks.go`
- Create: `internal/tasks/cleanup_uploads.go`
- Create: `internal/tasks/cleanup_cache_entries.go`
- Create: `internal/tasks/cleanup_storage_locations.go`
- Create: `internal/tasks/cleanup_parts.go`
- Create: `internal/tasks/cleanup_merges.go`
- Test: `internal/tasks/cleanup_uploads_test.go`

- [ ] **Step 1: Write `tasks.go`**

```go
package tasks

import (
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
	dbpkg "github.com/falcondev-oss/github-actions-cache-server-go/internal/db"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/storage"
)

type Deps struct {
	Cfg     *config.Config
	Queries *dbpkg.Queries
	Storage *storage.Service
}

const pageSize = 10
```

- [ ] **Step 2: Write the failing test for cleanup_uploads**

```go
package tasks

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
	dbpkg "github.com/falcondev-oss/github-actions-cache-server-go/internal/db"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/storage"
	_ "modernc.org/sqlite"
)

func newTestDeps(t *testing.T) Deps {
	t.Helper()
	raw, _ := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	t.Cleanup(func() { raw.Close() })
	raw.SetMaxOpenConns(1)
	d := &dbpkg.DB{DB: raw, Driver: dbpkg.SQLite}
	if err := dbpkg.Migrate(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	q := dbpkg.New(d)
	a, _ := storage.NewFilesystemAdapter(t.TempDir())
	return Deps{
		Cfg:     &config.Config{CacheCleanupOlderThanDays: 90},
		Queries: q,
		Storage: storage.NewService(q, a, storage.ServiceConfig{}),
	}
}

func TestCleanupUploads_RemovesStale(t *testing.T) {
	ctx := context.Background()
	d := newTestDeps(t)
	old := time.Now().Add(-2 * time.Minute).UnixMilli()
	u, _ := d.Storage.CreateUpload(ctx, "k", "v", "s", "r")
	_ = d.Storage.UploadPart(ctx, u.ID, 0, strings.NewReader("x"))
	_ = d.Queries.SetUploadCreatedAt(ctx, u.ID, old)

	if err := CleanupUploads(d)(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := d.Queries.FindUploadByID(ctx, u.ID)
	if got != nil {
		t.Errorf("expected upload deleted, got %+v", got)
	}
}
```

> Note: this test calls a helper `SetUploadCreatedAt` we don't have yet. Add it to queries.

- [ ] **Step 3: Add helper to `internal/db/queries.go`**

```go
func (q *Queries) SetUploadCreatedAt(ctx context.Context, id, t int64) error {
	stmt := fmt.Sprintf(`UPDATE uploads SET createdAt=%s, lastPartUploadedAt=%s WHERE id=%s`,
		q.ph(1), q.ph(2), q.ph(3))
	_, err := q.d.ExecContext(ctx, stmt, t, t, id)
	return err
}
```

- [ ] **Step 4: Implement cleanup tasks**

`internal/tasks/cleanup_uploads.go`:

```go
package tasks

import (
	"context"
	"time"
)

func CleanupUploads(d Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		if d.Cfg.DisableCleanupJobs {
			return nil
		}
		threshold := time.Now().Add(-time.Minute).UnixMilli()
		for page := 0; ; page++ {
			ups, err := d.Queries.ListStaleUploads(ctx, threshold, pageSize, page*pageSize)
			if err != nil {
				return err
			}
			for _, u := range ups {
				_ = d.Queries.DeleteUpload(ctx, u.ID)
				_ = d.Storage.Adapter().DeleteFolder(ctx, u.FolderName)
			}
			if len(ups) < pageSize {
				return nil
			}
		}
	}
}
```

`internal/tasks/cleanup_cache_entries.go`:

```go
package tasks

import (
	"context"
	"time"
)

func CleanupCacheEntries(d Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		if d.Cfg.DisableCleanupJobs {
			return nil
		}
		threshold := time.Now().Add(-time.Duration(d.Cfg.CacheCleanupOlderThanDays) * 24 * time.Hour).UnixMilli()
		for page := 0; ; page++ {
			locs, err := d.Queries.ListExpiredStorageLocations(ctx, threshold, pageSize, page*pageSize)
			if err != nil {
				return err
			}
			for _, l := range locs {
				_ = d.Queries.DeleteStorageLocation(ctx, l.ID)
				_ = d.Storage.Adapter().DeleteFolder(ctx, l.FolderName)
			}
			if len(locs) < pageSize {
				return nil
			}
		}
	}
}
```

`internal/tasks/cleanup_storage_locations.go`:

```go
package tasks

import "context"

func CleanupStorageLocations(d Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		if d.Cfg.DisableCleanupJobs {
			return nil
		}
		for page := 0; ; page++ {
			locs, err := d.Queries.ListOrphanedStorageLocations(ctx, pageSize, page*pageSize)
			if err != nil {
				return err
			}
			for _, l := range locs {
				_ = d.Queries.DeleteStorageLocation(ctx, l.ID)
				_ = d.Storage.Adapter().DeleteFolder(ctx, l.FolderName)
			}
			if len(locs) < pageSize {
				return nil
			}
		}
	}
}
```

`internal/tasks/cleanup_parts.go`:

```go
package tasks

import (
	"context"
	"fmt"
	"time"
)

func CleanupParts(d Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		if d.Cfg.DisableCleanupJobs {
			return nil
		}
		for page := 0; ; page++ {
			locs, err := d.Queries.ListMergedNotCleanedStorageLocations(ctx, pageSize, page*pageSize)
			if err != nil {
				return err
			}
			for _, l := range locs {
				if err := d.Queries.UpdateStoragePartsDeleted(ctx, l.ID, time.Now().UnixMilli()); err != nil {
					return err
				}
				if err := d.Storage.Adapter().DeleteFolder(ctx, fmt.Sprintf("%s/parts", l.FolderName)); err != nil {
					return err
				}
			}
			if len(locs) < pageSize {
				return nil
			}
		}
	}
}
```

`internal/tasks/cleanup_merges.go`:

```go
package tasks

import (
	"context"
	"time"
)

func CleanupMerges(d Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		if d.Cfg.DisableCleanupJobs {
			return nil
		}
		threshold := time.Now().Add(-15 * time.Minute).UnixMilli()
		_, err := d.Queries.ResetStalledMerges(ctx, threshold)
		return err
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tasks/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tasks internal/db/queries.go
git commit -m "feat(tasks): five cleanup tasks (uploads, entries, locations, parts, merges)"
```

---

## Phase 13 — Management API (REST)

The TS uses oRPC + OpenAPI; we expose a plain REST surface with the same shape. Auth: `X-Api-Key` header equals `MANAGEMENT_API_KEY`. Returns 503 if unset.

Routes:
- `GET /management-api/cache-entries/{id}`
- `GET /management-api/cache-entries` — list with filters
- `GET /management-api/cache-entries/match` — match by primary/restore keys
- `DELETE /management-api/cache-entries/{id}`
- `DELETE /management-api/cache-entries` — bulk delete by filters
- `GET /management-api/storage-locations/{id}`
- `DELETE /management-api/storage-locations/{id}`

### Task 13.1: List & filter helpers in queries

**Files:**
- Modify: `internal/db/queries.go`

- [ ] **Step 1: Add helpers**

```go
type CacheEntryFilter struct {
	Key, Version, Scope, RepoID string
}

func (q *Queries) ListCacheEntries(ctx context.Context, f CacheEntryFilter, limit, offset int) ([]CacheEntry, int, error) {
	conds, args := []string{"1=1"}, []any{}
	add := func(field, val string) {
		if val == "" {
			return
		}
		conds = append(conds, fmt.Sprintf(`%s=%s`, field, q.ph(len(args)+1)))
		args = append(args, val)
	}
	add(`"key"`, f.Key)
	add(`version`, f.Version)
	add(`scope`, f.Scope)
	add(`repoId`, f.RepoID)
	where := strings.Join(conds, " AND ")

	countStmt := `SELECT count(*) FROM cache_entries WHERE ` + where
	var total int
	if err := q.d.QueryRowContext(ctx, countStmt, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, updatedAt, locationId
		FROM cache_entries WHERE %s ORDER BY updatedAt DESC LIMIT %s OFFSET %s`,
		where, q.ph(len(args)+1), q.ph(len(args)+2))
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, limit, offset)
	rows, err := q.d.QueryContext(ctx, stmt, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []CacheEntry
	for rows.Next() {
		var e CacheEntry
		if err := rows.Scan(&e.ID, &e.Key, &e.Version, &e.Scope, &e.RepoID, &e.UpdatedAt, &e.LocationID); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (q *Queries) DeleteCacheEntries(ctx context.Context, f CacheEntryFilter) (int64, error) {
	conds, args := []string{"1=1"}, []any{}
	add := func(field, val string) {
		if val == "" {
			return
		}
		conds = append(conds, fmt.Sprintf(`%s=%s`, field, q.ph(len(args)+1)))
		args = append(args, val)
	}
	add(`"key"`, f.Key)
	add(`version`, f.Version)
	add(`scope`, f.Scope)
	add(`repoId`, f.RepoID)
	where := strings.Join(conds, " AND ")
	res, err := q.d.ExecContext(ctx, `DELETE FROM cache_entries WHERE `+where, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add internal/db/queries.go
git commit -m "feat(db): list/delete-many helpers for management API"
```

---

### Task 13.2: Management handlers

**Files:**
- Create: `internal/server/management.go`
- Test: `internal/server/management_test.go`

- [ ] **Step 1: Write the failing test**

```go
package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/auth"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
	dbpkg "github.com/falcondev-oss/github-actions-cache-server-go/internal/db"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/logging"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/storage"

	"database/sql"
	"net/http/httptest"
	_ "modernc.org/sqlite"
)

func newMgmtServer(t *testing.T, apiKey string) (*httptest.Server, *storage.Service) {
	t.Helper()
	raw, _ := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	t.Cleanup(func() { raw.Close() })
	raw.SetMaxOpenConns(1)
	d := &dbpkg.DB{DB: raw, Driver: dbpkg.SQLite}
	_ = dbpkg.Migrate(context.Background(), d)
	q := dbpkg.New(d)
	a, _ := storage.NewFilesystemAdapter(t.TempDir())
	cfg := &config.Config{APIBaseURL: "http://localhost:3000", ManagementAPIKey: apiKey}
	svc := storage.NewService(q, a, storage.ServiceConfig{})
	v := auth.NewVerifier(nil, "issuer", true)
	h := NewHandler(Deps{Cfg: cfg, Logger: logging.New(false), Storage: svc, Verifier: v})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, svc
}

func TestManagement_RequiresAPIKey(t *testing.T) {
	srv, _ := newMgmtServer(t, "secret")
	resp, _ := http.Get(srv.URL + "/management-api/cache-entries")
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("status=%d", resp.StatusCode)
	}
}

func TestManagement_503IfDisabled(t *testing.T) {
	srv, _ := newMgmtServer(t, "")
	resp, _ := http.Get(srv.URL + "/management-api/cache-entries")
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("status=%d", resp.StatusCode)
	}
}

func TestManagement_ListAndGet(t *testing.T) {
	srv, svc := newMgmtServer(t, "secret")
	ctx := context.Background()
	u, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("x"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "s", "r")

	req, _ := http.NewRequest("GET", srv.URL+"/management-api/cache-entries", nil)
	req.Header.Set("X-Api-Key", "secret")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var page struct {
		Total int                  `json:"total"`
		Items []map[string]any     `json:"items"`
	}
	_ = json.Unmarshal(body, &page)
	if page.Total != 1 || len(page.Items) != 1 {
		t.Errorf("got %+v", page)
	}
}
```

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/server/...`
Expected: failure.

- [ ] **Step 3: Implement & remove stub**

Delete `func registerManagement(_ *http.ServeMux, _ Deps) {}` from `server.go`.

Write `internal/server/management.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	dbpkg "github.com/falcondev-oss/github-actions-cache-server-go/internal/db"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/storage"
)

func registerManagement(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("GET /management-api/cache-entries/match", mgmtAuth(d, mgmtMatch(d)))
	mux.HandleFunc("GET /management-api/cache-entries/{id}", mgmtAuth(d, mgmtGetEntry(d)))
	mux.HandleFunc("GET /management-api/cache-entries", mgmtAuth(d, mgmtListEntries(d)))
	mux.HandleFunc("DELETE /management-api/cache-entries/{id}", mgmtAuth(d, mgmtDeleteEntry(d)))
	mux.HandleFunc("DELETE /management-api/cache-entries", mgmtAuth(d, mgmtDeleteEntries(d)))
	mux.HandleFunc("GET /management-api/storage-locations/{id}", mgmtAuth(d, mgmtGetLocation(d)))
	mux.HandleFunc("DELETE /management-api/storage-locations/{id}", mgmtAuth(d, mgmtDeleteLocation(d)))
}

func mgmtAuth(d Deps, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Cfg.ManagementAPIKey == "" {
			http.Error(w, "management api disabled", http.StatusServiceUnavailable)
			return
		}
		if r.Header.Get("X-Api-Key") != d.Cfg.ManagementAPIKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func mgmtGetEntry(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := dbpkg.New(d.Storage.Q()).GetCacheEntry(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if e == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Cache entry not found"})
			return
		}
		writeJSON(w, http.StatusOK, e)
	}
}

func mgmtListEntries(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 {
			page = 1
		}
		ipp, _ := strconv.Atoi(q.Get("itemsPerPage"))
		if ipp < 1 || ipp > 100 {
			ipp = 20
		}
		f := dbpkg.CacheEntryFilter{
			Key: q.Get("key"), Version: q.Get("version"),
			Scope: q.Get("scope"), RepoID: q.Get("repoId"),
		}
		items, total, err := dbpkg.New(d.Storage.Q()).ListCacheEntries(r.Context(), f, ipp, (page-1)*ipp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []dbpkg.CacheEntry{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"total": total, "items": items})
	}
}

func mgmtDeleteEntry(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := dbpkg.New(d.Storage.Q()).DeleteCacheEntry(r.Context(), r.PathValue("id")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func mgmtDeleteEntries(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f := dbpkg.CacheEntryFilter{
			Key: q.Get("key"), Version: q.Get("version"),
			Scope: q.Get("scope"), RepoID: q.Get("repoId"),
		}
		n, err := dbpkg.New(d.Storage.Q()).DeleteCacheEntries(r.Context(), f)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"deleted": n})
	}
}

func mgmtGetLocation(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		loc, err := dbpkg.New(d.Storage.Q()).GetStorageLocation(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if loc == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Storage location not found"})
			return
		}
		writeJSON(w, http.StatusOK, loc)
	}
}

func mgmtDeleteLocation(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		loc, err := dbpkg.New(d.Storage.Q()).GetStorageLocation(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if loc == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := dbpkg.New(d.Storage.Q()).DeleteStorageLocation(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = d.Storage.Adapter().DeleteFolder(r.Context(), loc.FolderName)
		w.WriteHeader(http.StatusNoContent)
	}
}

type matchResult struct {
	Match dbpkg.CacheEntry  `json:"match"`
	Type  storage.MatchType `json:"type"`
}

func mgmtMatch(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		primary := q.Get("primaryKey")
		version := q.Get("version")
		repoID := q.Get("repoId")
		scopes := q["scopes"]
		restore := q["restoreKeys"]
		if primary == "" || version == "" || repoID == "" || len(scopes) == 0 {
			http.Error(w, "primaryKey, version, repoId, scopes required", http.StatusBadRequest)
			return
		}
		m, err := d.Storage.MatchCacheEntry(r.Context(), storage.MatchInput{
			Keys: append([]string{primary}, restore...), Version: version, Scopes: scopes, RepoID: repoID,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if m == nil {
			writeJSON(w, http.StatusOK, nil)
			return
		}
		writeJSON(w, http.StatusOK, matchResult{Match: m.Entry, Type: m.Type})
	}
}
```

- [ ] **Step 4: Add `Q()` accessor to Service**

In `internal/storage/service.go`:

```go
func (s *Service) Q() *dbpkg.DB {
	return s.q.DB()
}
```

And in `internal/db/queries.go`:

```go
func (q *Queries) DB() *DB { return q.d }
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/server/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/management.go internal/server/management_test.go internal/server/server.go internal/storage/service.go internal/db/queries.go
git commit -m "feat(server): management REST API"
```

---

## Phase 14 — Catch-all proxy

Unknown paths get reverse-proxied to `DEFAULT_ACTIONS_RESULTS_URL`. This is what allows the runner client to talk through us for endpoints we don't implement.

### Task 14.1: Reverse-proxy

**Files:**
- Create: `internal/server/proxy.go`

- [ ] **Step 1: Implement & remove stub**

Delete `func registerProxy(_ *http.ServeMux, _ Deps) {}` from `server.go`.

Write `internal/server/proxy.go`:

```go
package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

func registerProxy(mux *http.ServeMux, d Deps) {
	if d.Cfg.DefaultActionsResultsURL == "" {
		mux.HandleFunc("/", http.NotFound)
		return
	}
	target, err := url.Parse(d.Cfg.DefaultActionsResultsURL)
	if err != nil {
		mux.HandleFunc("/", http.NotFound)
		return
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	originalDirector := rp.Director
	rp.Director = func(r *http.Request) {
		originalDirector(r)
		r.Host = target.Host
	}
	mux.Handle("/", rp)
}
```

> Note: this catch-all matches anything not matched by other patterns. With Go 1.22 ServeMux precedence, the more-specific routes (`POST /twirp/...`, etc.) take priority.

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add internal/server/proxy.go internal/server/server.go
git commit -m "feat(server): catch-all reverse proxy to default actions results URL"
```

---

## Phase 15 — Composition Root

### Task 15.1: Wire it all in `cmd/cache-server/main.go`

**Files:**
- Modify: `cmd/cache-server/main.go`

- [ ] **Step 1: Replace placeholder with real wiring**

```go
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/auth"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/cron"
	dbpkg "github.com/falcondev-oss/github-actions-cache-server-go/internal/db"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/logging"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/server"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/storage"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/tasks"
)

func main() {
	cfg, err := config.Load(nil)
	if err != nil {
		panic(err)
	}
	logger := logging.New(cfg.Debug)

	d, err := dbpkg.Open(cfg)
	if err != nil {
		logger.Error("db open", "err", err)
		os.Exit(1)
	}
	defer d.Close()
	if err := dbpkg.Migrate(context.Background(), d); err != nil {
		logger.Error("migrate", "err", err)
		os.Exit(1)
	}
	q := dbpkg.New(d)

	adapter, err := buildStorageAdapter(cfg)
	if err != nil {
		logger.Error("storage init", "err", err)
		os.Exit(1)
	}
	svc := storage.NewService(q, adapter, storage.ServiceConfig{
		APIBaseURL:            cfg.APIBaseURL,
		EnableDirectDownloads: cfg.EnableDirectDownloads,
		Logger:                logger,
	})

	verifier := auth.NewVerifier(
		auth.NewJWKSFetcher("https://token.actions.githubusercontent.com/.well-known/jwks"),
		"https://token.actions.githubusercontent.com",
		cfg.SkipTokenValidation,
	)

	handler := server.NewHandler(server.Deps{
		Cfg: cfg, Logger: logger, Storage: svc, Verifier: verifier,
	})

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 30 * time.Second,
	}

	taskDeps := tasks.Deps{Cfg: cfg, Queries: q, Storage: svc}
	scheduler := cron.New().WithLogger(logger)
	scheduler.Every(5*time.Minute, "cleanup:uploads", tasks.CleanupUploads(taskDeps))
	scheduler.Every(time.Hour, "cleanup:parts", tasks.CleanupParts(taskDeps))
	scheduler.Every(time.Hour, "cleanup:merges", tasks.CleanupMerges(taskDeps))
	scheduler.Every(24*time.Hour, "cleanup:cache-entries", tasks.CleanupCacheEntries(taskDeps))
	scheduler.Every(24*time.Hour, "cleanup:storage-locations", tasks.CleanupStorageLocations(taskDeps))

	rootCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	scheduler.Run(rootCtx)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("listening", "addr", cfg.ListenAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen", "err", err)
		}
	}()

	<-rootCtx.Done()
	logger.Info("shutting down")
	shutCtx, sc := context.WithTimeout(context.Background(), 30*time.Second)
	defer sc()
	_ = httpSrv.Shutdown(shutCtx)
	svc.WaitForOngoingMerges(shutCtx)
	scheduler.Wait()
	wg.Wait()
}

func buildStorageAdapter(cfg *config.Config) (storage.Adapter, error) {
	switch cfg.StorageDriver {
	case "filesystem":
		return storage.NewFilesystemAdapter(cfg.StorageFilesystemPath)
	case "s3":
		return storage.NewS3Adapter(cfg)
	case "gcs":
		return storage.NewGCSAdapter(cfg)
	}
	return nil, errors.New("unknown storage driver")
}
```

- [ ] **Step 2: Add stub adapters so the build passes** (real implementations come in next phases)

Append to `internal/storage/s3.go`:

```go
package storage

import (
	"errors"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
)

func NewS3Adapter(_ *config.Config) (Adapter, error) {
	return nil, errors.New("s3 adapter not yet implemented")
}
```

Append to `internal/storage/gcs.go`:

```go
package storage

import (
	"errors"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
)

func NewGCSAdapter(_ *config.Config) (Adapter, error) {
	return nil, errors.New("gcs adapter not yet implemented")
}
```

- [ ] **Step 3: Build and run smoke test**

```bash
go build -o cache-server ./cmd/cache-server
API_BASE_URL=http://localhost:3000 ./cache-server &
sleep 1
curl -sS http://localhost:3000/health
kill %1 || true
```

Expected: `healthy`.

- [ ] **Step 4: Commit**

```bash
git add cmd internal/storage/s3.go internal/storage/gcs.go
git commit -m "feat: composition root with HTTP server + cron + graceful shutdown"
```

---

## Phase 16 — S3 Adapter (stdlib SigV4)

S3 SigV4 over plain `net/http`. Operations needed: PUT object (streaming), GET object, DELETE objects (batch), LIST objects v2, HEAD bucket, presigned GET URL.

We use **single-PUT uploads** (not multipart), since cache parts are at most a few MB each. The streaming download of parts can therefore be a vanilla GET. For very large `merged` blobs we still single-PUT — for blobs >5 GB the user should keep `ENABLE_DIRECT_DOWNLOADS=false` and rely on the server proxying. (Multipart upload can be added later behind a flag without changing the interface.)

### Task 16.1: SigV4 signer

**Files:**
- Create: `internal/storage/sigv4.go`
- Test: `internal/storage/sigv4_test.go`

- [ ] **Step 1: Add a known-good fixture test**

Use the canonical AWS docs example: a GET request to `examplebucket.s3.amazonaws.com` with fixed timestamp and credentials, expecting the documented signature.

```go
package storage

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSigV4_GetObject(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	now, _ := time.Parse("20060102T150405Z", "20130524T000000Z")
	signSigV4(req, "us-east-1", "s3",
		"AKIAIOSFODNN7EXAMPLE",
		"wJalrXUtnEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"", now, hashEmpty)
	got := req.Header.Get("Authorization")
	if !strings.Contains(got, "Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41") {
		t.Errorf("got %q", got)
	}
}
```

(`hashEmpty` is the SHA-256 hex of the empty string; the function recomputes it but the test passes a zero-payload GET.)

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/storage/...`
Expected: failure.

- [ ] **Step 3: Implement SigV4**

```go
package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const hashEmpty = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func signSigV4(req *http.Request, region, service, accessKey, secretKey, sessionToken string, now time.Time, payloadHash string) {
	if payloadHash == "" {
		payloadHash = hashEmpty
	}
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", sessionToken)
	}
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.URL.Host)
	}

	canonHeaders, signedHeaders := canonicalHeaders(req)
	canonical := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQuery(req.URL),
		canonHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credScope,
		sha256Hex([]byte(canonical)),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	req.Header.Set("Authorization",
		fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
			accessKey, credScope, signedHeaders, signature))
}

func canonicalURI(u *url.URL) string {
	if u.Path == "" {
		return "/"
	}
	parts := strings.Split(u.Path, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func canonicalQuery(u *url.URL) string {
	if u.RawQuery == "" {
		return ""
	}
	q := u.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		vals := q[k]
		sort.Strings(vals)
		for j, v := range vals {
			if i > 0 || j > 0 {
				b.WriteByte('&')
			}
			b.WriteString(url.QueryEscape(k))
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(v))
		}
	}
	return b.String()
}

func canonicalHeaders(req *http.Request) (string, string) {
	hdrs := []string{}
	values := map[string]string{}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		hdrs = append(hdrs, lk)
		values[lk] = strings.Join(v, ",")
	}
	hdrs = append(hdrs, "host")
	values["host"] = req.Host
	if req.Host == "" {
		values["host"] = req.URL.Host
	}
	sort.Strings(hdrs)
	var canon strings.Builder
	for _, h := range hdrs {
		canon.WriteString(h)
		canon.WriteByte(':')
		canon.WriteString(strings.TrimSpace(values[h]))
		canon.WriteByte('\n')
	}
	return canon.String(), strings.Join(hdrs, ";")
}

func presignSigV4(req *http.Request, region, service, accessKey, secretKey string, now time.Time, expires time.Duration) string {
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")
	credScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)

	q := req.URL.Query()
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", accessKey+"/"+credScope)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", fmt.Sprintf("%d", int(expires.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Host", req.URL.Host)

	canonical := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQuery(req.URL),
		"host:" + req.URL.Host + "\n",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credScope,
		sha256Hex([]byte(canonical)),
	}, "\n")
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	sig := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))
	q.Set("X-Amz-Signature", sig)
	req.URL.RawQuery = q.Encode()
	return req.URL.String()
}

var _ = io.Reader(nil)
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/storage/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/sigv4.go internal/storage/sigv4_test.go
git commit -m "feat(storage): SigV4 request signer + presigner (stdlib only)"
```

---

### Task 16.2: S3 adapter

**Files:**
- Modify: `internal/storage/s3.go`
- Test: `internal/storage/s3_test.go` (uses `testcontainers-go` with MinIO)

- [ ] **Step 1: Add testcontainers**

Run:
```bash
go get github.com/testcontainers/testcontainers-go
```

- [ ] **Step 2: Write the failing test**

Write `internal/storage/s3_test.go`:

```go
//go:build integration

package storage

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestS3_RoundTrip(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "quay.io/minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin",
		},
		WaitingFor: wait.ForLog("API:"),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req, Started: true,
	})
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	defer c.Terminate(ctx)

	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "9000")
	endpoint := "http://" + host + ":" + port.Port()

	cfg := &config.Config{
		StorageDriver: "s3", S3Bucket: "vitest", AWSRegion: "us-east-1",
		AWSEndpointURL: endpoint, AWSAccessKeyID: "minioadmin", AWSSecretAccessKey: "minioadmin",
	}
	a, err := NewS3Adapter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.UploadStream(ctx, "test/parts/0", strings.NewReader("hello")); err != nil {
		t.Fatalf("upload: %v", err)
	}
	r, err := a.CreateDownloadStream(ctx, "test/parts/0")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	body, _ := io.ReadAll(r)
	if string(body) != "hello" {
		t.Errorf("got %q", body)
	}
}
```

- [ ] **Step 3: Implement S3 adapter**

Replace `internal/storage/s3.go`:

```go
package storage

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
)

type S3Adapter struct {
	bucket    string
	region    string
	endpoint  string
	keyID     string
	keySecret string
	prefix    string
	client    *http.Client
}

func NewS3Adapter(cfg *config.Config) (Adapter, error) {
	if cfg.S3Bucket == "" {
		return nil, errors.New("S3 bucket required")
	}
	endpoint := cfg.AWSEndpointURL
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.AWSRegion)
	}
	a := &S3Adapter{
		bucket:    cfg.S3Bucket,
		region:    cfg.AWSRegion,
		endpoint:  strings.TrimRight(endpoint, "/"),
		keyID:     cfg.AWSAccessKeyID,
		keySecret: cfg.AWSSecretAccessKey,
		prefix:    "gh-actions-cache",
		client:    &http.Client{Timeout: 0},
	}
	if err := a.headBucket(context.Background()); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *S3Adapter) url(key string) *url.URL {
	u, _ := url.Parse(a.endpoint + "/" + a.bucket + "/" + key)
	return u
}

func (a *S3Adapter) sign(req *http.Request, payloadHash string) {
	signSigV4(req, a.region, "s3", a.keyID, a.keySecret, "", time.Now(), payloadHash)
}

func (a *S3Adapter) do(req *http.Request) (*http.Response, error) {
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("s3 %s %s: %d %s", req.Method, req.URL.Path, resp.StatusCode, string(body))
	}
	return resp, nil
}

func (a *S3Adapter) headBucket(ctx context.Context) error {
	u, _ := url.Parse(a.endpoint + "/" + a.bucket)
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
	a.sign(req, hashEmpty)
	resp, err := a.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (a *S3Adapter) CreateDownloadStream(ctx context.Context, name string) (io.ReadCloser, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.url(a.prefix+"/"+name).String(), nil)
	a.sign(req, hashEmpty)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, ErrObjectNotFound
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("s3 get: %d %s", resp.StatusCode, body)
	}
	return resp.Body, nil
}

func (a *S3Adapter) UploadStream(ctx context.Context, name string, body io.Reader) error {
	buf, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, a.url(a.prefix+"/"+name).String(), bytes.NewReader(buf))
	req.ContentLength = int64(len(buf))
	a.sign(req, sha256Hex(buf))
	resp, err := a.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (a *S3Adapter) DeleteFolder(ctx context.Context, name string) error {
	return a.deleteByPrefix(ctx, a.prefix+"/"+name+"/")
}

func (a *S3Adapter) Clear(ctx context.Context) error {
	return a.deleteByPrefix(ctx, a.prefix+"/")
}

type listResult struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	Contents []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
}

func (a *S3Adapter) listObjects(ctx context.Context, prefix, token string) (*listResult, error) {
	u, _ := url.Parse(a.endpoint + "/" + a.bucket)
	q := u.Query()
	q.Set("list-type", "2")
	q.Set("prefix", prefix)
	if token != "" {
		q.Set("continuation-token", token)
	}
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	a.sign(req, hashEmpty)
	resp, err := a.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var lr listResult
	if err := xml.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, err
	}
	return &lr, nil
}

func (a *S3Adapter) deleteByPrefix(ctx context.Context, prefix string) error {
	token := ""
	for {
		lr, err := a.listObjects(ctx, prefix, token)
		if err != nil {
			return err
		}
		if len(lr.Contents) == 0 {
			return nil
		}
		var keys []string
		for _, c := range lr.Contents {
			keys = append(keys, c.Key)
		}
		if err := a.deleteObjects(ctx, keys); err != nil {
			return err
		}
		if !lr.IsTruncated {
			return nil
		}
		token = lr.NextContinuationToken
	}
}

func (a *S3Adapter) deleteObjects(ctx context.Context, keys []string) error {
	type obj struct {
		Key string `xml:"Key"`
	}
	type delete struct {
		XMLName xml.Name `xml:"Delete"`
		Object  []obj    `xml:"Object"`
		Quiet   bool     `xml:"Quiet"`
	}
	for i := 0; i < len(keys); i += 1000 {
		end := i + 1000
		if end > len(keys) {
			end = len(keys)
		}
		batch := delete{Quiet: true}
		for _, k := range keys[i:end] {
			batch.Object = append(batch.Object, obj{Key: k})
		}
		body, _ := xml.Marshal(batch)
		u, _ := url.Parse(a.endpoint + "/" + a.bucket + "?delete=")
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
		a.sign(req, sha256Hex(body))
		resp, err := a.do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (a *S3Adapter) CountFilesInFolder(ctx context.Context, name string) (int, error) {
	prefix := a.prefix + "/" + name + "/"
	count := 0
	token := ""
	for {
		lr, err := a.listObjects(ctx, prefix, token)
		if err != nil {
			return 0, err
		}
		count += len(lr.Contents)
		if !lr.IsTruncated {
			return count, nil
		}
		token = lr.NextContinuationToken
	}
}

func (a *S3Adapter) CreateDownloadURL(_ context.Context, name string) (string, bool, error) {
	u := a.url(a.prefix + "/" + name)
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	signed := presignSigV4(req, a.region, "s3", a.keyID, a.keySecret, time.Now(), 10*time.Minute)
	return signed, true, nil
}
```

- [ ] **Step 4: Run integration test**

Run: `go test -tags=integration -run TestS3_RoundTrip ./internal/storage/...`
Expected: PASS (or SKIP if no Docker).

- [ ] **Step 5: Commit**

```bash
git add internal/storage/s3.go internal/storage/s3_test.go go.mod go.sum
git commit -m "feat(storage): S3 adapter using stdlib net/http + SigV4"
```

---

## Phase 17 — GCS Adapter

We use a service-account-key JWT exchange for OAuth2 + the JSON API at `https://storage.googleapis.com/storage/v1/...`. For uploads, we use the simple media upload (`/upload/storage/v1/b/{bucket}/o?uploadType=media`); single-PUT, sufficient for cache parts.

Presigned URLs (V4 signing) for GCS use the same SigV4-style structure with `service=storage`, `region=auto`, signing with the SA key (RSA-PSS over `goog4_request`). For simplicity we keep `EnableDirectDownloads=false` for GCS in the first cut — the proxy will stream — and skip presigning. This is documented as a known limitation.

### Task 17.1: GCS adapter (skip presigning initially)

**Files:**
- Modify: `internal/storage/gcs.go`
- Test: `internal/storage/gcs_test.go` (uses `fsouza/fake-gcs-server`)

- [ ] **Step 1: Implement adapter**

```go
package storage

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
)

type GCSAdapter struct {
	bucket   string
	endpoint string
	prefix   string
	client   *http.Client

	creds   *gcsCreds
	tokenMu sync.Mutex
	token   string
	tokExp  time.Time
}

type gcsCreds struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

func NewGCSAdapter(cfg *config.Config) (Adapter, error) {
	if cfg.GCSBucket == "" {
		return nil, errors.New("GCS bucket required")
	}
	endpoint := cfg.GCSEndpoint
	if endpoint == "" {
		endpoint = "https://storage.googleapis.com"
	}
	a := &GCSAdapter{
		bucket:   cfg.GCSBucket,
		endpoint: strings.TrimRight(endpoint, "/"),
		prefix:   "gh-actions-cache",
		client:   &http.Client{},
	}
	if cfg.GCSServiceAccountKey != "" {
		raw, err := os.ReadFile(cfg.GCSServiceAccountKey)
		if err != nil {
			return nil, err
		}
		var c gcsCreds
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		a.creds = &c
	}
	if err := a.headBucket(context.Background()); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *GCSAdapter) authToken(ctx context.Context) (string, error) {
	if a.creds == nil {
		return "", nil
	}
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()
	if a.token != "" && time.Now().Before(a.tokExp.Add(-time.Minute)) {
		return a.token, nil
	}
	block, _ := pem.Decode([]byte(a.creds.PrivateKey))
	if block == nil {
		return "", errors.New("invalid private key")
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	pk, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return "", errors.New("not RSA private key")
	}
	header := `{"alg":"RS256","typ":"JWT"}`
	now := time.Now()
	claims := map[string]any{
		"iss":   a.creds.ClientEmail,
		"scope": "https://www.googleapis.com/auth/devstorage.read_write",
		"aud":   a.creds.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	cb, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding.EncodeToString
	signing := enc([]byte(header)) + "." + enc(cb)
	h := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(nil, pk, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	jwt := signing + "." + enc(sig)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", jwt)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.creds.TokenURI, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gcs token exchange: %d %s", resp.StatusCode, body)
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	a.token = t.AccessToken
	a.tokExp = now.Add(time.Duration(t.ExpiresIn) * time.Second)
	return a.token, nil
}

func (a *GCSAdapter) authReq(ctx context.Context, method, u string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	tok, err := a.authToken(ctx)
	if err != nil {
		return nil, err
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return req, nil
}

func (a *GCSAdapter) headBucket(ctx context.Context) error {
	u := a.endpoint + "/storage/v1/b/" + url.PathEscape(a.bucket)
	req, err := a.authReq(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("head bucket: %d %s", resp.StatusCode, body)
	}
	return nil
}

func (a *GCSAdapter) objectURL(name string) string {
	return a.endpoint + "/storage/v1/b/" + url.PathEscape(a.bucket) +
		"/o/" + url.PathEscape(a.prefix+"/"+name) + "?alt=media"
}

func (a *GCSAdapter) CreateDownloadStream(ctx context.Context, name string) (io.ReadCloser, error) {
	req, err := a.authReq(ctx, http.MethodGet, a.objectURL(name), nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, ErrObjectNotFound
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("gcs get: %d %s", resp.StatusCode, body)
	}
	return resp.Body, nil
}

func (a *GCSAdapter) UploadStream(ctx context.Context, name string, body io.Reader) error {
	uploadURL := a.endpoint + "/upload/storage/v1/b/" + url.PathEscape(a.bucket) +
		"/o?uploadType=media&name=" + url.QueryEscape(a.prefix+"/"+name)
	buf, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	req, err := a.authReq(ctx, http.MethodPost, uploadURL, strings.NewReader(string(buf)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(buf))
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gcs upload: %d %s", resp.StatusCode, b)
	}
	return nil
}

func (a *GCSAdapter) listPrefix(ctx context.Context, prefix string) ([]string, error) {
	pageToken := ""
	var keys []string
	for {
		u := a.endpoint + "/storage/v1/b/" + url.PathEscape(a.bucket) + "/o?prefix=" + url.QueryEscape(prefix)
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		req, err := a.authReq(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := a.client.Do(req)
		if err != nil {
			return nil, err
		}
		var page struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
			NextPageToken string `json:"nextPageToken"`
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			keys = append(keys, it.Name)
		}
		if page.NextPageToken == "" {
			return keys, nil
		}
		pageToken = page.NextPageToken
	}
}

func (a *GCSAdapter) deleteOne(ctx context.Context, key string) error {
	u := a.endpoint + "/storage/v1/b/" + url.PathEscape(a.bucket) + "/o/" + url.PathEscape(key)
	req, err := a.authReq(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gcs delete %s: %d %s", key, resp.StatusCode, b)
	}
	return nil
}

func (a *GCSAdapter) DeleteFolder(ctx context.Context, name string) error {
	keys, err := a.listPrefix(ctx, a.prefix+"/"+name+"/")
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := a.deleteOne(ctx, k); err != nil {
			return err
		}
	}
	return nil
}

func (a *GCSAdapter) Clear(ctx context.Context) error {
	keys, err := a.listPrefix(ctx, a.prefix+"/")
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := a.deleteOne(ctx, k); err != nil {
			return err
		}
	}
	return nil
}

func (a *GCSAdapter) CountFilesInFolder(ctx context.Context, name string) (int, error) {
	keys, err := a.listPrefix(ctx, a.prefix+"/"+name+"/")
	return len(keys), err
}

func (a *GCSAdapter) CreateDownloadURL(_ context.Context, _ string) (string, bool, error) {
	return "", false, nil
}
```

- [ ] **Step 2: Write integration test**

```go
//go:build integration

package storage

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestGCS_RoundTrip(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "fsouza/fake-gcs-server:latest",
		ExposedPorts: []string{"4443/tcp"},
		Cmd:          []string{"-scheme", "http", "-port", "4443", "-public-host", "localhost"},
		WaitingFor:   wait.ForLog("server started"),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req, Started: true,
	})
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	defer c.Terminate(ctx)

	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "4443")
	endpoint := "http://" + host + ":" + port.Port()

	cfg := &config.Config{StorageDriver: "gcs", GCSBucket: "vitest", GCSEndpoint: endpoint}
	a, err := NewGCSAdapter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.UploadStream(ctx, "test/parts/0", strings.NewReader("hello")); err != nil {
		t.Fatalf("upload: %v", err)
	}
	r, err := a.CreateDownloadStream(ctx, "test/parts/0")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	body, _ := io.ReadAll(r)
	if string(body) != "hello" {
		t.Errorf("got %q", body)
	}
}
```

- [ ] **Step 3: Run**

Run: `go test -tags=integration -run TestGCS_RoundTrip ./internal/storage/...`
Expected: PASS or SKIP (no Docker).

- [ ] **Step 4: Commit**

```bash
git add internal/storage/gcs.go internal/storage/gcs_test.go
git commit -m "feat(storage): GCS adapter via stdlib net/http + RS256 SA JWT"
```

---

## Phase 18 — End-to-End Test

Use the real `actions/cache` library? That requires Node. A fully Go-only e2e: drive the server via raw HTTP using the same call sequence the client uses.

### Task 18.1: E2E test

**Files:**
- Create: `tests/e2e/e2e_test.go`

- [ ] **Step 1: Write the test**

```go
package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/falcondev-oss/github-actions-cache-server-go/internal/auth"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/config"
	dbpkg "github.com/falcondev-oss/github-actions-cache-server-go/internal/db"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/logging"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/server"
	"github.com/falcondev-oss/github-actions-cache-server-go/internal/storage"
	_ "modernc.org/sqlite"
)

func b64url(b []byte) string { return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=") }

func unsignedToken(scopes []auth.Scope, repoID string) string {
	header := []byte(`{"alg":"none","typ":"JWT"}`)
	ac, _ := json.Marshal(scopes)
	payload, _ := json.Marshal(map[string]any{"ac": string(ac), "repository_id": repoID})
	return b64url(header) + "." + b64url(payload) + "."
}

func TestE2E_Roundtrip(t *testing.T) {
	ctx := context.Background()
	raw, _ := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	defer raw.Close()
	raw.SetMaxOpenConns(1)
	d := &dbpkg.DB{DB: raw, Driver: dbpkg.SQLite}
	_ = dbpkg.Migrate(ctx, d)
	q := dbpkg.New(d)
	a, _ := storage.NewFilesystemAdapter(t.TempDir())
	cfg := &config.Config{APIBaseURL: "http://localhost:3000", SkipTokenValidation: true}
	svc := storage.NewService(q, a, storage.ServiceConfig{APIBaseURL: cfg.APIBaseURL})
	v := auth.NewVerifier(nil, "issuer", true)
	h := server.NewHandler(server.Deps{Cfg: cfg, Logger: logging.New(false), Storage: svc, Verifier: v})
	srv := httptest.NewServer(h)
	defer srv.Close()
	cfg.APIBaseURL = srv.URL
	tok := unsignedToken([]auth.Scope{{Scope: "main", Permission: 3}}, "42")

	bodyJSON, _ := json.Marshal(map[string]any{"key": "k1", "version": "v1"})
	req, _ := http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ := http.DefaultClient.Do(req)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("create status=%d body=%s", resp.StatusCode, body)
	}
	var createOut struct {
		OK             bool   `json:"ok"`
		SignedUploadURL string `json:"signed_upload_url"`
	}
	_ = json.Unmarshal(body, &createOut)
	if !createOut.OK {
		t.Fatalf("create not ok: %s", body)
	}

	payload := bytes.Repeat([]byte("AB"), 512)
	blockBytes := make([]byte, 64)
	binary.BigEndian.PutUint32(blockBytes[16:20], 0)
	blockID := base64.StdEncoding.EncodeToString(blockBytes)
	chunkURL := createOut.SignedUploadURL + "?blockid=" + blockID
	req, _ = http.NewRequest("PUT", chunkURL, bytes.NewReader(payload))
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("upload chunk status=%d", resp.StatusCode)
	}
	req, _ = http.NewRequest("PUT", createOut.SignedUploadURL+"?comp=blocklist", bytes.NewReader(nil))
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("commit blocklist status=%d", resp.StatusCode)
	}

	bodyJSON, _ = json.Marshal(map[string]any{"key": "k1", "version": "v1"})
	req, _ = http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/FinalizeCacheEntryUpload",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ = http.DefaultClient.Do(req)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("finalize status=%d body=%s", resp.StatusCode, body)
	}
	var finOut struct {
		OK      bool   `json:"ok"`
		EntryID string `json:"entry_id"`
	}
	_ = json.Unmarshal(body, &finOut)
	if !finOut.OK {
		t.Fatalf("finalize not ok: %s", body)
	}
	if _, err := strconv.ParseInt(finOut.EntryID, 10, 64); err != nil {
		t.Errorf("entry_id not numeric: %s", finOut.EntryID)
	}

	bodyJSON, _ = json.Marshal(map[string]any{"key": "k1", "version": "v1"})
	req, _ = http.NewRequest("POST",
		srv.URL+"/twirp/github.actions.results.api.v1.CacheService/GetCacheEntryDownloadURL",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, _ = http.DefaultClient.Do(req)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	var getOut struct {
		OK                bool   `json:"ok"`
		SignedDownloadURL string `json:"signed_download_url"`
		MatchedKey        string `json:"matched_key"`
	}
	_ = json.Unmarshal(body, &getOut)
	if !getOut.OK || getOut.MatchedKey != "k1" {
		t.Fatalf("get not ok: %s", body)
	}

	resp, err := http.Get(getOut.SignedDownloadURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: %d bytes", len(got))
	}

	_ = rand.Reader
}
```

- [ ] **Step 2: Run**

Run: `go test ./tests/e2e/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add tests/e2e
git commit -m "test(e2e): full create/upload/finalize/download cycle"
```

---

## Phase 19 — Dockerfile, docker-compose, README

### Task 19.1: Multi-stage Dockerfile

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`

- [ ] **Step 1: Write `Dockerfile`**

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG BUILD_HASH
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.buildHash=${BUILD_HASH}" -o /out/cache-server ./cmd/cache-server

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /out/cache-server /cache-server
USER nonroot:nonroot
EXPOSE 3000
ENTRYPOINT ["/cache-server"]
```

- [ ] **Step 2: Write `docker-compose.yml`**

```yaml
services:
  cache-server:
    build: .
    ports:
      - "3000:3000"
    environment:
      API_BASE_URL: http://localhost:3000
      STORAGE_DRIVER: filesystem
      STORAGE_FILESYSTEM_PATH: /data/cache
      DB_DRIVER: sqlite
      DB_SQLITE_PATH: /data/cache-server.db
    volumes:
      - cache-data:/data
volumes:
  cache-data:
```

- [ ] **Step 3: Smoke test the image**

```bash
docker build -t gha-cache-go .
docker run --rm -d -p 3001:3000 -e API_BASE_URL=http://localhost:3001 --name gha-cache-go gha-cache-go
sleep 2
curl -fsS http://localhost:3001/health
docker rm -f gha-cache-go
```

Expected: `healthy`.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile docker-compose.yml
git commit -m "chore: distroless multi-stage Dockerfile + compose example"
```

---

### Task 19.2: README

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write the README**

```markdown
# GitHub Actions Cache Server (Go)

Go port of [falcondev-oss/github-actions-cache-server](https://github.com/falcondev-oss/github-actions-cache-server). Drop-in replacement for the official GitHub Actions cache that works with `actions/cache@v4`.

## Quick start

```yaml
services:
  cache-server:
    image: ghcr.io/your/gha-cache-go
    ports: ["3000:3000"]
    environment:
      API_BASE_URL: http://localhost:3000
      STORAGE_DRIVER: filesystem
      STORAGE_FILESYSTEM_PATH: /data/cache
      DB_DRIVER: sqlite
      DB_SQLITE_PATH: /data/cache-server.db
    volumes: [cache-data:/data]
volumes: { cache-data: }
```

In your workflow:

```yaml
env:
  ACTIONS_CACHE_URL: http://localhost:3000/
  ACTIONS_RESULTS_URL: http://localhost:3000/
```

## Configuration

| Variable | Default | Notes |
|---|---|---|
| `API_BASE_URL` | _required_ | Public base URL |
| `STORAGE_DRIVER` | `filesystem` | `filesystem` \| `s3` \| `gcs` |
| `STORAGE_FILESYSTEM_PATH` | `.data/storage/filesystem` | filesystem driver |
| `STORAGE_S3_BUCKET` | | required for s3 |
| `AWS_REGION` | `us-east-1` | s3 |
| `AWS_ENDPOINT_URL` | | s3 (MinIO etc.) |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | | s3 |
| `STORAGE_GCS_BUCKET` | | required for gcs |
| `STORAGE_GCS_SERVICE_ACCOUNT_KEY` | | path to SA key JSON |
| `STORAGE_GCS_ENDPOINT` | | gcs (fake-gcs-server etc.) |
| `DB_DRIVER` | `sqlite` | `sqlite` \| `postgres` \| `mysql` |
| `DB_SQLITE_PATH` | `.data/sqlite.db` | sqlite |
| `DB_POSTGRES_URL` | | postgres |
| `DB_POSTGRES_HOST/PORT/DATABASE/USER/PASSWORD` | | postgres |
| `DB_MYSQL_HOST/PORT/DATABASE/USER/PASSWORD` | | mysql |
| `CACHE_CLEANUP_OLDER_THAN_DAYS` | `90` | |
| `DISABLE_CLEANUP_JOBS` | `false` | |
| `ENABLE_DIRECT_DOWNLOADS` | `false` | use presigned URLs |
| `SKIP_TOKEN_VALIDATION` | `false` | dev only |
| `MANAGEMENT_API_KEY` | | enables `/management-api/*` |

## Architecture

- `cmd/cache-server` — entrypoint
- `internal/server` — HTTP handlers
- `internal/storage` — adapter interface + filesystem/S3/GCS
- `internal/db` — `database/sql` queries + migrations
- `internal/auth` — JWKS + JWT verification (RS256/ES256)
- `internal/cron` — interval scheduler
- `internal/tasks` — five cleanup tasks

## License

MIT
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README"
```

---

## Phase 20 — Verification

### Task 20.1: Final full-suite run

- [ ] **Step 1: Lint**

Run:
```bash
go vet ./...
gofmt -l .
```
Expected: no output.

- [ ] **Step 2: Unit tests**

Run: `go test -race ./...`
Expected: all PASS.

- [ ] **Step 3: Integration tests**

Run: `go test -race -tags=integration ./...`
Expected: all PASS (or SKIP if Docker absent).

- [ ] **Step 4: Build artifact**

Run:
```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o cache-server ./cmd/cache-server
ls -lh cache-server
```
Expected: a single static binary, ~15-25 MB.

- [ ] **Step 5: Smoke test against real `actions/cache` (optional, requires Node)**

```bash
./cache-server &
SERVER_PID=$!
sleep 1

# Use the upstream tests/e2e.test.ts logic via Node, pointing at our server.
# (Optional — the Go e2e test already exercises every code path.)

kill $SERVER_PID
```

- [ ] **Step 6: Commit a final tag**

```bash
git tag v0.1.0
git log --oneline | head -30
```

---

## Self-Review Checklist (run after the plan is complete)

- **Routes covered:**
  - `GET /health` — Phase 8
  - `GET /` — Phase 8
  - `POST /twirp/.../{Create,Get,Finalize}CacheEntry` — Phase 9
  - `PUT /devstoreaccount1/upload/{id}` — Phase 10
  - `PUT /upload/{id}` (alias) — Phase 10
  - `GET /download/{id}` — Phase 11
  - `*/management-api/*` — Phase 13
  - catch-all proxy — Phase 14

- **Storage drivers:**
  - filesystem — Phase 5
  - S3 — Phase 16
  - GCS — Phase 17

- **DB drivers:**
  - sqlite (modernc.org/sqlite, pure Go) — Phase 4
  - postgres (lib/pq) — Phase 4
  - mysql (go-sql-driver/mysql) — Phase 4

- **Cleanup tasks:**
  - cleanup:uploads — Phase 12
  - cleanup:cache-entries — Phase 12
  - cleanup:storage-locations — Phase 12
  - cleanup:parts — Phase 12
  - cleanup:merges — Phase 12

- **Auth:**
  - JWKS fetch + cache — Phase 7
  - RS256 + ES256 verify — Phase 7
  - Scope/permission extraction — Phase 7
  - SKIP_TOKEN_VALIDATION fallback — Phase 7

- **Streaming behavior:**
  - Upload-part streaming directly into adapter — Phase 6
  - Merge-on-first-download via `io.Pipe` + `io.MultiWriter` — Phase 6
  - Graceful shutdown waits for ongoing merges — Phase 15

- **Docker:**
  - Multi-stage distroless build — Phase 19
  - docker-compose example — Phase 19

---

## Execution Notes

The plan is divided into 20 phases with ~50 tasks. Phases are mostly sequential; only the cloud adapters (Phase 16/17) are independent of each other.

Estimated effort for one engineer comfortable with Go: **15-25 working days**, including integration testing and shaking out Twirp wire-format quirks (e.g., the `x-ms-request-id` header workaround, block-id encoding variants, snake_case JSON fields).

If you skip S3 + GCS adapters and ship filesystem-only, the timeline shrinks to **6-10 working days** and the binary still serves single-host self-hosted runners well.

---

## Phase 21 — App-level adjustments for chart compatibility

The upstream chart sets `PORT=3000` and points readers to `/management-api/_docs`. Two small adjustments make our binary drop-in compatible with the chart and the upstream docs.

### Task 21.1: Honour `PORT` env var in addition to `LISTEN_ADDR`

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Append the failing test**

```go
func TestLoad_PORTBeatsListenAddr(t *testing.T) {
	cfg, err := Load(envFunc(map[string]string{
		"API_BASE_URL": "http://localhost:3000",
		"PORT":         "8080",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", cfg.ListenAddr)
	}
}

func TestLoad_NITRO_PORTAlsoWorks(t *testing.T) {
	cfg, _ := Load(envFunc(map[string]string{
		"API_BASE_URL": "http://localhost:3000",
		"NITRO_PORT":   "9090",
	}))
	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want :9090", cfg.ListenAddr)
	}
}
```

- [ ] **Step 2: Run the test (expect failure)**

Run: `go test ./internal/config/...`
Expected: failures.

- [ ] **Step 3: Add port resolution**

In `internal/config/config.go`, after `c.ListenAddr = ":3000"` defaults are set, replace the `LISTEN_ADDR` block with:

```go
	if v := env("LISTEN_ADDR"); v != "" {
		c.ListenAddr = v
	} else if v := env("PORT"); v != "" {
		c.ListenAddr = ":" + v
	} else if v := env("NITRO_PORT"); v != "" {
		c.ListenAddr = ":" + v
	}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat(config): honour PORT and NITRO_PORT env vars"
```

---

### Task 21.2: Serve management API docs at `/management-api/_docs`

The TS uses oRPC's OpenAPI auto-generation. We serve a hand-rolled spec from a Go literal so there's no runtime overhead. Swagger UI is loaded from a CDN to keep the binary slim — easy to swap for embedded UI later.

**Files:**
- Create: `internal/server/openapi.go`
- Modify: `internal/server/management.go`
- Modify: `internal/server/management_test.go`

- [ ] **Step 1: Append the failing test**

```go
func TestManagement_DocsSpecJSON(t *testing.T) {
	srv, _ := newMgmtServer(t, "secret")
	resp, err := http.Get(srv.URL + "/management-api/_docs/spec.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var doc map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&doc)
	if doc["openapi"] != "3.1.0" {
		t.Errorf("openapi field = %v", doc["openapi"])
	}
}

func TestManagement_DocsHTML(t *testing.T) {
	srv, _ := newMgmtServer(t, "secret")
	resp, _ := http.Get(srv.URL + "/management-api/_docs")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !bytes.Contains(body, []byte("swagger-ui")) {
		t.Errorf("status=%d body has swagger-ui? %v", resp.StatusCode, bytes.Contains(body, []byte("swagger-ui")))
	}
}
```

(Add `bytes` to imports in the test file.)

- [ ] **Step 2: Run test (expect failure)**

Run: `go test ./internal/server/...`
Expected: failures.

- [ ] **Step 3: Implement**

Write `internal/server/openapi.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
)

func openAPISpec(apiBaseURL string) map[string]any {
	server := apiBaseURL + "/management-api"
	cacheEntry := map[string]any{
		"type": "object",
		"required": []string{"id", "key", "version", "scope", "repoId", "updatedAt", "locationId"},
		"properties": map[string]any{
			"id": map[string]any{"type": "string"}, "key": map[string]any{"type": "string"},
			"version": map[string]any{"type": "string"}, "scope": map[string]any{"type": "string"},
			"repoId": map[string]any{"type": "string"}, "updatedAt": map[string]any{"type": "integer"},
			"locationId": map[string]any{"type": "string"},
		},
	}
	storageLocation := map[string]any{
		"type": "object",
		"required": []string{"id", "folderName", "partCount"},
		"properties": map[string]any{
			"id": map[string]any{"type": "string"}, "folderName": map[string]any{"type": "string"},
			"partCount":        map[string]any{"type": "integer"},
			"mergeStartedAt":   map[string]any{"type": "integer", "nullable": true},
			"mergedAt":         map[string]any{"type": "integer", "nullable": true},
			"partsDeletedAt":   map[string]any{"type": "integer", "nullable": true},
			"lastDownloadedAt": map[string]any{"type": "integer", "nullable": true},
		},
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info":    map[string]any{"title": "Cache Server Management API", "version": "1.0.0"},
		"servers": []map[string]any{{"url": server}},
		"security": []map[string]any{{"apiKeyHeader": []any{}}},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"apiKeyHeader": map[string]any{"type": "apiKey", "in": "header", "name": "X-Api-Key"},
			},
			"schemas": map[string]any{
				"CacheEntry":      cacheEntry,
				"StorageLocation": storageLocation,
			},
		},
		"paths": map[string]any{
			"/cache-entries":         pathListDelete("CacheEntry"),
			"/cache-entries/{id}":    pathGetDelete("CacheEntry"),
			"/cache-entries/match":   pathMatch(),
			"/storage-locations/{id}": pathGetDelete("StorageLocation"),
		},
	}
}

func pathGetDelete(schema string) map[string]any {
	idParam := []any{map[string]any{
		"name": "id", "in": "path", "required": true,
		"schema": map[string]any{"type": "string"},
	}}
	ref := map[string]any{"$ref": "#/components/schemas/" + schema}
	jsonContent := map[string]any{"application/json": map[string]any{"schema": ref}}
	return map[string]any{
		"get": map[string]any{
			"summary":    "Get " + schema, "parameters": idParam,
			"responses":  map[string]any{"200": map[string]any{"description": "OK", "content": jsonContent}},
		},
		"delete": map[string]any{
			"summary": "Delete " + schema, "parameters": idParam,
			"responses": map[string]any{"204": map[string]any{"description": "Deleted"}},
		},
	}
}

func pathListDelete(schema string) map[string]any {
	filterParams := []any{
		queryParam("key"), queryParam("version"), queryParam("scope"), queryParam("repoId"),
	}
	ref := map[string]any{"$ref": "#/components/schemas/" + schema}
	return map[string]any{
		"get": map[string]any{
			"summary": "List " + schema + "s",
			"parameters": append(filterParams,
				map[string]any{"name": "page", "in": "query", "schema": map[string]any{"type": "integer", "default": 1}},
				map[string]any{"name": "itemsPerPage", "in": "query", "schema": map[string]any{"type": "integer", "default": 20}},
			),
			"responses": map[string]any{"200": map[string]any{
				"description": "OK",
				"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{
					"type": "object", "required": []string{"total", "items"},
					"properties": map[string]any{
						"total": map[string]any{"type": "integer"},
						"items": map[string]any{"type": "array", "items": ref},
					},
				}}},
			}},
		},
		"delete": map[string]any{
			"summary":    "Delete many " + schema + "s",
			"parameters": filterParams,
			"responses":  map[string]any{"200": map[string]any{"description": "OK"}},
		},
	}
}

func pathMatch() map[string]any {
	return map[string]any{"get": map[string]any{
		"summary": "Match cache entry",
		"parameters": []any{
			map[string]any{"name": "primaryKey", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "version", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "repoId", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "scopes", "in": "query", "required": true,
				"schema": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
			map[string]any{"name": "restoreKeys", "in": "query",
				"schema": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
		},
		"responses": map[string]any{"200": map[string]any{"description": "OK"}},
	}}
}

func queryParam(name string) map[string]any {
	return map[string]any{"name": name, "in": "query", "schema": map[string]any{"type": "string"}}
}

const swaggerUIHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Cache Server Management API</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
</head><body><div id="swagger-ui"></div>
<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>window.onload=()=>SwaggerUIBundle({url:"/management-api/_docs/spec.json",dom_id:"#swagger-ui"})</script>
</body></html>`

func mgmtDocsHTML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerUIHTML))
}

func mgmtDocsSpec(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAPISpec(d.Cfg.APIBaseURL))
	}
}
```

In `internal/server/management.go`, add inside `registerManagement` (above `mgmtAuth`-wrapped routes):

```go
	mux.HandleFunc("GET /management-api/_docs", mgmtDocsHTML)
	mux.HandleFunc("GET /management-api/_docs/spec.json", mgmtDocsSpec(d))
```

Note: docs endpoints intentionally are *not* gated by API key — Swagger UI being readable doesn't expose any data; the actual data endpoints still require the key.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/openapi.go internal/server/management.go internal/server/management_test.go
git commit -m "feat(server): OpenAPI spec + Swagger UI at /management-api/_docs"
```

---

## Phase 22 — Helm Chart

Mirror the upstream chart at `install/kubernetes/gha-cache-server/`. License: MIT (upstream); attribution noted in chart annotations.

### Task 22.1: `Chart.yaml` + `.helmignore`

**Files:**
- Create: `install/kubernetes/gha-cache-server/Chart.yaml`
- Create: `install/kubernetes/gha-cache-server/.helmignore`

- [ ] **Step 1: Write `Chart.yaml`**

```yaml
apiVersion: v2
name: gha-cache-server
description: Self-hosted GitHub Actions cache server (Go port)
type: application
version: 0.1.0
appVersion: "0.1.0"
home: https://github.com/falcondev-oss/github-actions-cache-server
sources:
  - https://github.com/falcondev-oss/github-actions-cache-server
keywords:
  - github-actions
  - cache
  - ci
maintainers:
  - name: Go port maintainers
annotations:
  category: CI/CD
  licenses: MIT
```

- [ ] **Step 2: Write `.helmignore`**

```
.DS_Store
.git/
.gitignore
.bzr/
.hg/
.svn/
*.swp
*.bak
*.tmp
*.orig
*~
.idea/
.vscode/
*.tmproj
```

- [ ] **Step 3: Commit**

```bash
git add install/kubernetes/gha-cache-server/Chart.yaml install/kubernetes/gha-cache-server/.helmignore
git commit -m "chore(helm): chart metadata and helmignore"
```

---

### Task 22.2: `values.yaml`

This is the user-facing configuration surface. Mirrors upstream keys 1:1 so existing operators can switch image and continue with their existing `values.yaml`.

**Files:**
- Create: `install/kubernetes/gha-cache-server/values.yaml`

- [ ] **Step 1: Write the file**

```yaml
replicaCount: 1

image:
  repository: ghcr.io/falcondev-oss/github-actions-cache-server-go
  pullPolicy: IfNotPresent
  tag: ""
  variant: ""

imagePullSecrets: []
nameOverride: ""
fullnameOverride: ""

config:
  apiBaseUrl: ""
  enableDirectDownloads: false
  cacheCleanupOlderThanDays: 90
  managementApiKey: ""

  storage:
    driver: filesystem
    filesystem:
      path: /data/cache
    s3: {}
    gcs: {}

  db:
    driver: sqlite
    sqlite:
      path: /data/cache-server.db
    postgres: {}
    mysql: {}

existingSecret: ""

serviceAccount:
  create: true
  automount: true
  annotations: {}
  name: ""

podAnnotations: {}
podLabels: {}

podSecurityContext:
  fsGroup: 1000

securityContext:
  capabilities:
    drop: [ALL]
  runAsNonRoot: true
  runAsUser: 1000
  readOnlyRootFilesystem: true

service:
  type: ClusterIP
  port: 3000
  annotations: {}

ingress:
  enabled: false
  className: ""
  annotations: {}
  hosts:
    - host: chart-example.local
      paths:
        - path: /
          pathType: ImplementationSpecific
  tls: []

resources:
  limits:
    memory: 1Gi
  requests:
    cpu: 100m
    memory: 128Mi

livenessProbe:
  httpGet:
    path: /health
    port: cache
readinessProbe:
  httpGet:
    path: /health
    port: cache

autoscaling:
  enabled: false
  minReplicas: 1
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 70

persistentVolumeClaim:
  enabled: null
  accessModes:
    - ReadWriteOnce
  storage: 20Gi
  annotations: {}
  labels: {}

nodeSelector: {}
tolerations: []
affinity: {}

extraEnv: []
extraEnvFrom: []
topologySpreadConstraints: []
extraVolumes: []
extraVolumeMounts: []
```

> Note: resources are right-sized down from the upstream (4Gi limit → 1Gi) since the Go binary is dramatically lighter than Node. `readOnlyRootFilesystem: true` is added because the Go binary needs no scratch space outside `/data`.

- [ ] **Step 2: Commit**

```bash
git add install/kubernetes/gha-cache-server/values.yaml
git commit -m "chore(helm): values.yaml with same surface as upstream"
```

---

### Task 22.3: `_helpers.tpl`

**Files:**
- Create: `install/kubernetes/gha-cache-server/templates/_helpers.tpl`

- [ ] **Step 1: Write the file**

```yaml
{{- define "ghacs.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "ghacs.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end }}

{{- define "ghacs.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "ghacs.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ghacs.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "ghacs.labels" -}}
helm.sh/chart: {{ include "ghacs.chart" . }}
{{ include "ghacs.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "ghacs.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "ghacs.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end }}

{{- define "ghacs.pvcName" -}}
{{ include "ghacs.fullname" . }}-data
{{- end }}

{{- define "ghacs.multipleReplicas" -}}
{{- if or (and .Values.autoscaling.enabled (gt (.Values.autoscaling.maxReplicas | int) 1)) (gt (.Values.replicaCount | int) 1) -}}
true
{{- else -}}
false
{{- end -}}
{{- end }}

{{- define "ghacs.pvcEnabled" -}}
{{- if kindIs "bool" .Values.persistentVolumeClaim.enabled -}}
{{- .Values.persistentVolumeClaim.enabled -}}
{{- else -}}
{{- or (eq .Values.config.storage.driver "filesystem") (eq .Values.config.db.driver "sqlite") -}}
{{- end -}}
{{- end }}

{{- define "ghacs.pvcAccessModes" -}}
{{- if and (eq (include "ghacs.multipleReplicas" .) "true") (eq .Values.config.storage.driver "filesystem") -}}
- ReadWriteMany
{{- else -}}
{{ toYaml .Values.persistentVolumeClaim.accessModes }}
{{- end -}}
{{- end }}

{{- define "ghacs.validate" -}}
{{- if and (eq .Values.config.db.driver "sqlite") (eq (include "ghacs.multipleReplicas" .) "true") -}}
{{- fail "SQLite cannot be used with multiple replicas. Switch to postgres or mysql." -}}
{{- end -}}
{{- end }}

{{- define "ghacs.env" -}}
- name: PORT
  value: "3000"
- name: API_BASE_URL
  value: {{ default (printf "http://%s.%s.svc.cluster.local:%v" (include "ghacs.fullname" .) .Release.Namespace .Values.service.port) .Values.config.apiBaseUrl | quote }}
- name: ENABLE_DIRECT_DOWNLOADS
  value: {{ .Values.config.enableDirectDownloads | quote }}
- name: CACHE_CLEANUP_OLDER_THAN_DAYS
  value: {{ .Values.config.cacheCleanupOlderThanDays | quote }}
{{- if .Values.config.disableCleanupJobs }}
- name: DISABLE_CLEANUP_JOBS
  value: "true"
{{- end }}
{{- if .Values.config.debug }}
- name: DEBUG
  value: "true"
{{- end }}
{{- if .Values.config.managementApiKey }}
- name: MANAGEMENT_API_KEY
  value: {{ .Values.config.managementApiKey | quote }}
{{- end }}
- name: STORAGE_DRIVER
  value: {{ .Values.config.storage.driver | quote }}
{{- if eq .Values.config.storage.driver "filesystem" }}
- name: STORAGE_FILESYSTEM_PATH
  value: {{ .Values.config.storage.filesystem.path | quote }}
{{- else if eq .Values.config.storage.driver "s3" }}
{{- with .Values.config.storage.s3 }}
{{- if .bucket }}{{- print "\n- name: STORAGE_S3_BUCKET\n  value: " | nospace }}{{ .bucket | quote }}{{- end }}
{{- if .region }}{{- print "\n- name: AWS_REGION\n  value: " | nospace }}{{ .region | quote }}{{- end }}
{{- if .endpointUrl }}{{- print "\n- name: AWS_ENDPOINT_URL\n  value: " | nospace }}{{ .endpointUrl | quote }}{{- end }}
{{- if .accessKeyId }}{{- print "\n- name: AWS_ACCESS_KEY_ID\n  value: " | nospace }}{{ .accessKeyId | quote }}{{- end }}
{{- if .secretAccessKey }}{{- print "\n- name: AWS_SECRET_ACCESS_KEY\n  value: " | nospace }}{{ .secretAccessKey | quote }}{{- end }}
{{- end }}
{{- else if eq .Values.config.storage.driver "gcs" }}
{{- with .Values.config.storage.gcs }}
{{- if .bucket }}{{- print "\n- name: STORAGE_GCS_BUCKET\n  value: " | nospace }}{{ .bucket | quote }}{{- end }}
{{- if .serviceAccountKey }}{{- print "\n- name: STORAGE_GCS_SERVICE_ACCOUNT_KEY\n  value: " | nospace }}{{ .serviceAccountKey | quote }}{{- end }}
{{- if .endpoint }}{{- print "\n- name: STORAGE_GCS_ENDPOINT\n  value: " | nospace }}{{ .endpoint | quote }}{{- end }}
{{- end }}
{{- end }}
- name: DB_DRIVER
  value: {{ .Values.config.db.driver | quote }}
{{- if eq .Values.config.db.driver "sqlite" }}
- name: DB_SQLITE_PATH
  value: {{ .Values.config.db.sqlite.path | quote }}
{{- else if eq .Values.config.db.driver "postgres" }}
{{- with .Values.config.db.postgres }}
{{- if .url }}{{- print "\n- name: DB_POSTGRES_URL\n  value: " | nospace }}{{ .url | quote }}
{{- else }}
{{- if .database }}{{- print "\n- name: DB_POSTGRES_DATABASE\n  value: " | nospace }}{{ .database | quote }}{{- end }}
{{- if .host }}{{- print "\n- name: DB_POSTGRES_HOST\n  value: " | nospace }}{{ .host | quote }}{{- end }}
{{- if .port }}{{- print "\n- name: DB_POSTGRES_PORT\n  value: " | nospace }}{{ .port | quote }}{{- end }}
{{- if .user }}{{- print "\n- name: DB_POSTGRES_USER\n  value: " | nospace }}{{ .user | quote }}{{- end }}
{{- if .password }}{{- print "\n- name: DB_POSTGRES_PASSWORD\n  value: " | nospace }}{{ .password | quote }}{{- end }}
{{- end }}
{{- end }}
{{- else if eq .Values.config.db.driver "mysql" }}
{{- with .Values.config.db.mysql }}
{{- if .database }}{{- print "\n- name: DB_MYSQL_DATABASE\n  value: " | nospace }}{{ .database | quote }}{{- end }}
{{- if .host }}{{- print "\n- name: DB_MYSQL_HOST\n  value: " | nospace }}{{ .host | quote }}{{- end }}
{{- if .port }}{{- print "\n- name: DB_MYSQL_PORT\n  value: " | nospace }}{{ .port | quote }}{{- end }}
{{- if .user }}{{- print "\n- name: DB_MYSQL_USER\n  value: " | nospace }}{{ .user | quote }}{{- end }}
{{- if .password }}{{- print "\n- name: DB_MYSQL_PASSWORD\n  value: " | nospace }}{{ .password | quote }}{{- end }}
{{- end }}
{{- end }}
{{- end }}
```

- [ ] **Step 2: Commit**

```bash
git add install/kubernetes/gha-cache-server/templates/_helpers.tpl
git commit -m "chore(helm): _helpers.tpl with naming + env-block builders"
```

---

### Task 22.4: `deployment.yaml`

**Files:**
- Create: `install/kubernetes/gha-cache-server/templates/deployment.yaml`

- [ ] **Step 1: Write the file**

```yaml
{{- include "ghacs.validate" . -}}
{{- $pvc := include "ghacs.pvcEnabled" . -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "ghacs.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels: {{- include "ghacs.labels" . | nindent 4 }}
spec:
  {{- if not .Values.autoscaling.enabled }}
  replicas: {{ .Values.replicaCount }}
  {{- end }}
  selector:
    matchLabels: {{- include "ghacs.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      {{- with .Values.podAnnotations }}
      annotations: {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "ghacs.labels" . | nindent 8 }}
        {{- with .Values.podLabels }}{{- toYaml . | nindent 8 }}{{- end }}
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets: {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "ghacs.serviceAccountName" . }}
      securityContext: {{- toYaml .Values.podSecurityContext | nindent 8 }}
      {{- with .Values.topologySpreadConstraints }}
      topologySpreadConstraints: {{- toYaml . | nindent 8 }}
      {{- end }}
      containers:
        - name: {{ .Chart.Name }}
          securityContext: {{- toYaml .Values.securityContext | nindent 12 }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}{{ if .Values.image.variant }}-{{ .Values.image.variant }}{{ end }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: cache
              containerPort: 3000
              protocol: TCP
          livenessProbe: {{- toYaml .Values.livenessProbe | nindent 12 }}
          readinessProbe: {{- toYaml .Values.readinessProbe | nindent 12 }}
          resources: {{- toYaml .Values.resources | nindent 12 }}
          {{- if or (eq $pvc "true") .Values.extraVolumeMounts }}
          volumeMounts:
            {{- if eq $pvc "true" }}
            - name: data
              mountPath: /data
            {{- end }}
            {{- with .Values.extraVolumeMounts }}{{- toYaml . | nindent 12 }}{{- end }}
          {{- end }}
          env:
            {{- include "ghacs.env" . | nindent 12 }}
            {{- with .Values.extraEnv }}{{- toYaml . | nindent 12 }}{{- end }}
          {{- if or .Values.existingSecret .Values.extraEnvFrom }}
          envFrom:
            {{- if .Values.existingSecret }}
            - secretRef:
                name: {{ .Values.existingSecret }}
            {{- end }}
            {{- with .Values.extraEnvFrom }}{{- toYaml . | nindent 12 }}{{- end }}
          {{- end }}
      {{- if or (eq $pvc "true") .Values.extraVolumes }}
      volumes:
        {{- if eq $pvc "true" }}
        - name: data
          persistentVolumeClaim:
            claimName: {{ include "ghacs.pvcName" . }}
        {{- end }}
        {{- with .Values.extraVolumes }}{{- toYaml . | nindent 8 }}{{- end }}
      {{- end }}
      {{- with .Values.nodeSelector }}
      nodeSelector: {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity: {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations: {{- toYaml . | nindent 8 }}
      {{- end }}
```

- [ ] **Step 2: Commit**

```bash
git add install/kubernetes/gha-cache-server/templates/deployment.yaml
git commit -m "chore(helm): Deployment template"
```

---

### Task 22.5: `service.yaml`, `serviceaccount.yaml`, `ingress.yaml`

**Files:**
- Create: `install/kubernetes/gha-cache-server/templates/service.yaml`
- Create: `install/kubernetes/gha-cache-server/templates/serviceaccount.yaml`
- Create: `install/kubernetes/gha-cache-server/templates/ingress.yaml`

- [ ] **Step 1: `service.yaml`**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ include "ghacs.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels: {{- include "ghacs.labels" . | nindent 4 }}
  {{- with .Values.service.annotations }}
  annotations: {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  type: {{ .Values.service.type }}
  ports:
    - port: {{ .Values.service.port }}
      targetPort: cache
      protocol: TCP
      name: cache
  selector: {{- include "ghacs.selectorLabels" . | nindent 4 }}
```

- [ ] **Step 2: `serviceaccount.yaml`**

```yaml
{{- if .Values.serviceAccount.create -}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "ghacs.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
  labels: {{- include "ghacs.labels" . | nindent 4 }}
  {{- with .Values.serviceAccount.annotations }}
  annotations: {{- toYaml . | nindent 4 }}
  {{- end }}
automountServiceAccountToken: {{ .Values.serviceAccount.automount }}
{{- end }}
```

- [ ] **Step 3: `ingress.yaml`**

```yaml
{{- if .Values.ingress.enabled -}}
{{- $name := include "ghacs.fullname" . -}}
{{- $port := .Values.service.port -}}
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ $name }}
  namespace: {{ .Release.Namespace }}
  labels: {{- include "ghacs.labels" . | nindent 4 }}
  {{- with .Values.ingress.annotations }}
  annotations: {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  {{- if .Values.ingress.className }}
  ingressClassName: {{ .Values.ingress.className }}
  {{- end }}
  {{- with .Values.ingress.tls }}
  tls:
    {{- range . }}
    - hosts:
        {{- range .hosts }}
        - {{ . | quote }}
        {{- end }}
      secretName: {{ .secretName }}
    {{- end }}
  {{- end }}
  rules:
    {{- range .Values.ingress.hosts }}
    - host: {{ .host | quote }}
      http:
        paths:
          {{- range .paths }}
          - path: {{ .path }}
            pathType: {{ .pathType | default "ImplementationSpecific" }}
            backend:
              service:
                name: {{ $name }}
                port:
                  number: {{ $port }}
          {{- end }}
    {{- end }}
{{- end }}
```

- [ ] **Step 4: Commit**

```bash
git add install/kubernetes/gha-cache-server/templates/{service,serviceaccount,ingress}.yaml
git commit -m "chore(helm): Service, ServiceAccount, Ingress templates"
```

---

### Task 22.6: `persistentvolumeclaim.yaml`, `hpa.yaml`, `NOTES.txt`, `tests/test-connection.yaml`

**Files:**
- Create: `install/kubernetes/gha-cache-server/templates/persistentvolumeclaim.yaml`
- Create: `install/kubernetes/gha-cache-server/templates/hpa.yaml`
- Create: `install/kubernetes/gha-cache-server/templates/NOTES.txt`
- Create: `install/kubernetes/gha-cache-server/templates/tests/test-connection.yaml`

- [ ] **Step 1: `persistentvolumeclaim.yaml`**

```yaml
{{- if eq (include "ghacs.pvcEnabled" .) "true" -}}
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ include "ghacs.pvcName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "ghacs.labels" . | nindent 4 }}
    {{- with .Values.persistentVolumeClaim.labels }}{{- toYaml . | nindent 4 }}{{- end }}
  {{- with .Values.persistentVolumeClaim.annotations }}
  annotations: {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  accessModes:
    {{- include "ghacs.pvcAccessModes" . | nindent 4 }}
  resources:
    requests:
      storage: {{ .Values.persistentVolumeClaim.storage }}
  volumeMode: Filesystem
  {{- with .Values.persistentVolumeClaim.storageClassName }}
  storageClassName: {{ . }}
  {{- end }}
{{- end }}
```

- [ ] **Step 2: `hpa.yaml`**

```yaml
{{- if .Values.autoscaling.enabled }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "ghacs.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels: {{- include "ghacs.labels" . | nindent 4 }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "ghacs.fullname" . }}
  minReplicas: {{ .Values.autoscaling.minReplicas }}
  maxReplicas: {{ .Values.autoscaling.maxReplicas }}
  metrics:
    {{- with .Values.autoscaling.targetCPUUtilizationPercentage }}
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ . }}
    {{- end }}
    {{- with .Values.autoscaling.targetMemoryUtilizationPercentage }}
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: {{ . }}
    {{- end }}
{{- end }}
```

- [ ] **Step 3: `NOTES.txt`**

```
GitHub Actions Cache Server (Go) deployed.

  Release:   {{ .Release.Name }}
  Namespace: {{ .Release.Namespace }}
  Storage:   {{ .Values.config.storage.driver }}
  Database:  {{ .Values.config.db.driver }}

{{- with .Values.config.apiBaseUrl }}
  API URL:   {{ . }}
{{- end }}

Set ACTIONS_RESULTS_URL on your runners to the API URL.
Docs: https://gha-cache-server.falcondev.io/getting-started
```

- [ ] **Step 4: `tests/test-connection.yaml`**

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: "{{ include "ghacs.fullname" . }}-test-connection"
  namespace: {{ .Release.Namespace }}
  labels: {{- include "ghacs.labels" . | nindent 4 }}
  annotations:
    "helm.sh/hook": test
spec:
  restartPolicy: Never
  containers:
    - name: curl
      image: curlimages/curl:8.10.1
      args:
        - "-fsS"
        - "http://{{ include "ghacs.fullname" . }}:{{ .Values.service.port }}/health"
```

- [ ] **Step 5: Commit**

```bash
git add install/kubernetes/gha-cache-server/templates/persistentvolumeclaim.yaml \
        install/kubernetes/gha-cache-server/templates/hpa.yaml \
        install/kubernetes/gha-cache-server/templates/NOTES.txt \
        install/kubernetes/gha-cache-server/templates/tests/test-connection.yaml
git commit -m "chore(helm): PVC, HPA, NOTES, test-connection"
```

---

### Task 22.7: Helm lint and template smoke tests

**Files:**
- Create: `install/kubernetes/gha-cache-server/ci/sqlite-fs-values.yaml`
- Create: `install/kubernetes/gha-cache-server/ci/postgres-s3-values.yaml`
- Create: `install/kubernetes/gha-cache-server/ci/autoscaling-values.yaml`

These small fixture files exercise different code paths. They keep `helm lint` strict.

- [ ] **Step 1: Write `ci/sqlite-fs-values.yaml`**

```yaml
config:
  storage:
    driver: filesystem
    filesystem:
      path: /data/cache
  db:
    driver: sqlite
    sqlite:
      path: /data/cache-server.db
```

- [ ] **Step 2: Write `ci/postgres-s3-values.yaml`**

```yaml
config:
  storage:
    driver: s3
    s3:
      bucket: gh-actions-cache
      region: eu-west-1
  db:
    driver: postgres
    postgres:
      url: postgres://u:p@pg:5432/db
```

- [ ] **Step 3: Write `ci/autoscaling-values.yaml`**

```yaml
replicaCount: 3
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 5
config:
  storage:
    driver: filesystem
  db:
    driver: postgres
    postgres:
      url: postgres://u:p@pg:5432/db
```

- [ ] **Step 4: Run lint and template renders locally**

```bash
helm lint install/kubernetes/gha-cache-server
for f in install/kubernetes/gha-cache-server/ci/*.yaml; do
  helm template test-release install/kubernetes/gha-cache-server -f "$f" >/dev/null
done
```

Expected: no errors.

- [ ] **Step 5: Verify the SQLite + multi-replica failure path**

```bash
helm template test install/kubernetes/gha-cache-server \
  --set replicaCount=3 --set config.db.driver=sqlite 2>&1 | grep -i "SQLite cannot be used"
```

Expected: error containing "SQLite cannot be used".

- [ ] **Step 6: Commit**

```bash
git add install/kubernetes/gha-cache-server/ci
git commit -m "test(helm): CI value files for lint matrix"
```

---

## Phase 23 — GitHub Actions Workflows

### Task 23.1: `ci.yml` — lint, test, build

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the file**

```yaml
name: CI
on:
  pull_request:
    branches: [main, master, dev]
  push:
    branches: [main, master, dev]
  workflow_dispatch:

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

jobs:
  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true
      - run: go vet ./...
      - run: |
          fmt=$(gofmt -l .)
          if [ -n "$fmt" ]; then echo "unformatted:"; echo "$fmt"; exit 1; fi
      - uses: golangci/golangci-lint-action@v6
        with:
          version: v1.61.0

  test-unit:
    name: Unit tests
    runs-on: ubuntu-latest
    needs: [lint]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true
      - run: go test -race -coverprofile=coverage.out ./...
      - uses: actions/upload-artifact@v4
        with:
          name: coverage
          path: coverage.out

  test-integration:
    name: Integration (${{ matrix.storage }} + ${{ matrix.db }})
    runs-on: ubuntu-latest
    needs: [lint]
    strategy:
      fail-fast: false
      matrix:
        storage: [filesystem, s3, gcs]
        db: [sqlite, postgres, mysql]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true
      - run: go test -race -tags=integration -timeout=15m ./...
        env:
          TEST_STORAGE_DRIVER: ${{ matrix.storage }}
          TEST_DB_DRIVER: ${{ matrix.db }}

  build:
    name: Build binary
    runs-on: ubuntu-latest
    needs: [lint]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true
      - run: CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o cache-server ./cmd/cache-server
      - uses: actions/upload-artifact@v4
        with:
          name: cache-server-linux-amd64
          path: cache-server

  helm-lint:
    name: Helm lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
        with:
          version: v3.16.1
      - run: helm lint install/kubernetes/gha-cache-server
      - name: Template all CI value files
        run: |
          for f in install/kubernetes/gha-cache-server/ci/*.yaml; do
            echo "=== $f ==="
            helm template test install/kubernetes/gha-cache-server -f "$f" > /dev/null
          done
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: lint, unit, integration matrix, build, helm lint"
```

---

### Task 23.2: `release.yml` — image and chart publishing on tag

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Write the file**

```yaml
name: Release

on:
  push:
    tags: ['v*']
  workflow_dispatch:

permissions:
  contents: write
  packages: write

jobs:
  build-image:
    name: Build & push image
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Resolve tag
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=semver,pattern={{major}}
            type=ref,event=branch
            type=raw,value=latest,enable={{is_default_branch}}

      - uses: docker/build-push-action@v6
        with:
          context: .
          file: ./Dockerfile
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: |
            BUILD_HASH=${{ github.sha }}
          provenance: true
          sbom: true

  release-binaries:
    name: Release binaries
    runs-on: ubuntu-latest
    needs: [build-image]
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  publish-chart:
    name: Publish Helm chart
    runs-on: ubuntu-latest
    needs: [build-image]
    env:
      CHART_DIR: install/kubernetes/gha-cache-server
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
        with:
          version: v3.16.1

      - name: Sync chart appVersion to release tag
        run: |
          VERSION="${GITHUB_REF_NAME#v}"
          sed -i -E "s/^appVersion:.*$/appVersion: \"$VERSION\"/" "$CHART_DIR/Chart.yaml"
          sed -i -E "s/^version:.*$/version: $VERSION/" "$CHART_DIR/Chart.yaml"

      - name: Login to GHCR (Helm OCI)
        run: echo "${{ secrets.GITHUB_TOKEN }}" | helm registry login ghcr.io -u "${{ github.actor }}" --password-stdin

      - name: Package & push
        run: |
          helm package "$CHART_DIR" --destination packaged
          for pkg in packaged/*.tgz; do
            helm push "$pkg" "oci://ghcr.io/${{ github.repository_owner }}/charts"
          done
```

- [ ] **Step 2: Add `.goreleaser.yaml`**

```yaml
version: 2
project_name: cache-server
before:
  hooks:
    - go mod tidy
builds:
  - id: cache-server
    main: ./cmd/cache-server
    binary: cache-server
    env: [CGO_ENABLED=0]
    flags: [-trimpath]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]

archives:
  - format: tar.gz
    name_template: '{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}'
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: 'checksums.txt'

snapshot:
  version_template: '{{ incpatch .Version }}-next'

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
      - '^chore:'

release:
  draft: false
  prerelease: auto
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml .goreleaser.yaml
git commit -m "ci: release pipeline (multi-arch image, binaries, OCI helm chart)"
```

---

### Task 23.3: Update Dockerfile to use the matched build args

**Files:**
- Modify: `Dockerfile`

- [ ] **Step 1: Replace contents**

```dockerfile
ARG BASE_IMAGE=gcr.io/distroless/static:nonroot

FROM golang:1.22-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG BUILD_HASH=dev
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${BUILD_HASH}" \
    -o /out/cache-server ./cmd/cache-server

FROM ${BASE_IMAGE}
WORKDIR /
COPY --from=builder /out/cache-server /cache-server
USER nonroot:nonroot
EXPOSE 3000
ENTRYPOINT ["/cache-server"]
```

> Note: we drop the upstream's `caged` variant — V8 pointer compression is Node-only. If you want a debugging variant, set `BASE_IMAGE=alpine:3.20` and rebuild; the `Dockerfile` already supports that via the build arg.

- [ ] **Step 2: Smoke build**

```bash
docker build --build-arg BUILD_HASH=$(git rev-parse --short HEAD) -t gha-cache-go:dev .
docker run --rm -p 3001:3000 -e API_BASE_URL=http://localhost:3001 -d --name gha-test gha-cache-go:dev
sleep 1
curl -fsS http://localhost:3001/health
docker rm -f gha-test
```

Expected: `healthy`.

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "chore: parameterised Dockerfile (BASE_IMAGE, VERSION, BUILD_HASH)"
```

---

## Phase 24 — Documentation Site (VitePress)

Mirror `falcondev-oss/docs/packages/gha-cache-server`. Same page hierarchy, our own prose, deployable to any static host (the upstream uses Cloudflare Workers via Wrangler).

### Task 24.1: VitePress scaffold

**Files:**
- Create: `docs/package.json`
- Create: `docs/.vitepress/config.ts`
- Create: `docs/.gitignore`

- [ ] **Step 1: Write `docs/package.json`**

```json
{
  "name": "gha-cache-server-go-docs",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vitepress dev",
    "build": "vitepress build",
    "preview": "vitepress preview"
  },
  "devDependencies": {
    "vitepress": "^1.5.0",
    "vue": "^3.5.0"
  }
}
```

- [ ] **Step 2: Write `docs/.gitignore`**

```
.vitepress/cache
.vitepress/dist
node_modules
```

- [ ] **Step 3: Write `docs/.vitepress/config.ts`**

```ts
import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'GHA Cache Server (Go)',
  description: 'Self-hosted GitHub Actions cache server, Go port. Drop-in compatible with actions/cache.',
  cleanUrls: true,
  themeConfig: {
    nav: [
      { text: 'Guide', link: '/getting-started' },
      { text: 'GitHub', link: 'https://github.com/falcondev-oss/github-actions-cache-server' },
    ],
    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Getting Started', link: '/getting-started' },
          { text: 'How it works', link: '/how-it-works' },
          { text: 'Helm Chart', link: '/helm' },
          { text: 'Management API', link: '/management-api' },
        ],
      },
      {
        text: 'Storage Drivers',
        items: [
          { text: 'File System', link: '/storage-drivers/file-system' },
          { text: 'S3 / MinIO', link: '/storage-drivers/s3' },
          { text: 'Google Cloud Storage', link: '/storage-drivers/google-cloud-storage' },
        ],
      },
      {
        text: 'Database Drivers',
        items: [
          { text: 'SQLite', link: '/database-drivers/sqlite' },
          { text: 'PostgreSQL', link: '/database-drivers/postgres' },
          { text: 'MySQL', link: '/database-drivers/mysql' },
        ],
      },
    ],
  },
})
```

- [ ] **Step 4: Commit**

```bash
git add docs/package.json docs/.vitepress/config.ts docs/.gitignore
git commit -m "docs: VitePress scaffold"
```

---

### Task 24.2: Homepage and core pages

**Files:**
- Create: `docs/index.md`
- Create: `docs/getting-started.md`
- Create: `docs/how-it-works.md`
- Create: `docs/helm.md`
- Create: `docs/management-api.md`

- [ ] **Step 1: `docs/index.md`**

```markdown
---
layout: home

hero:
  name: GitHub Actions
  text: Cache Server (Go)
  tagline: A small, fast, self-hosted GitHub Actions cache. Drop-in compatible with actions/cache. Single static binary.
  actions:
    - theme: brand
      text: Get Started
      link: /getting-started
    - theme: alt
      text: How it works
      link: /how-it-works

features:
  - title: One static binary
    details: ~20MB, no runtime, no dependencies. Distroless image.
    icon: 🪶
  - title: Storage drivers
    details: filesystem, S3 / MinIO, Google Cloud Storage.
    icon: 📦
  - title: Database drivers
    details: SQLite (default), PostgreSQL, MySQL.
    icon: 🗄️
  - title: No workflow changes
    details: Uses the same cache protocol — point your runner at the server and you're done.
    icon: ⚙️
  - title: Helm chart
    details: First-class Kubernetes deploy. Auto PVC for filesystem/sqlite, HPA-aware.
    icon: ⛵
  - title: Management API
    details: REST + OpenAPI/Swagger UI for inspecting and pruning cache.
    icon: 🔧
---
```

- [ ] **Step 2: `docs/getting-started.md`**

```markdown
---
title: Getting Started
outline: [2, 3]
---

# Getting Started

The cache server is shipped as a Docker image and a Helm chart. The simplest deployment uses filesystem storage and SQLite, all on a single volume.

## Docker Compose

```yaml
services:
  cache-server:
    image: ghcr.io/falcondev-oss/github-actions-cache-server-go:latest
    ports:
      - '3000:3000'
    environment:
      API_BASE_URL: http://localhost:3000
      STORAGE_DRIVER: filesystem
      STORAGE_FILESYSTEM_PATH: /data/cache
      DB_DRIVER: sqlite
      DB_SQLITE_PATH: /data/cache-server.db
    volumes:
      - cache-data:/data

volumes:
  cache-data:
```

## Kubernetes via Helm

Requires Helm 3.8+ for OCI registries.

```bash
helm install cache-server \
  oci://ghcr.io/falcondev-oss/charts/gha-cache-server
```

Customise via `values.yaml` (see [Helm Chart](/helm) for the full surface).

## Self-hosted runner setup

The runner must be told to talk to your cache server instead of GitHub's. Set `ACTIONS_RESULTS_URL` to the cache server's API URL, **with a trailing slash**.

The official GitHub runner overwrites `ACTIONS_RESULTS_URL` with GitHub's endpoint at boot. Two options to defeat that:

1. **Forked runner image** with the override patched in: [`falcondev-oss/github-actions-runner`](https://github.com/falcondev-oss/github-actions-runner). The image accepts `CUSTOM_ACTIONS_RESULTS_URL`.
2. **Binary patch** of `Runner.Worker.dll` to rename the env var the runner overwrites. See the upstream [docs](https://gha-cache-server.falcondev.io/getting-started#binary-patch).

## Required environment variables

| Variable | Default | Notes |
|---|---|---|
| `API_BASE_URL` | _required_ | Public base URL the runner can reach |
| `STORAGE_DRIVER` | `filesystem` | `filesystem` \| `s3` \| `gcs` |
| `DB_DRIVER` | `sqlite` | `sqlite` \| `postgres` \| `mysql` |
| `PORT` | `3000` | Listen port |
| `CACHE_CLEANUP_OLDER_THAN_DAYS` | `90` | 0 disables age-based cleanup |
| `ENABLE_DIRECT_DOWNLOADS` | `false` | Hand the runner a presigned URL |
| `MANAGEMENT_API_KEY` | _unset_ | Enables `/management-api` when set |
| `SKIP_TOKEN_VALIDATION` | `false` | Dev only — disables JWT verification |
| `DEBUG` | `false` | Verbose logging |

Per-driver variables are documented under [Storage Drivers](/storage-drivers/file-system) and [Database Drivers](/database-drivers/sqlite).
```

- [ ] **Step 3: `docs/how-it-works.md`**

```markdown
---
title: How it works
---

# How it works

The cache server reproduces the GitHub Actions cache wire protocol so that an unmodified `actions/cache` (v3 or v4) can save and restore caches against it.

## Protocol surface

- **Twirp v2 control plane** at `/twirp/github.actions.results.api.v1.CacheService/{Create,Get,Finalize}CacheEntry` (JSON over POST). Tells the runner where to upload and where to download.
- **Azure-style block-blob upload** at `/devstoreaccount1/upload/{uploadId}`. The runner PUTs each chunk with `?blockid=<base64>`, then commits with `?comp=blocklist`.
- **Direct streaming download** at `/download/{cacheEntryId}`. Used when direct-download presigned URLs are disabled or the entry hasn't yet been merged.
- **Catch-all proxy** for any other paths the runner might hit — forwards to `DEFAULT_ACTIONS_RESULTS_URL`.

## Storage layout

Every upload gets a numeric folder. Each chunk lands at `<folder>/parts/<index>`. On the first download, the server **streams parts back to the runner *and* simultaneously writes them to a `<folder>/merged` blob** so subsequent downloads can use a single object (and a single presigned URL when supported).

A small state machine in `storage_locations` tracks `mergeStartedAt`/`mergedAt`/`partsDeletedAt`. Cleanup tasks reset stalled merges, drop orphaned locations, and prune entries older than `CACHE_CLEANUP_OLDER_THAN_DAYS`.

## Authentication

Each runner request carries a JWT issued by `https://token.actions.githubusercontent.com`. The server fetches that issuer's JWKS, caches it for 10 minutes, and verifies signatures (RS256, ES256). Two custom claims are extracted:

- `ac` — JSON-encoded array of `{Scope, Permission}` pairs (Permission ≥ 2 = write).
- `repository_id` — namespace for the cache.
```

- [ ] **Step 4: `docs/helm.md`**

```markdown
---
title: Helm Chart
---

# Helm Chart

The chart lives at `install/kubernetes/gha-cache-server` in the repo and is published to `oci://ghcr.io/falcondev-oss/charts/gha-cache-server`.

## Install

```bash
helm install cache-server \
  oci://ghcr.io/falcondev-oss/charts/gha-cache-server \
  -f my-values.yaml
```

## Common configurations

### Filesystem + SQLite (single-node)

```yaml
replicaCount: 1
config:
  storage:
    driver: filesystem
    filesystem:
      path: /data/cache
  db:
    driver: sqlite
    sqlite:
      path: /data/cache-server.db
persistentVolumeClaim:
  storage: 50Gi
```

### S3 + Postgres (HA)

```yaml
replicaCount: 3
autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10

existingSecret: cache-server-secrets

config:
  enableDirectDownloads: true
  storage:
    driver: s3
    s3:
      bucket: gh-actions-cache
      region: eu-west-1
  db:
    driver: postgres
    postgres:
      host: pg.internal
      port: 5432
      database: cache
      user: cache
```

The `cache-server-secrets` secret should contain `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `DB_POSTGRES_PASSWORD`.

## Behaviour notes

- **PVC auto-enables** when `storage.driver=filesystem` *or* `db.driver=sqlite`.
- **`accessModes` auto-switches** to `ReadWriteMany` if you scale the filesystem driver beyond one replica.
- **SQLite + multi-replica is rejected** at install time — switch to Postgres or MySQL.
```

- [ ] **Step 5: `docs/management-api.md`**

```markdown
---
title: Management API
---

# Management API

When `MANAGEMENT_API_KEY` is set, the server exposes a REST surface for inspecting and pruning the cache. Without the key, all `/management-api/*` routes return `503`.

## Auth

Every request must carry the API key in `X-Api-Key`.

## Interactive docs

Swagger UI: `https://<your-cache-server>/management-api/_docs`
OpenAPI JSON: `https://<your-cache-server>/management-api/_docs/spec.json`

## Endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/management-api/cache-entries` | List with filters and pagination |
| GET | `/management-api/cache-entries/{id}` | Fetch one |
| GET | `/management-api/cache-entries/match` | Replay match logic for a primary/restore-key combo |
| DELETE | `/management-api/cache-entries/{id}` | Delete one |
| DELETE | `/management-api/cache-entries` | Bulk delete by filters |
| GET | `/management-api/storage-locations/{id}` | Fetch storage location |
| DELETE | `/management-api/storage-locations/{id}` | Delete location and underlying files |

Filter parameters: `key`, `version`, `scope`, `repoId`, plus `page` and `itemsPerPage` (1-100).
```

- [ ] **Step 6: Commit**

```bash
git add docs/index.md docs/getting-started.md docs/how-it-works.md docs/helm.md docs/management-api.md
git commit -m "docs: homepage, getting-started, how-it-works, helm, management-api"
```

---

### Task 24.3: Driver pages

**Files:**
- Create: `docs/storage-drivers/file-system.md`
- Create: `docs/storage-drivers/s3.md`
- Create: `docs/storage-drivers/google-cloud-storage.md`
- Create: `docs/database-drivers/sqlite.md`
- Create: `docs/database-drivers/postgres.md`
- Create: `docs/database-drivers/mysql.md`

- [ ] **Step 1: `storage-drivers/file-system.md`**

```markdown
---
title: File System
---

# File System

Driver value: `filesystem` (default).

## Variables

| Var | Default | Notes |
|---|---|---|
| `STORAGE_FILESYSTEM_PATH` | `.data/storage/filesystem` | Created if missing |

## When to use

- Single-host deployments.
- Small teams. The filesystem driver is the simplest path; combine it with the SQLite driver for zero-dependency operation.

For multi-replica deployments use S3 or GCS instead — the filesystem driver requires `ReadWriteMany` shared volumes which add operational cost.
```

- [ ] **Step 2: `storage-drivers/s3.md`**

```markdown
---
title: S3 / MinIO
---

# S3 / MinIO

Driver value: `s3`. Works against any S3-compatible API (AWS S3, MinIO, Ceph RGW, etc).

## Variables

| Var | Default | Notes |
|---|---|---|
| `STORAGE_S3_BUCKET` | _required_ | Bucket name |
| `AWS_REGION` | `us-east-1` | |
| `AWS_ENDPOINT_URL` | _empty_ | Set for non-AWS providers |
| `AWS_ACCESS_KEY_ID` | _empty_ | Falls back to instance role / IRSA |
| `AWS_SECRET_ACCESS_KEY` | _empty_ | |

## MinIO example

```yaml
services:
  cache-server:
    image: ghcr.io/falcondev-oss/github-actions-cache-server-go:latest
    ports: ['3000:3000']
    environment:
      API_BASE_URL: http://localhost:3000
      STORAGE_DRIVER: s3
      STORAGE_S3_BUCKET: gh-actions-cache
      AWS_ENDPOINT_URL: http://minio:9000
      AWS_ACCESS_KEY_ID: minioadmin
      AWS_SECRET_ACCESS_KEY: minioadmin
  minio:
    image: quay.io/minio/minio
    command: server /data
    ports: ['9000:9000']
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
```

## Direct downloads

Set `ENABLE_DIRECT_DOWNLOADS=true` to have the server hand the runner a 10-minute presigned URL instead of streaming the cache through itself. Reduces bandwidth on the cache server and improves throughput.

The runner must be able to reach the bucket directly.
```

- [ ] **Step 3: `storage-drivers/google-cloud-storage.md`**

```markdown
---
title: Google Cloud Storage
---

# Google Cloud Storage

Driver value: `gcs`.

## Variables

| Var | Default | Notes |
|---|---|---|
| `STORAGE_GCS_BUCKET` | _required_ | |
| `STORAGE_GCS_SERVICE_ACCOUNT_KEY` | _empty_ | Path to JSON key file. Omit on GKE to use Workload Identity. |
| `STORAGE_GCS_ENDPOINT` | _empty_ | Set when targeting fake-gcs-server in tests |

## Example

```yaml
services:
  cache-server:
    image: ghcr.io/falcondev-oss/github-actions-cache-server-go:latest
    ports: ['3000:3000']
    environment:
      API_BASE_URL: http://localhost:3000
      STORAGE_DRIVER: gcs
      STORAGE_GCS_BUCKET: gh-actions-cache
      STORAGE_GCS_SERVICE_ACCOUNT_KEY: /gcp/sa.json
    volumes:
      - ./sa.json:/gcp/sa.json:ro
```

## Direct downloads

Not yet wired up for GCS — `ENABLE_DIRECT_DOWNLOADS=true` falls back to streaming through the cache server. PRs welcome.
```

- [ ] **Step 4: `database-drivers/sqlite.md`**

```markdown
---
title: SQLite
---

# SQLite

Driver value: `sqlite` (default).

## Variables

| Var | Default | Notes |
|---|---|---|
| `DB_SQLITE_PATH` | `.data/sqlite.db` | Created if missing |

## Caveats

- **Single-replica only.** SQLite cannot support concurrent writers from multiple processes. The Helm chart refuses to render a Deployment when `replicaCount > 1` with the SQLite driver.
- WAL mode is enabled automatically for better write throughput.
```

- [ ] **Step 5: `database-drivers/postgres.md`**

```markdown
---
title: PostgreSQL
---

# PostgreSQL

Driver value: `postgres`.

## Variables

Use either `DB_POSTGRES_URL` or the individual fields below.

| Var | Required | Notes |
|---|---|---|
| `DB_POSTGRES_URL` | one-of | Connection string (`postgres://user:pass@host:port/db`) |
| `DB_POSTGRES_HOST` | one-of | |
| `DB_POSTGRES_PORT` | one-of | |
| `DB_POSTGRES_DATABASE` | one-of | |
| `DB_POSTGRES_USER` | one-of | |
| `DB_POSTGRES_PASSWORD` | one-of | |

## Notes

- Migrations run on startup. Idempotent.
- Pool size: 10 connections.
```

- [ ] **Step 6: `database-drivers/mysql.md`**

```markdown
---
title: MySQL
---

# MySQL

Driver value: `mysql`.

## Variables

| Var | Notes |
|---|---|
| `DB_MYSQL_HOST` | |
| `DB_MYSQL_PORT` | typically 3306 |
| `DB_MYSQL_DATABASE` | |
| `DB_MYSQL_USER` | |
| `DB_MYSQL_PASSWORD` | |

Tested against MySQL 8.x. MariaDB has not been formally tested but should work since we only use ANSI SQL.
```

- [ ] **Step 7: Commit**

```bash
git add docs/storage-drivers docs/database-drivers
git commit -m "docs: storage and database driver pages"
```

---

### Task 24.4: Docs deployment workflow

**Files:**
- Create: `.github/workflows/docs.yml`

- [ ] **Step 1: Write the workflow**

```yaml
name: Docs

on:
  push:
    branches: [main, master]
    paths:
      - 'docs/**'
      - '.github/workflows/docs.yml'
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: docs
  cancel-in-progress: true

jobs:
  build:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: docs
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 'lts/*'
          cache: npm
          cache-dependency-path: docs/package-lock.json
      - run: npm ci || npm install
      - run: npm run build
      - uses: actions/upload-pages-artifact@v3
        with:
          path: docs/.vitepress/dist

  deploy:
    needs: build
    runs-on: ubuntu-latest
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - id: deployment
        uses: actions/deploy-pages@v4
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/docs.yml
git commit -m "ci: deploy docs to GitHub Pages on main"
```

---

## Phase 25 — Final Verification

### Task 25.1: Whole-tree sanity check

- [ ] **Step 1: Tree shape matches upstream**

Run:
```bash
find . -maxdepth 4 \( -path ./.git -o -path ./node_modules -o -path '*/.vitepress/cache' -o -path '*/.vitepress/dist' \) -prune -o -type f -print | sort
```

Expected file set includes:
- `cmd/cache-server/main.go`
- `internal/{config,logging,ids,db,storage,auth,server,cron,tasks}/...`
- `tests/e2e/e2e_test.go`
- `Dockerfile`, `docker-compose.yml`, `go.mod`, `go.sum`, `.goreleaser.yaml`
- `.github/workflows/{ci,release,docs}.yml`
- `install/kubernetes/gha-cache-server/{Chart.yaml,values.yaml,.helmignore,templates/*,ci/*}`
- `docs/{package.json,index.md,getting-started.md,how-it-works.md,helm.md,management-api.md,storage-drivers/*.md,database-drivers/*.md,.vitepress/config.ts}`
- `README.md`

- [ ] **Step 2: Run every check**

```bash
go vet ./...
gofmt -l .
go test -race ./...
go test -race -tags=integration ./... || true   # SKIPs without docker
helm lint install/kubernetes/gha-cache-server
for f in install/kubernetes/gha-cache-server/ci/*.yaml; do
  helm template release install/kubernetes/gha-cache-server -f "$f" > /dev/null
done
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o cache-server ./cmd/cache-server
ls -lh cache-server
```

Expected: every step exits 0 (the integration suite may print SKIP messages without docker).

- [ ] **Step 3: Commit final tag**

```bash
git tag v0.1.0
git log --oneline | head -50
```

---

## Final Self-Review

Re-read against the upstream layout:

| Upstream | Our equivalent | Notes |
|---|---|---|
| `routes/twirp/...` | `internal/server/twirp.go` | Phase 9 |
| `routes/devstoreaccount1/upload/...` | `internal/server/upload.go` | Phase 10 |
| `routes/upload/[uploadId].put.ts` | alias inside `upload.go` | Phase 10 |
| `routes/download/[cacheEntryId].ts` | `internal/server/download.go` | Phase 11 |
| `routes/management-api/...` | `internal/server/management.go` + `openapi.go` | Phase 13, 21 |
| `routes/[...path].ts` (proxy) | `internal/server/proxy.go` | Phase 14 |
| `routes/health.ts`, `routes/index.ts` | `internal/server/health.go` | Phase 8 |
| `lib/storage.ts` | `internal/storage/{adapter,filesystem,s3,gcs,service}.go` | Phase 5, 6, 16, 17 |
| `lib/db.ts` + `lib/migrations.ts` | `internal/db/{db,queries,migrations}.go` | Phase 4 |
| `lib/scope.ts` | `internal/auth/{jwks,jwt,scope}.go` | Phase 7 |
| `lib/env.ts` + `lib/schemas.ts` | `internal/config/config.go` | Phase 1, 21 |
| `lib/helpers.ts` | `internal/ids/ids.go` | Phase 3 |
| `tasks/cleanup/*.ts` | `internal/tasks/cleanup_*.go` | Phase 12 |
| `nitro.config.ts` (scheduledTasks) | `internal/cron/cron.go` + wiring in `main.go` | Phase 12, 15 |
| `Dockerfile` | `Dockerfile` (Go-based, distroless) | Phase 23 |
| `install/kubernetes/.../*` | `install/kubernetes/.../*` (1:1 file layout, our authoring) | Phase 22 |
| `.github/workflows/ci-cd.yml` | `.github/workflows/ci.yml` | Phase 23 |
| `.github/workflows/release.yml` | `.github/workflows/release.yml` + `.goreleaser.yaml` | Phase 23 |
| `falcondev-oss/docs/.../*` | `docs/*` (VitePress with our prose) | Phase 24 |

**Notable intentional differences from upstream:**

1. **No `caged` image variant.** That variant uses Node-only V8 pointer compression. Our binary already runs at a fraction of the upstream's memory footprint, so the variant is unnecessary. The Dockerfile parameterises `BASE_IMAGE` so a debug build image is one arg away.

2. **Image and chart names** are suffixed `-go` (`ghcr.io/falcondev-oss/github-actions-cache-server-go`). They sit alongside, not over, the upstream so operators can pin per-deployment.

3. **Single-file OpenAPI spec** (vs. oRPC's runtime generation). Same surface, no runtime cost.

4. **Stricter container security defaults** (`readOnlyRootFilesystem: true`, dropped resources limits). The Go binary doesn't need scratch space outside the data volume.

5. **Larger test matrix** (lint, unit, integration matrix, build, helm-lint as separate jobs) for clearer CI signal.


