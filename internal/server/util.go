package server

import (
	"strconv"

	"github.com/navruzm/gha-cache-server/internal/auth"
	"github.com/navruzm/gha-cache-server/internal/storage"
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
