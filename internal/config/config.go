package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
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

	GCSBucket            string
	GCSServiceAccountKey string
	GCSEndpoint          string

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

	DiskPressureMinFreeBytes    int64
	DiskPressureTargetFreeBytes int64
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
	} else if v := env("PORT"); v != "" {
		c.ListenAddr = ":" + v
	} else if v := env("NITRO_PORT"); v != "" {
		c.ListenAddr = ":" + v
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

	if v := env("DISK_PRESSURE_MIN_FREE_BYTES"); v != "" {
		n, err := parseBytes(v)
		if err != nil {
			return nil, fmt.Errorf("DISK_PRESSURE_MIN_FREE_BYTES: %w", err)
		}
		c.DiskPressureMinFreeBytes = n
	}
	if v := env("DISK_PRESSURE_TARGET_FREE_BYTES"); v != "" {
		n, err := parseBytes(v)
		if err != nil {
			return nil, fmt.Errorf("DISK_PRESSURE_TARGET_FREE_BYTES: %w", err)
		}
		c.DiskPressureTargetFreeBytes = n
	}

	return c, nil
}

func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty value")
	}
	s = strings.TrimSuffix(s, "i")
	s = strings.TrimSuffix(s, "I")
	if s == "" {
		return 0, fmt.Errorf("invalid byte size %q", s)
	}
	suffix := s[len(s)-1]
	var mul int64 = 1
	digits := s
	switch suffix {
	case 'K', 'k':
		mul = 1 << 10
		digits = s[:len(s)-1]
	case 'M', 'm':
		mul = 1 << 20
		digits = s[:len(s)-1]
	case 'G', 'g':
		mul = 1 << 30
		digits = s[:len(s)-1]
	case 'T', 't':
		mul = 1 << 40
		digits = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(digits), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("byte size must be >= 0, got %d", n)
	}
	return n * mul, nil
}

func parseBool(v string) bool {
	switch v {
	case "1", "true", "TRUE", "True", "yes", "on":
		return true
	}
	return false
}
