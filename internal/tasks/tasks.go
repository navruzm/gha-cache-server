package tasks

import (
	"github.com/navruzm/github-actions-cache-server-go/internal/config"
	dbpkg "github.com/navruzm/github-actions-cache-server-go/internal/db"
	"github.com/navruzm/github-actions-cache-server-go/internal/storage"
)

type Deps struct {
	Cfg     *config.Config
	Queries *dbpkg.Queries
	Storage *storage.Service
}

const pageSize = 10
