package storage

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/navruzm/gha-cache-server/internal/config"
)

type S3Adapter struct {
	bucket    string
	region    string
	endpoint  string
	keyID     string
	keySecret string
	prefix    string
	client    *http.Client
}

func NewS3Adapter(cfg *config.Config) (Adapter, error) {
	if cfg.S3Bucket == "" {
		return nil, errors.New("S3 bucket required")
	}
	endpoint := cfg.AWSEndpointURL
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.AWSRegion)
	}
	a := &S3Adapter{
		bucket:    cfg.S3Bucket,
		region:    cfg.AWSRegion,
		endpoint:  strings.TrimRight(endpoint, "/"),
		keyID:     cfg.AWSAccessKeyID,
		keySecret: cfg.AWSSecretAccessKey,
		prefix:    "gh-actions-cache",
		client:    &http.Client{Timeout: 0},
	}
	if err := a.headBucket(context.Background()); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *S3Adapter) url(key string) *url.URL {
	u, _ := url.Parse(a.endpoint + "/" + a.bucket + "/" + key)
	return u
}

func (a *S3Adapter) sign(req *http.Request, payloadHash string) {
	signSigV4(req, a.region, "s3", a.keyID, a.keySecret, "", time.Now(), payloadHash)
}

func (a *S3Adapter) do(req *http.Request) (*http.Response, error) {
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("s3 %s %s: %d %s", req.Method, req.URL.Path, resp.StatusCode, string(body))
	}
	return resp, nil
}

func (a *S3Adapter) headBucket(ctx context.Context) error {
	u, _ := url.Parse(a.endpoint + "/" + a.bucket)
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
	a.sign(req, hashEmpty)
	resp, err := a.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (a *S3Adapter) CreateDownloadStream(ctx context.Context, name string) (io.ReadCloser, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.url(a.prefix+"/"+name).String(), nil)
	a.sign(req, hashEmpty)
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
		return nil, fmt.Errorf("s3 get: %d %s", resp.StatusCode, body)
	}
	return resp.Body, nil
}

func (a *S3Adapter) UploadStream(ctx context.Context, name string, body io.Reader) error {
	buf, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, a.url(a.prefix+"/"+name).String(), bytes.NewReader(buf))
	req.ContentLength = int64(len(buf))
	a.sign(req, sha256Hex(buf))
	resp, err := a.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (a *S3Adapter) DeleteFolder(ctx context.Context, name string) error {
	return a.deleteByPrefix(ctx, a.prefix+"/"+name+"/")
}

func (a *S3Adapter) Clear(ctx context.Context) error {
	return a.deleteByPrefix(ctx, a.prefix+"/")
}

type listResult struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	Contents []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
}

func (a *S3Adapter) listObjects(ctx context.Context, prefix, token string) (*listResult, error) {
	u, _ := url.Parse(a.endpoint + "/" + a.bucket)
	q := u.Query()
	q.Set("list-type", "2")
	q.Set("prefix", prefix)
	if token != "" {
		q.Set("continuation-token", token)
	}
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	a.sign(req, hashEmpty)
	resp, err := a.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var lr listResult
	if err := xml.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, err
	}
	return &lr, nil
}

func (a *S3Adapter) deleteByPrefix(ctx context.Context, prefix string) error {
	token := ""
	for {
		lr, err := a.listObjects(ctx, prefix, token)
		if err != nil {
			return err
		}
		if len(lr.Contents) == 0 {
			return nil
		}
		var keys []string
		for _, c := range lr.Contents {
			keys = append(keys, c.Key)
		}
		if err := a.deleteObjects(ctx, keys); err != nil {
			return err
		}
		if !lr.IsTruncated {
			return nil
		}
		token = lr.NextContinuationToken
	}
}

func (a *S3Adapter) deleteObjects(ctx context.Context, keys []string) error {
	type obj struct {
		Key string `xml:"Key"`
	}
	type del struct {
		XMLName xml.Name `xml:"Delete"`
		Object  []obj    `xml:"Object"`
		Quiet   bool     `xml:"Quiet"`
	}
	for i := 0; i < len(keys); i += 1000 {
		end := i + 1000
		if end > len(keys) {
			end = len(keys)
		}
		batch := del{Quiet: true}
		for _, k := range keys[i:end] {
			batch.Object = append(batch.Object, obj{Key: k})
		}
		body, _ := xml.Marshal(batch)
		u, _ := url.Parse(a.endpoint + "/" + a.bucket + "?delete=")
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
		a.sign(req, sha256Hex(body))
		resp, err := a.do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (a *S3Adapter) CountFilesInFolder(ctx context.Context, name string) (int, error) {
	prefix := a.prefix + "/" + name + "/"
	count := 0
	token := ""
	for {
		lr, err := a.listObjects(ctx, prefix, token)
		if err != nil {
			return 0, err
		}
		count += len(lr.Contents)
		if !lr.IsTruncated {
			return count, nil
		}
		token = lr.NextContinuationToken
	}
}

func (a *S3Adapter) CreateDownloadURL(_ context.Context, name string) (string, bool, error) {
	u := a.url(a.prefix + "/" + name)
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	signed := presignSigV4(req, a.region, "s3", a.keyID, a.keySecret, time.Now(), 10*time.Minute)
	return signed, true, nil
}
