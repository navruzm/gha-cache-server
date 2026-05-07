package server

import (
	"encoding/json"
	"net/http"

	"github.com/navruzm/github-actions-cache-server-go/internal/auth"
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

type twirpFinalizeBody struct {
	Key       string `json:"key"`
	Version   string `json:"version"`
	SizeBytes string `json:"size_bytes"`
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
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":      false,
				"message": "an upload for this key+version is already in progress",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":                true,
			"signed_upload_url": d.Cfg.APIBaseURL + "/devstoreaccount1/upload/" + strconvFormatInt(u.ID),
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
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":      false,
				"message": "no cache entry matched the requested keys",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":                  true,
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
		var body twirpFinalizeBody
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
			d.Logger.Warn("finalize failed", "key", body.Key, "version", body.Version, "size_bytes", body.SizeBytes, "err", err)
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":      false,
				"message": err.Error(),
			})
			return
		}
		if u == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":      false,
				"message": "no upload found for this key+version",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entry_id": strconvFormatInt(u.ID)})
	}
}
