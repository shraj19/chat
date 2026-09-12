package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// ctxKey is a private type for context keys to avoid collisions.
type ctxKey int

const (
	requestIDKey ctxKey = iota
	userIDKey
)

// RequestID middleware generates a unique request ID for each request,
// stores it in the context, and sets it as a response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client sent one (for distributed tracing)
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}

		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set("X-Request-ID", id)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID retrieves the request ID from context.
// Returns empty string if not present.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// WithUserID stores the user ID in context (called from auth middleware).
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// GetUserID retrieves the user ID from context.
func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(userIDKey).(string); ok {
		return id
	}
	return ""
}
