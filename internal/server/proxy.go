package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

func registerProxy(mux *http.ServeMux, d Deps) {
	if d.Cfg.DefaultActionsResultsURL == "" {
		mux.HandleFunc("/", http.NotFound)
		return
	}
	target, err := url.Parse(d.Cfg.DefaultActionsResultsURL)
	if err != nil {
		mux.HandleFunc("/", http.NotFound)
		return
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	originalDirector := rp.Director
	rp.Director = func(r *http.Request) {
		originalDirector(r)
		r.Host = target.Host
	}
	mux.Handle("/", rp)
}
