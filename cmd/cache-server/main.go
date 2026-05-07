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

	"github.com/navruzm/gha-cache-server/internal/auth"
	"github.com/navruzm/gha-cache-server/internal/config"
	"github.com/navruzm/gha-cache-server/internal/cron"
	dbpkg "github.com/navruzm/gha-cache-server/internal/db"
	"github.com/navruzm/gha-cache-server/internal/logging"
	"github.com/navruzm/gha-cache-server/internal/server"
	"github.com/navruzm/gha-cache-server/internal/storage"
	"github.com/navruzm/gha-cache-server/internal/tasks"
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
	scheduler.Every(5*time.Minute, "cleanup:disk-pressure", tasks.CleanupDiskPressure(taskDeps))
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
