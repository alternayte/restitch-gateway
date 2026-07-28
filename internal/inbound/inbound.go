package inbound

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// InboundAuthConfig configures gateway-level inbound authentication.
type InboundAuthConfig struct {
	APIKeys []string   `yaml:"api_keys"`
	JWT     *JWTConfig `yaml:"jwt"`
}

// JWTConfig configures JWT validation via JWKS.
type JWTConfig struct {
	JWKSURL  string `yaml:"jwks_url"`
	Issuer   string `yaml:"issuer"`
	Audience string `yaml:"audience"`
}

type claimsKey struct{}

// WithClaims stores JWT claims in the context.
func WithClaims(ctx context.Context, claims jwt.MapClaims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

// GetClaims retrieves JWT claims from the context (nil if absent).
func GetClaims(ctx context.Context) jwt.MapClaims {
	if c, ok := ctx.Value(claimsKey{}).(jwt.MapClaims); ok {
		return c
	}
	return nil
}

// Authenticator validates inbound requests via API keys and/or JWT.
type Authenticator struct {
	apiKeys [][]byte
	kf      keyfunc.Keyfunc
	issuer  string
	aud     string
}

// New builds the authenticator. When cfg.JWT is set, it constructs a JWKS
// keyfunc for token validation. Returns error if JWKS is unreachable.
func New(ctx context.Context, cfg InboundAuthConfig) (*Authenticator, error) {
	a := &Authenticator{
		apiKeys: make([][]byte, len(cfg.APIKeys)),
	}
	for i, k := range cfg.APIKeys {
		a.apiKeys[i] = []byte(k)
	}

	if cfg.JWT != nil {
		kf, err := keyfunc.NewDefaultCtx(ctx, []string{cfg.JWT.JWKSURL})
		if err != nil {
			return nil, err
		}
		a.kf = kf
		a.issuer = cfg.JWT.Issuer
		a.aud = cfg.JWT.Audience
	}

	return a, nil
}

// Middleware wraps a handler with inbound auth checking.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try API key first
		if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
			if a.checkAPIKey([]byte(apiKey)) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Try JWT bearer token
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, ok := a.validateJWT(tokenStr)
			if ok {
				ctx := WithClaims(r.Context(), claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Neither passed
		w.Header().Set("WWW-Authenticate", "Bearer")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	})
}

// checkAPIKey checks if the provided key matches any configured key using
// constant-time comparison.
func (a *Authenticator) checkAPIKey(key []byte) bool {
	for _, k := range a.apiKeys {
		if subtle.ConstantTimeCompare(key, k) == 1 {
			return true
		}
	}
	return false
}

// validateJWT parses and validates a JWT token string. Returns claims and
// true on success.
func (a *Authenticator) validateJWT(tokenStr string) (jwt.MapClaims, bool) {
	if a.kf == nil {
		return nil, false
	}

	var opts []jwt.ParserOption
	opts = append(opts, jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}))
	if a.issuer != "" {
		opts = append(opts, jwt.WithIssuer(a.issuer))
	}
	if a.aud != "" {
		opts = append(opts, jwt.WithAudience(a.aud))
	}

	token, err := jwt.Parse(tokenStr, a.kf.KeyfuncCtx(context.Background()), opts...)
	if err != nil || !token.Valid {
		return nil, false
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, false
	}

	return claims, true
}
