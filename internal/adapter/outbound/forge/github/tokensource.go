package github

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	gh "github.com/google/go-github/v90/github"
)

type TokenSource struct {
	appID      int64
	privateKey any
	mutex      sync.Mutex
	tokens     map[int64]cachedToken
}

func NewTokenSource(appID int64, privateKeyPEM []byte) (*TokenSource, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("앱 개인키를 읽지 못했습니다: %w", err)
	}
	return &TokenSource{appID: appID, privateKey: key, tokens: map[int64]cachedToken{}}, nil
}

func (s *TokenSource) InstallationToken(ctx context.Context, installationID int64) (string, error) {
	s.mutex.Lock()
	cached, ok := s.tokens[installationID]
	s.mutex.Unlock()
	if ok && cached.valid(time.Now()) {
		return cached.value, nil
	}

	client, err := s.AppClient()
	if err != nil {
		return "", err
	}
	token, _, err := client.Apps.CreateInstallationToken(ctx, installationID, nil)
	if err != nil {
		return "", fmt.Errorf("설치 토큰을 발급하지 못했습니다: %w", err)
	}

	s.mutex.Lock()
	s.tokens[installationID] = cachedToken{value: token.GetToken(), expiresAt: token.GetExpiresAt().Time}
	s.mutex.Unlock()
	return token.GetToken(), nil
}

func (s *TokenSource) AppClient() (*gh.Client, error) {
	appJWT, err := s.appJWT()
	if err != nil {
		return nil, err
	}
	client, err := gh.NewClient(gh.WithTimeout(githubRequestTimeout), gh.WithAuthToken(appJWT))
	if err != nil {
		return nil, fmt.Errorf("GitHub 앱 클라이언트를 만들지 못했습니다: %w", err)
	}
	return client, nil
}

func (s *TokenSource) appJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
		Issuer:    strconv.FormatInt(s.appID, 10),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(s.privateKey)
	if err != nil {
		return "", fmt.Errorf("앱 JWT를 만들지 못했습니다: %w", err)
	}
	return signed, nil
}
