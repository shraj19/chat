package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const userContextKey = contextKey("userID")

type JWTMaker struct {
	secretKey string
}

type Claims struct {
	jwt.RegisteredClaims
}

type InvalidTokenError struct {
	Reason string
	Err    error
}

func (e *InvalidTokenError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("invalid token: %s, underlying error: %v", e.Reason, e.Err)
	}
	return fmt.Sprintf("invalid token: %s", e.Reason)
}

type JWTSecretKeyTooShortError struct {
	Length int
}

func (e *JWTSecretKeyTooShortError) Error() string {
	return fmt.Sprintf("JWT secret key is too short; must be at least 32 characters, got %d", e.Length)
}

var ErrInvalidUserID = errors.New("invalid user ID")
var ErrInvalidTokenDuration = errors.New("invalid token duration")

type AccessTokenError struct {
	Message string
	Err     error
}

func (e *AccessTokenError) Error() string {
	return e.Message
}

func NewJWTMaker(secretKey string) (*JWTMaker, error) {
	if len(secretKey) < 32 {
		return nil, &JWTSecretKeyTooShortError{Length: len(secretKey)}
	}

	return &JWTMaker{secretKey: secretKey}, nil
}

func (m *JWTMaker) CreateToken(userID uuid.UUID, duration time.Duration) (string, error) {
	if userID == uuid.Nil {
		return "", ErrInvalidUserID
	}
	if duration <= 0 {
		return "", ErrInvalidTokenDuration
	}

	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    "relay",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.secretKey))
}

func (m *JWTMaker) VerifyToken(tokenStr string) (*Claims, error) {
	if tokenStr == "" {
		return nil, &InvalidTokenError{Reason: "empty token string"}
	}

	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, &InvalidTokenError{Reason: "unexpected signing method"}
		}
		return []byte(m.secretKey), nil
	})

	if err != nil {
		return nil, &InvalidTokenError{Reason: "parse failed", Err: err}
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, &InvalidTokenError{Reason: "invalid claims", Err: nil}
	}

	if claims.Subject == "" {
		return nil, &InvalidTokenError{Reason: "invalid subject in claims", Err: nil}
	}

	return claims, nil
}

// UserID extracts the user ID from the claims Subject field.
func (c *Claims) UserID() (uuid.UUID, error) {
	return uuid.Parse(c.Subject)
}

func ExtractTokenFromCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie("access_token")
	if err != nil {
		return "", &AccessTokenError{Message: "access_token cookie not found", Err: err}
	}
	if cookie.Value == "" {
		return "", &AccessTokenError{Message: "access_token cookie is empty", Err: nil}
	}
	return cookie.Value, nil
}

// Context key for user ID
func SetUserInContext(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, userContextKey, userID)
}

func GetUserFromContext(ctx context.Context) (uuid.UUID, bool) {
	if ctx == nil {
		return uuid.Nil, false
	}
	userID, ok := ctx.Value(userContextKey).(uuid.UUID)
	return userID, ok
}
