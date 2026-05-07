//go:build integration

package storage

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/navruzm/github-actions-cache-server-go/internal/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestS3_RoundTrip(t *testing.T) {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "quay.io/minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin",
		},
		WaitingFor: wait.ForLog("API:"),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req, Started: true,
	})
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	defer c.Terminate(ctx)

	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "9000")
	endpoint := "http://" + host + ":" + port.Port()

	// Pre-create the bucket via signed PUT before the adapter's headBucket runs.
	bucket := "vitest"
	createReq, _ := http.NewRequestWithContext(ctx, http.MethodPut, endpoint+"/"+bucket, nil)
	signSigV4(createReq, "us-east-1", "s3", "minioadmin", "minioadmin", "", time.Now(), hashEmpty)
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	createResp.Body.Close()
	if createResp.StatusCode >= 400 && createResp.StatusCode != http.StatusConflict {
		t.Fatalf("create bucket: status %d", createResp.StatusCode)
	}

	cfg := &config.Config{
		StorageDriver: "s3", S3Bucket: bucket, AWSRegion: "us-east-1",
		AWSEndpointURL: endpoint, AWSAccessKeyID: "minioadmin", AWSSecretAccessKey: "minioadmin",
	}
	a, err := NewS3Adapter(cfg)
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
