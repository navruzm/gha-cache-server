package server

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestUpload_PutBlockAndCommit(t *testing.T) {
	srv, svc, _ := newTestServer(t)
	u, _ := svc.CreateUpload(context.Background(), "k", "v", "main", "42")

	b := make([]byte, 64)
	binary.BigEndian.PutUint32(b[16:20], 0)
	blockID := base64.StdEncoding.EncodeToString(b)

	url := srv.URL + "/devstoreaccount1/upload/" + strconv.FormatInt(u.ID, 10) + "?blockid=" + blockID
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader("payload-zero"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if resp.Header.Get("x-ms-request-id") == "" {
		t.Error("missing x-ms-request-id")
	}

	commitURL := srv.URL + "/devstoreaccount1/upload/" + strconv.FormatInt(u.ID, 10) + "?comp=blocklist"
	req, _ = http.NewRequest(http.MethodPut, commitURL, strings.NewReader(""))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("commit status=%d", resp.StatusCode)
	}
}

func TestUpload_AliasPath(t *testing.T) {
	srv, svc, _ := newTestServer(t)
	u, _ := svc.CreateUpload(context.Background(), "k", "v", "main", "42")
	url := srv.URL + "/upload/" + strconv.FormatInt(u.ID, 10) + "?comp=blocklist"
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader(""))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
