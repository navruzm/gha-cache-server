package storage

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/navruzm/gha-cache-server/internal/config"
)

type GCSAdapter struct {
	bucket   string
	endpoint string
	prefix   string
	client   *http.Client

	creds   *gcsCreds
	tokenMu sync.Mutex
	token   string
	tokExp  time.Time
}

type gcsCreds struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

func NewGCSAdapter(cfg *config.Config) (Adapter, error) {
	if cfg.GCSBucket == "" {
		return nil, errors.New("GCS bucket required")
	}
	endpoint := cfg.GCSEndpoint
	if endpoint == "" {
		endpoint = "https://storage.googleapis.com"
	}
	a := &GCSAdapter{
		bucket:   cfg.GCSBucket,
		endpoint: strings.TrimRight(endpoint, "/"),
		prefix:   "gh-actions-cache",
		client:   &http.Client{},
	}
	if cfg.GCSServiceAccountKey != "" {
		raw, err := os.ReadFile(cfg.GCSServiceAccountKey)
		if err != nil {
			return nil, err
		}
		var c gcsCreds
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		a.creds = &c
	}
	if err := a.headBucket(context.Background()); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *GCSAdapter) authToken(ctx context.Context) (string, error) {
	if a.creds == nil {
		return "", nil
	}
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()
	if a.token != "" && time.Now().Before(a.tokExp.Add(-time.Minute)) {
		return a.token, nil
	}
	block, _ := pem.Decode([]byte(a.creds.PrivateKey))
	if block == nil {
		return "", errors.New("invalid private key")
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	pk, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return "", errors.New("not RSA private key")
	}
	header := `{"alg":"RS256","typ":"JWT"}`
	now := time.Now()
	claims := map[string]any{
		"iss":   a.creds.ClientEmail,
		"scope": "https://www.googleapis.com/auth/devstorage.read_write",
		"aud":   a.creds.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	cb, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding.EncodeToString
	signing := enc([]byte(header)) + "." + enc(cb)
	h := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(nil, pk, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	jwt := signing + "." + enc(sig)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", jwt)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.creds.TokenURI, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gcs token exchange: %d %s", resp.StatusCode, body)
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	a.token = t.AccessToken
	a.tokExp = now.Add(time.Duration(t.ExpiresIn) * time.Second)
	return a.token, nil
}

func (a *GCSAdapter) authReq(ctx context.Context, method, u string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	tok, err := a.authToken(ctx)
	if err != nil {
		return nil, err
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return req, nil
}

func (a *GCSAdapter) headBucket(ctx context.Context) error {
	u := a.endpoint + "/storage/v1/b/" + url.PathEscape(a.bucket)
	req, err := a.authReq(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("head bucket: %d %s", resp.StatusCode, body)
	}
	return nil
}

func (a *GCSAdapter) objectURL(name string) string {
	return a.endpoint + "/storage/v1/b/" + url.PathEscape(a.bucket) +
		"/o/" + url.PathEscape(a.prefix+"/"+name) + "?alt=media"
}

func (a *GCSAdapter) CreateDownloadStream(ctx context.Context, name string) (io.ReadCloser, error) {
	req, err := a.authReq(ctx, http.MethodGet, a.objectURL(name), nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, ErrObjectNotFound
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("gcs get: %d %s", resp.StatusCode, body)
	}
	return resp.Body, nil
}

func (a *GCSAdapter) UploadStream(ctx context.Context, name string, body io.Reader) error {
	uploadURL := a.endpoint + "/upload/storage/v1/b/" + url.PathEscape(a.bucket) +
		"/o?uploadType=media&name=" + url.QueryEscape(a.prefix+"/"+name)
	buf, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	req, err := a.authReq(ctx, http.MethodPost, uploadURL, strings.NewReader(string(buf)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(buf))
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gcs upload: %d %s", resp.StatusCode, b)
	}
	return nil
}

func (a *GCSAdapter) listPrefix(ctx context.Context, prefix string) ([]string, error) {
	pageToken := ""
	var keys []string
	for {
		u := a.endpoint + "/storage/v1/b/" + url.PathEscape(a.bucket) + "/o?prefix=" + url.QueryEscape(prefix)
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		req, err := a.authReq(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := a.client.Do(req)
		if err != nil {
			return nil, err
		}
		var page struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
			NextPageToken string `json:"nextPageToken"`
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			keys = append(keys, it.Name)
		}
		if page.NextPageToken == "" {
			return keys, nil
		}
		pageToken = page.NextPageToken
	}
}

func (a *GCSAdapter) deleteOne(ctx context.Context, key string) error {
	u := a.endpoint + "/storage/v1/b/" + url.PathEscape(a.bucket) + "/o/" + url.PathEscape(key)
	req, err := a.authReq(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gcs delete %s: %d %s", key, resp.StatusCode, b)
	}
	return nil
}

func (a *GCSAdapter) DeleteFolder(ctx context.Context, name string) error {
	keys, err := a.listPrefix(ctx, a.prefix+"/"+name+"/")
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := a.deleteOne(ctx, k); err != nil {
			return err
		}
	}
	return nil
}

func (a *GCSAdapter) Clear(ctx context.Context) error {
	keys, err := a.listPrefix(ctx, a.prefix+"/")
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := a.deleteOne(ctx, k); err != nil {
			return err
		}
	}
	return nil
}

func (a *GCSAdapter) CountFilesInFolder(ctx context.Context, name string) (int, error) {
	keys, err := a.listPrefix(ctx, a.prefix+"/"+name+"/")
	return len(keys), err
}

func (a *GCSAdapter) CreateDownloadURL(_ context.Context, _ string) (string, bool, error) {
	return "", false, nil
}
