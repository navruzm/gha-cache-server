package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const hashEmpty = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func signSigV4(req *http.Request, region, service, accessKey, secretKey, sessionToken string, now time.Time, payloadHash string) {
	if payloadHash == "" {
		payloadHash = hashEmpty
	}
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", sessionToken)
	}
	if req.Host == "" {
		req.Host = req.URL.Host
	}

	canonHeaders, signedHeaders := canonicalHeaders(req)
	canonical := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQuery(req.URL),
		canonHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credScope,
		sha256Hex([]byte(canonical)),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	req.Header.Set("Authorization",
		fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
			accessKey, credScope, signedHeaders, signature))
}

func canonicalURI(u *url.URL) string {
	if u.Path == "" {
		return "/"
	}
	parts := strings.Split(u.Path, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func canonicalQuery(u *url.URL) string {
	if u.RawQuery == "" {
		return ""
	}
	q := u.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		vals := q[k]
		sort.Strings(vals)
		for j, v := range vals {
			if i > 0 || j > 0 {
				b.WriteByte('&')
			}
			b.WriteString(url.QueryEscape(k))
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(v))
		}
	}
	return b.String()
}

func canonicalHeaders(req *http.Request) (string, string) {
	hdrs := []string{}
	values := map[string]string{}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if lk == "host" {
			continue
		}
		hdrs = append(hdrs, lk)
		values[lk] = strings.Join(v, ",")
	}
	hdrs = append(hdrs, "host")
	values["host"] = req.Host
	if req.Host == "" {
		values["host"] = req.URL.Host
	}
	sort.Strings(hdrs)
	var canon strings.Builder
	for _, h := range hdrs {
		canon.WriteString(h)
		canon.WriteByte(':')
		canon.WriteString(strings.TrimSpace(values[h]))
		canon.WriteByte('\n')
	}
	return canon.String(), strings.Join(hdrs, ";")
}

func presignSigV4(req *http.Request, region, service, accessKey, secretKey string, now time.Time, expires time.Duration) string {
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")
	credScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)

	q := req.URL.Query()
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", accessKey+"/"+credScope)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", fmt.Sprintf("%d", int(expires.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Host", req.URL.Host)

	canonical := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQuery(req.URL),
		"host:" + req.URL.Host + "\n",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credScope,
		sha256Hex([]byte(canonical)),
	}, "\n")
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	sig := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))
	q.Set("X-Amz-Signature", sig)
	req.URL.RawQuery = q.Encode()
	return req.URL.String()
}

var _ = io.Reader(nil)
