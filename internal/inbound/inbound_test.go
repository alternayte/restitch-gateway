package inbound

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestCheckAPIKey(t *testing.T) {
	a := &Authenticator{
		apiKeys: [][]byte{[]byte("key1"), []byte("key2")},
	}

	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	t.Run("correct key passes", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-API-Key", "key1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("wrong key rejected", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-API-Key", "wrong")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Errorf("expected 401, got %d", rec.Code)
		}
		if rec.Header().Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("expected WWW-Authenticate: Bearer")
		}
	})

	t.Run("no credentials rejected", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Errorf("expected 401, got %d", rec.Code)
		}
		var body map[string]string
		json.NewDecoder(rec.Body).Decode(&body)
		if body["error"] != "unauthorized" {
			t.Errorf("expected error=unauthorized, got %s", body["error"])
		}
	})
}

// jwksServer starts a test JWKS endpoint serving the public key.
func jwksServer(t *testing.T, key *rsa.PublicKey) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Build a minimal JWKS response
		jwks := map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"use": "sig",
					"alg": "RS256",
					"kid": "test-key",
					"n":   base64URLEncode(key.N.Bytes()),
					"e":   base64URLEncode(big.NewInt(int64(key.E)).Bytes()),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
}

func base64URLEncode(data []byte) string {
	const enc = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	result := make([]byte, 0, (len(data)*4+2)/3)
	for i := 0; i < len(data); i += 3 {
		var val uint32
		n := 0
		for j := 0; j < 3 && i+j < len(data); j++ {
			val = (val << 8) | uint32(data[i+j])
			n++
		}
		val <<= (3 - uint(n)) * 8
		chars := n + 1
		for j := 3; j >= 4-chars; j-- {
			result = append(result, enc[(val>>(uint(j)*6))&0x3f])
		}
	}
	return string(result)
}

func TestJWTValidation(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	jwksSrv := jwksServer(t, &privKey.PublicKey)
	defer jwksSrv.Close()

	a, err := New(t.Context(), InboundAuthConfig{
		JWT: &JWTConfig{
			JWKSURL:  jwksSrv.URL,
			Issuer:   "test-issuer",
			Audience: "test-aud",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r.Context())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(claims)
	}))

	t.Run("valid JWT passes and claims available", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": "user-42",
			"iss": "test-issuer",
			"aud": "test-aud",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "test-key"
		tokenStr, err := token.SignedString(privKey)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != 200 {
			t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
		}

		var claims map[string]any
		json.NewDecoder(rec.Body).Decode(&claims)
		if claims["sub"] != "user-42" {
			t.Errorf("expected sub=user-42, got %v", claims["sub"])
		}
	})

	t.Run("expired JWT rejected", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": "user-42",
			"iss": "test-issuer",
			"aud": "test-aud",
			"exp": time.Now().Add(-time.Hour).Unix(),
		})
		token.Header["kid"] = "test-key"
		tokenStr, err := token.SignedString(privKey)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != 401 {
			t.Errorf("expected 401 for expired JWT, got %d", rec.Code)
		}
	})

	t.Run("wrong issuer rejected", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"sub": "user-42",
			"iss": "wrong-issuer",
			"aud": "test-aud",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "test-key"
		tokenStr, err := token.SignedString(privKey)
		if err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+tokenStr)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != 401 {
			t.Errorf("expected 401 for wrong issuer, got %d", rec.Code)
		}
	})
}

func TestGetClaims_Nil(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	c := GetClaims(req.Context())
	if c != nil {
		t.Errorf("expected nil claims from empty context, got %v", c)
	}
}
