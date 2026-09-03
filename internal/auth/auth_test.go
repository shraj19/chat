package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"chat-v2/internal/auth"
)

const testSecret = "test-secret-key-that-is-at-least-32-bytes-long"

func newTestMaker(t *testing.T) *auth.JWTMaker {
	t.Helper()
	maker, err := auth.NewJWTMaker(testSecret)
	if err != nil {
		t.Fatalf("NewJWTMaker() error: %v", err)
	}
	return maker
}

// --- JWTMaker Tests ---

func TestNewJWTMaker_EmptySecret(t *testing.T) {
	_, err := auth.NewJWTMaker("")
	if err == nil {
		t.Fatal("expected error for empty secret, got nil")
	}
}

func TestCreateToken_Valid(t *testing.T) {
	maker := newTestMaker(t)
	userID := uuid.New()

	token, err := maker.CreateToken(userID, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken() error: %v", err)
	}
	if token == "" {
		t.Fatal("CreateToken() returned empty token")
	}
}

func TestCreateToken_InvalidInputs(t *testing.T) {
	maker := newTestMaker(t)

	tests := []struct {
		name     string
		userID   uuid.UUID
		duration time.Duration
	}{
		{"nil userID", uuid.Nil, time.Hour},
		{"zero duration", uuid.New(), 0},
		{"negative duration", uuid.New(), -time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := maker.CreateToken(tt.userID, tt.duration)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestVerifyToken_Valid(t *testing.T) {
	maker := newTestMaker(t)
	userID := uuid.New()

	token, err := maker.CreateToken(userID, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken() error: %v", err)
	}

	claims, err := maker.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken() error: %v", err)
	}

	gotUserID, err := claims.UserID()
	if err != nil {
		t.Fatalf("claims.UserID() error: %v", err)
	}
	if gotUserID != userID {
		t.Errorf("claims.UserID() = %v, want %v", gotUserID, userID)
	}
	if claims.Issuer != "relay" {
		t.Errorf("claims.Issuer = %q, want %q", claims.Issuer, "relay")
	}
	if claims.Subject != userID.String() {
		t.Errorf("claims.Subject = %q, want %q", claims.Subject, userID.String())
	}
}

func TestVerifyToken_InvalidInputs(t *testing.T) {
	maker := newTestMaker(t)

	// Create a token with different secret for "wrong secret" test
	otherMaker, _ := auth.NewJWTMaker("different-secret-key-that-is-also-32-bytes")
	wrongSecretToken, _ := otherMaker.CreateToken(uuid.New(), time.Hour)

	// Create expired token
	expiredToken, _ := maker.CreateToken(uuid.New(), time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"malformed token", "not.a.valid.token"},
		{"wrong secret", wrongSecretToken},
		{"expired token", expiredToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := maker.VerifyToken(tt.token)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

// --- Password Hashing Tests ---

func TestHashPassword_Valid(t *testing.T) {
	hash, err := auth.HashPassword("mypassword123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword() returned empty hash")
	}
	if hash == "mypassword123" {
		t.Fatal("HashPassword() returned plaintext password")
	}
}

func TestCheckPassword(t *testing.T) {
	hash, err := auth.HashPassword("correctpassword")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}

	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{"correct password", "correctpassword", true},
		{"wrong password", "wrongpassword", false},
		{"empty password", "", false},
		{"similar password", "correctpassword1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := auth.CheckPassword(tt.password, hash)
			if got != tt.want {
				t.Errorf("CheckPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHashPassword_DifferentHashes(t *testing.T) {
	password := "samepassword"
	hash1, _ := auth.HashPassword(password)
	hash2, _ := auth.HashPassword(password)

	if hash1 == hash2 {
		t.Error("HashPassword() produced identical hashes for same input (bcrypt should salt)")
	}
}

// --- Context Helpers Tests ---

func TestSetGetUserFromContext(t *testing.T) {
	userID := uuid.New()
	ctx := auth.SetUserInContext(t.Context(), userID)

	got, ok := auth.GetUserFromContext(ctx)
	if !ok {
		t.Fatal("GetUserFromContext() returned ok=false")
	}
	if got != userID {
		t.Errorf("GetUserFromContext() = %v, want %v", got, userID)
	}
}

func TestGetUserFromContext_Missing(t *testing.T) {
	tests := []struct {
		name string
		ctx  func() context.Context
	}{
		{"empty context", func() context.Context { return t.Context() }},
		{"nil context", func() context.Context { return nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := auth.GetUserFromContext(tt.ctx())
			if ok {
				t.Error("GetUserFromContext() returned ok=true, want false")
			}
		})
	}
}

// --- ExtractTokenFromCookie Tests ---

func TestExtractTokenFromCookie(t *testing.T) {
	tests := []struct {
		name      string
		cookie    *http.Cookie
		wantToken string
		wantErr   bool
	}{
		{
			name:      "valid cookie",
			cookie:    &http.Cookie{Name: "access_token", Value: "my-token"},
			wantToken: "my-token",
			wantErr:   false,
		},
		{
			name:    "missing cookie",
			cookie:  nil,
			wantErr: true,
		},
		{
			name:    "empty cookie value",
			cookie:  &http.Cookie{Name: "access_token", Value: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}

			token, err := auth.ExtractTokenFromCookie(req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if token != tt.wantToken {
				t.Errorf("token = %q, want %q", token, tt.wantToken)
			}
		})
	}
}

// --- Middleware Tests ---

func TestMiddleware_ValidToken(t *testing.T) {
	maker := newTestMaker(t)
	userID := uuid.New()

	token, _ := maker.CreateToken(userID, time.Hour)

	var gotUserID uuid.UUID
	var gotOK bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, gotOK = auth.GetUserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := auth.Middleware(maker)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !gotOK {
		t.Fatal("middleware did not set userID in context")
	}
	if gotUserID != userID {
		t.Errorf("context userID = %v, want %v", gotUserID, userID)
	}
}

func TestMiddleware_Unauthorized(t *testing.T) {
	maker := newTestMaker(t)

	// Create expired token
	expiredToken, _ := maker.CreateToken(uuid.New(), time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	tests := []struct {
		name   string
		cookie *http.Cookie
	}{
		{"no cookie", nil},
		{"invalid token", &http.Cookie{Name: "access_token", Value: "invalid-token"}},
		{"expired token", &http.Cookie{Name: "access_token", Value: expiredToken}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			innerCalled := false
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				innerCalled = true
			})

			handler := auth.Middleware(maker)(inner)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			if innerCalled {
				t.Error("inner handler should not be called for unauthorized request")
			}
		})
	}
}
