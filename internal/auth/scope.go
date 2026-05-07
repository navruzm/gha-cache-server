package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Scope struct {
	Scope      string `json:"Scope"`
	Permission int    `json:"Permission"`
}

type AuthResult struct {
	RepoID string
	Scopes []Scope
}

type Verifier struct {
	fetcher  *JWKSFetcher
	issuer   string
	skipAuth bool
}

func NewVerifier(f *JWKSFetcher, issuer string, skip bool) *Verifier {
	return &Verifier{fetcher: f, issuer: issuer, skipAuth: skip}
}

var ErrUnauthorized = errors.New("unauthorized")

func (v *Verifier) Authorize(ctx context.Context, authzHeader string) (*AuthResult, error) {
	if !strings.HasPrefix(authzHeader, "Bearer ") {
		return nil, fmt.Errorf("%w: missing or malformed Authorization header", ErrUnauthorized)
	}
	token := strings.TrimPrefix(authzHeader, "Bearer ")

	var claims Claims
	var err error
	if v.skipAuth {
		claims, err = DecodeUnverified(token)
	} else {
		ks, ferr := v.fetcher.Fetch(ctx)
		if ferr != nil {
			return nil, fmt.Errorf("jwks fetch: %w", ferr)
		}
		claims, err = Verify(token, ks, v.issuer)
		if errors.Is(err, ErrUnknownKey) {
			ks, ferr = v.fetcher.ForceRefresh(ctx)
			if ferr == nil {
				claims, err = Verify(token, ks, v.issuer)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}

	acRaw, _ := claims["ac"].(string)
	if acRaw == "" {
		return nil, fmt.Errorf("%w: token missing cache scopes", ErrUnauthorized)
	}
	var scopes []Scope
	if err := json.Unmarshal([]byte(acRaw), &scopes); err != nil {
		return nil, fmt.Errorf("%w: invalid scopes JSON", ErrUnauthorized)
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("%w: empty scopes", ErrUnauthorized)
	}
	repoID, _ := claims["repository_id"].(string)
	if repoID == "" {
		if f, ok := claims["repository_id"].(float64); ok {
			repoID = fmt.Sprintf("%d", int64(f))
		}
	}
	if repoID == "" {
		return nil, fmt.Errorf("%w: token missing repository_id", ErrUnauthorized)
	}
	return &AuthResult{RepoID: repoID, Scopes: scopes}, nil
}

func WriteScope(scopes []Scope) (Scope, bool) {
	for _, s := range scopes {
		if s.Permission >= 2 {
			return s, true
		}
	}
	return Scope{}, false
}

func ScopesByPermissionDesc(scopes []Scope) []string {
	cp := append([]Scope(nil), scopes...)
	for i := 1; i < len(cp); i++ {
		for j := i; j > 0 && cp[j].Permission > cp[j-1].Permission; j-- {
			cp[j], cp[j-1] = cp[j-1], cp[j]
		}
	}
	out := make([]string, len(cp))
	for i, s := range cp {
		out[i] = s.Scope
	}
	return out
}
