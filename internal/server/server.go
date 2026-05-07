package server

import (
	"log/slog"
	"net/http"

	"github.com/navruzm/github-actions-cache-server-go/internal/auth"
	"github.com/navruzm/github-actions-cache-server-go/internal/config"
	"github.com/navruzm/github-actions-cache-server-go/internal/storage"
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
