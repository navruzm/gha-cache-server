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
