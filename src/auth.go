package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTClaims struct {
	jwt.RegisteredClaims
}

func newJWTClaims(iss string, iat, exp *jwt.NumericDate) JWTClaims {
	return JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    iss,
			IssuedAt:  iat,
			ExpiresAt: exp,
		},
	}
}

func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		key2, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("parse private key (PKCS1 or PKCS8): %w", err2)
		}
		var ok bool
		key, ok = key2.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
	}
	return key, nil
}

func createJWT(appId string, key *rsa.PrivateKey) (string, error) {
	now := time.Now()
	iat := jwt.NewNumericDate(now)
	exp := jwt.NewNumericDate(now.Add(10 * time.Minute))
	claims := newJWTClaims(appId, iat, exp)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(key)
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

type installationTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

func getInstallationAccessToken(appId string, installationID string, privateKeyPEM string) (string, error) {
	key, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", fmt.Errorf("private key: %w", err)
	}

	jwtToken, err := createJWT(appId, key)
	if err != nil {
		return "", fmt.Errorf("create JWT: %w", err)
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%s/access_tokens", installationID)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+jwtToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("failed to get installation access token: %s: %s", resp.Status, string(body))
	}

	var payload installationTokenResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return payload.Token, nil
}

func GetInstallationAccessToken(appId string, installationID string, privateKey string) (string, error) {
	return getInstallationAccessToken(appId, installationID, privateKey)
}
