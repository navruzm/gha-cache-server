package server

import (
	"net/http"
	"strconv"

	dbpkg "github.com/navruzm/gha-cache-server/internal/db"
	"github.com/navruzm/gha-cache-server/internal/storage"
)

func registerManagement(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("GET /management-api/_docs", mgmtDocsHTML)
	mux.HandleFunc("GET /management-api/_docs/spec.json", mgmtDocsSpec(d))
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
