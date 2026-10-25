package security

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"backend-golang-coding-test/internal/core/domain"
)

const testSecret = "test-secret-that-is-at-least-32-bytes-long"

func newTestJWT(t *testing.T, now time.Time) *JWTService {
	t.Helper()
	s, err := NewJWTService(testSecret, "test-issuer", time.Hour)
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	s.now = func() time.Time { return now }
	return s
}

func TestNewJWTService_Config(t *testing.T) {
	if _, err := NewJWTService("too-short", "iss", time.Hour); err == nil {
		t.Error("expected error for short secret")
	}
	if _, err := NewJWTService(testSecret, "iss", 0); err == nil {
		t.Error("expected error for zero ttl")
	}
}

func TestJWT_RoundTrip(t *testing.T) {
	now := time.Now()
	s := newTestJWT(t, now)

	token, exp, err := s.Issue(&domain.User{ID: "id-1", Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !exp.Equal(now.Add(time.Hour)) {
		t.Errorf("exp = %v", exp)
	}
	parsed, _, err := jwt.NewParser().ParseUnverified(token, jwt.MapClaims{})
	if err != nil || parsed.Method.Alg() != "HS256" {
		t.Fatalf("token alg = %v, err = %v", parsed.Method.Alg(), err)
	}

	claims, err := s.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != "id-1" || claims.Email != "alice@example.com" {
		t.Errorf("claims = %+v", claims)
	}
}

func TestJWT_VerifyRejects(t *testing.T) {
	now := time.Now()
	s := newTestJWT(t, now)
	valid, _, _ := s.Issue(&domain.User{ID: "id-1"})

	sign := func(method jwt.SigningMethod, key any, c jwt.MapClaims) string {
		tok, err := jwt.NewWithClaims(method, c).SignedString(key)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return tok
	}
	base := func() jwt.MapClaims {
		return jwt.MapClaims{"sub": "id-1", "iss": "test-issuer", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}
	}
	with := func(k string, v any) jwt.MapClaims { c := base(); c[k] = v; return c }
	without := func(k string) jwt.MapClaims { c := base(); delete(c, k); return c }

	tests := map[string]string{
		"garbage":          "not-a-jwt",
		"tampered payload": valid[:len(valid)-2] + "xx",
		"wrong secret":     sign(jwt.SigningMethodHS256, []byte(strings.Repeat("x", 32)), base()),
		"alg none":         sign(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, base()),
		"alg HS512":        sign(jwt.SigningMethodHS512, []byte(testSecret), base()),
		"expired":          sign(jwt.SigningMethodHS256, []byte(testSecret), with("exp", now.Add(-time.Minute).Unix())),
		"no expiry":        sign(jwt.SigningMethodHS256, []byte(testSecret), without("exp")),
		"wrong issuer":     sign(jwt.SigningMethodHS256, []byte(testSecret), with("iss", "evil")),
		"missing subject":  sign(jwt.SigningMethodHS256, []byte(testSecret), without("sub")),
		"issued in future": sign(jwt.SigningMethodHS256, []byte(testSecret), with("iat", now.Add(time.Hour).Unix())),
	}
	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Verify(token); !errors.Is(err, domain.ErrInvalidToken) {
				t.Errorf("Verify err = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func TestBcryptHasher(t *testing.T) {
	h := NewBcryptHasher(bcrypt.MinCost)
	hash, err := h.Hash("password123")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash == "password123" || !strings.HasPrefix(hash, "$2a$") {
		t.Errorf("unexpected hash %q", hash)
	}
	if err := h.Compare(hash, "password123"); err != nil {
		t.Errorf("Compare(correct) = %v", err)
	}
	if err := h.Compare(hash, "wrong"); err == nil {
		t.Error("Compare(wrong) succeeded")
	}
	if NewBcryptHasher(1000).cost != bcrypt.DefaultCost {
		t.Error("out-of-range cost should fall back to default")
	}
}
