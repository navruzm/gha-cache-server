//go:build integration

package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/navruzm/gha-cache-server/internal/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestGCS_RoundTrip(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "fsouza/fake-gcs-server:latest",
		ExposedPorts: []string{"4443/tcp"},
		Cmd:          []string{"-scheme", "http", "-port", "4443", "-public-host", "localhost"},
		WaitingFor:   wait.ForLog("server started"),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req, Started: true,
	})
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	defer c.Terminate(ctx)

	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "4443")
	endpoint := "http://" + host + ":" + port.Port()

	bucket := "vitest"
	// Pre-create the bucket via fake-gcs-server's HTTP API before constructing the adapter.
	createReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/storage/v1/b",
		bytes.NewReader([]byte(`{"name":"`+bucket+`"}`)))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	createResp.Body.Close()
	if createResp.StatusCode >= 400 && createResp.StatusCode != http.StatusConflict {
		t.Fatalf("create bucket: status %d", createResp.StatusCode)
	}

	cfg := &config.Config{StorageDriver: "gcs", GCSBucket: bucket, GCSEndpoint: endpoint}
	a, err := NewGCSAdapter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.UploadStream(ctx, "test/parts/0", strings.NewReader("hello")); err != nil {
		t.Fatalf("upload: %v", err)
	}
	r, err := a.CreateDownloadStream(ctx, "test/parts/0")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	body, _ := io.ReadAll(r)
	if string(body) != "hello" {
		t.Errorf("got %q", body)
	}
}
