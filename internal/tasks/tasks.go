package tasks

import (
	"github.com/navruzm/gha-cache-server/internal/config"
	dbpkg "github.com/navruzm/gha-cache-server/internal/db"
	"github.com/navruzm/gha-cache-server/internal/storage"
)

type Deps struct {
	Cfg     *config.Config
	Queries *dbpkg.Queries
	Storage *storage.Service
}

const pageSize = 10
