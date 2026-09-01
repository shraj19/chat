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

/**
 *	RateLimiter is a middleware that provides rate limiting for HTTP requests.
 *	It supports both Redis-based distributed rate limiting and in-memory token bucket rate limiting.
 *	The middleware identifies clients by authenticated user ID (if available) or by IP address.
 */
type RateLimiter struct {
	redisLimiter *redis_rate.Limiter
	memLimiters  sync.Map
	limit        int
	window       time.Duration
	keyPrefix    string
	ipStrategy   realclientip.Strategy
}

/**
 *	RateLimitResult represents the result of a rate limit check.
 *	It indicates whether the request is allowed, the limit, remaining requests, and retry-after duration.
 */
type RateLimitResult struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

/**
 *	NewRateLimiter creates a new RateLimiter instance.
 *	Parameters:
 *	- redis: Redis client for distributed rate limiting. If nil, only in-memory limiting is used.
 *	- keyPrefix: Prefix for Redis keys to avoid collisions.
 *	- limit: Maximum number of requests allowed in the specified window.
 *	- window: Time duration for the rate limit window.
 *	- trustedProxies: Number of trusted proxies to consider when extracting client IP from headers.
 */
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

			result := l.Allow(r.Context(), key)

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

/**
 *	Allow checks if the request is allowed under the rate limit.
 *	It first tries Redis for distributed rate limiting. If Redis is unavailable or fails, it falls back to an in-memory token bucket.
 */
func (l *RateLimiter) Allow(ctx context.Context, key string) RateLimitResult {
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
	return l.allowMemory(key)
}

func (l *RateLimiter) allowMemory(key string) RateLimitResult {
	// Create rate based on limit/window
	r := rate.Limit(float64(l.limit) / l.window.Seconds())

	limiterObj, _ := l.memLimiters.LoadOrStore(key, rate.NewLimiter(r, l.limit))
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
