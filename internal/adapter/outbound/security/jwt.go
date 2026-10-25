package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"backend-golang-coding-test/internal/core/domain"
	"backend-golang-coding-test/internal/core/port"
)

// MinSecretLength is the minimum HS256 key size (256 bits).
const MinSecretLength = 32

// JWTService implements port.TokenService using HMAC-SHA256 signed JWTs.
type JWTService struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

var _ port.TokenService = (*JWTService)(nil)

type claims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// NewJWTService validates the configuration and returns a JWTService.
func NewJWTService(secret, issuer string, ttl time.Duration) (*JWTService, error) {
	if len(secret) < MinSecretLength {
		return nil, fmt.Errorf("jwt secret must be at least %d bytes", MinSecretLength)
	}
	if ttl <= 0 {
		return nil, errors.New("jwt ttl must be positive")
	}
	return &JWTService{secret: []byte(secret), issuer: issuer, ttl: ttl, now: time.Now}, nil
}

// Issue signs a token whose subject is the user ID.
func (s *JWTService) Issue(user *domain.User) (string, time.Time, error) {
	now := s.now()
	exp := now.Add(s.ttl)
	c := claims{
		Email: user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, exp, nil
}

// Verify parses the token, rejecting any algorithm other than HS256 (which
// blocks "alg: none" and key-confusion attacks), a wrong issuer, or an
// expired / missing expiry.
func (s *JWTService) Verify(token string) (*port.TokenClaims, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c,
		func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidToken, err)
	}
	if c.Subject == "" {
		return nil, fmt.Errorf("%w: missing subject", domain.ErrInvalidToken)
	}
	return &port.TokenClaims{UserID: c.Subject, Email: c.Email, ExpiresAt: c.ExpiresAt.Time}, nil
}
