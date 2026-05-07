package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/navruzm/github-actions-cache-server-go/internal/config"
	"github.com/navruzm/github-actions-cache-server-go/internal/logging"
)

func TestHealth(t *testing.T) {
	h := NewHandler(Deps{
		Cfg:    &config.Config{},
		Logger: logging.New(false),
	})
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "healthy" {
		t.Errorf("status=%d body=%q", resp.StatusCode, body)
	}
}
