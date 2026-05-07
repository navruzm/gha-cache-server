package server

import (
	"net/http"
	"strconv"

	"github.com/navruzm/gha-cache-server/internal/ids"
)

func registerUpload(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("PUT /devstoreaccount1/upload/{uploadId}", uploadHandler(d))
	mux.HandleFunc("PUT /upload/{uploadId}", uploadHandler(d))
}

func uploadHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-ms-request-id", ids.UUIDv4())

		uploadID, err := strconv.ParseInt(r.PathValue("uploadId"), 10, 64)
		if err != nil {
			http.Error(w, "invalid upload id", http.StatusBadRequest)
			return
		}

		q := r.URL.Query()
		if q.Get("comp") == "blocklist" {
			w.WriteHeader(http.StatusCreated)
			return
		}

		blockID := q.Get("blockid")
		index := 0
		if blockID != "" {
			n, ok := chunkIndexFromBlockID(blockID)
			if !ok {
				http.Error(w, "invalid block id", http.StatusBadRequest)
				return
			}
			index = n
		}

		if err := d.Storage.UploadPart(r.Context(), uploadID, index, r.Body); err != nil {
			d.Logger.Error("uploadPart", "err", err)
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}
