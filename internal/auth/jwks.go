package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

type JWKSFetcher struct {
	url    string
	client *http.Client
	ttl    time.Duration

	mu      sync.Mutex
	current *Keyset
	expires time.Time
}

func NewJWKSFetcher(url string) *JWKSFetcher {
	return &JWKSFetcher{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
		ttl:    10 * time.Minute,
	}
}

func (f *JWKSFetcher) Fetch(ctx context.Context) (*Keyset, error) {
	f.mu.Lock()
	if f.current != nil && time.Now().Before(f.expires) {
		ks := f.current
		f.mu.Unlock()
		return ks, nil
	}
	f.mu.Unlock()
	return f.refresh(ctx)
}

func (f *JWKSFetcher) ForceRefresh(ctx context.Context) (*Keyset, error) {
	return f.refresh(ctx)
}

func (f *JWKSFetcher) refresh(ctx context.Context) (*Keyset, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks fetch: %s", resp.Status)
	}
	var raw struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("jwks decode: %w", err)
	}
	keys := map[string]any{}
	for _, k := range raw.Keys {
		switch k.Kty {
		case "RSA":
			n, err := base64.RawURLEncoding.DecodeString(k.N)
			if err != nil {
				continue
			}
			e, err := base64.RawURLEncoding.DecodeString(k.E)
			if err != nil {
				continue
			}
			eInt := 0
			for _, b := range e {
				eInt = eInt<<8 | int(b)
			}
			keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: eInt}
		case "EC":
			if k.Crv != "P-256" {
				continue
			}
			x, _ := base64.RawURLEncoding.DecodeString(k.X)
			y, _ := base64.RawURLEncoding.DecodeString(k.Y)
			keys[k.Kid] = &ecdsa.PublicKey{
				Curve: elliptic.P256(),
				X:     new(big.Int).SetBytes(x),
				Y:     new(big.Int).SetBytes(y),
			}
		}
	}
	ks := &Keyset{keys: keys}
	f.mu.Lock()
	f.current = ks
	f.expires = time.Now().Add(f.ttl)
	f.mu.Unlock()
	return ks, nil
}
