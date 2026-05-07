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

	return c, nil
}

func parseBool(v string) bool {
	switch v {
	case "1", "true", "TRUE", "True", "yes", "on":
		return true
	}
	return false
}
