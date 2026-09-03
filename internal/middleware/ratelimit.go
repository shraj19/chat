package middleware

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/google/uuid"
	"github.com/realclientip/realclientip-go"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"

	"chat-v2/internal/auth"
	"chat-v2/internal/pkg/logger"
)

// RateLimiter rate-limits HTTP requests, using Redis for distributed limiting
// and falling back to a per-instance in-memory token bucket when Redis is
// unavailable. Clients are identified by authenticated user ID, else by IP.
type RateLimiter struct {
	redisLimiter   *redis_rate.Limiter
	memoryLimiters sync.Map
	limit          int
	window         time.Duration
	keyPrefix      string
	ipStrategy     realclientip.Strategy
}

// RateLimitResult is the outcome of a rate-limit check: whether the request is
// allowed, plus the limit, remaining budget, and retry-after hint.
type RateLimitResult struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

// NewRateLimiter builds a RateLimiter. If redis is nil, only in-memory limiting
// is used. trustedProxies controls client-IP extraction: 0 uses RemoteAddr,
// 1+ trusts that many rightmost proxies in X-Forwarded-For.
func NewRateLimiter(redis *goredis.Client, keyPrefix string, limit int, window time.Duration, trustedProxies int) *RateLimiter {
	var redisLimiter *redis_rate.Limiter
	if redis != nil {
		redisLimiter = redis_rate.NewLimiter(redis)
	}

	// Configure IP extraction strategy based on trusted proxy count
	var ipStrategy realclientip.Strategy
	if trustedProxies > 0 {
		// Trust the rightmost N proxies, take the IP just before them
		ipStrategy, _ = realclientip.NewRightmostTrustedCountStrategy("X-Forwarded-For", trustedProxies)
	} else {
		// Don't trust any proxy headers, use RemoteAddr
		ipStrategy = realclientip.RemoteAddrStrategy{}
	}

	return &RateLimiter{
		redisLimiter: redisLimiter,
		limit:        limit,
		window:       window,
		keyPrefix:    keyPrefix,
		ipStrategy:   ipStrategy,
	}
}

// RateLimit returns middleware that rate limits requests.
// Deprecated: Use NewRateLimiter and RateLimitMiddleware for proper IP handling.
func RateLimit(redis *goredis.Client, keyPrefix string, limit int, window time.Duration) func(http.Handler) http.Handler {
	limiter := NewRateLimiter(redis, keyPrefix, limit, window, 0) // default: don't trust XFF
	return limiter.middleware()
}

// RateLimitWithConfig returns middleware with proper trusted proxy configuration.
func RateLimitWithConfig(redis *goredis.Client, keyPrefix string, limit int, window time.Duration, trustedProxies int) func(http.Handler) http.Handler {
	limiter := NewRateLimiter(redis, keyPrefix, limit, window, trustedProxies)
	return limiter.middleware()
}

func (l *RateLimiter) middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identifier := l.getClientIdentifier(r)
			key := l.keyPrefix + ":" + identifier

			result := l.Consume(r.Context(), key)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))

			if !result.Allowed {
				retryAfter := int(result.RetryAfter.Seconds())
				retryAfter = max(retryAfter, 1) // Ensure at least 1 second

				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				logger.Warn("Rate limit exceeded", "key", key, "identifier", identifier)
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Consume records one request against the limit for key and returns the result.
// It tries Redis first (distributed); on Redis error it falls back to a
// per-instance in-memory token bucket (approximate, no cross-node coordination).
func (l *RateLimiter) Consume(ctx context.Context, key string) RateLimitResult {
	// Try Redis first
	if l.redisLimiter != nil {
		result, err := l.redisLimiter.Allow(ctx, key, redis_rate.Limit{
			Rate:   l.limit,
			Burst:  l.limit,
			Period: l.window,
		})
		if err == nil {
			return RateLimitResult{
				Allowed:    result.Allowed > 0,
				Limit:      l.limit,
				Remaining:  max(0, result.Remaining),
				RetryAfter: result.RetryAfter,
			}
		}
		logger.Warn("Redis rate limit failed, falling back to memory", "error", err)
	}

	// Memory fallback using token bucket
	return l.consumeMemory(key)
}

func (l *RateLimiter) consumeMemory(key string) RateLimitResult {
	// Create rate based on limit/window
	r := rate.Limit(float64(l.limit) / l.window.Seconds())

	limiterObj, _ := l.memoryLimiters.LoadOrStore(key, rate.NewLimiter(r, l.limit))
	memLimiter := limiterObj.(*rate.Limiter)

	if memLimiter.Allow() {
		tokens := int(memLimiter.Tokens())
		return RateLimitResult{
			Allowed:   true,
			Limit:     l.limit,
			Remaining: tokens,
		}
	}

	return RateLimitResult{
		Allowed:    false,
		Limit:      l.limit,
		Remaining:  0,
		RetryAfter: l.window,
	}
}

func (l *RateLimiter) getClientIdentifier(r *http.Request) string {
	// Prefer authenticated user ID (can't be spoofed)
	if userID, ok := auth.GetUserFromContext(r.Context()); ok && userID != uuid.Nil {
		return "user:" + userID.String()
	}

	// Fall back to IP using configured strategy
	ip := l.ipStrategy.ClientIP(r.Header, r.RemoteAddr)
	if ip == "" {
		// Fallback if strategy returns empty (shouldn't happen with RemoteAddrStrategy)
		ip = r.RemoteAddr
	}
	return "ip:" + ip
}
