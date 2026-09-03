// Package conversation provides functionality for managing conversations.
package conversation

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"chat-v2/internal/metrics"
	"chat-v2/internal/pkg/logger"
)

const participantCacheTTL = 1 * time.Hour

// ParticipantCache is a cache for conversation participants using Redis.
type ParticipantCache struct {
	redis         *goredis.Client
	repo          *Repository
	populateGroup singleflight.Group // prevents concurrent populate storms
}

// NewParticipantCache returns a new ParticipantCache.
func NewParticipantCache(redis *goredis.Client, repo *Repository) *ParticipantCache {
	return &ParticipantCache{redis: redis, repo: repo}
}

func participantKey(convID uuid.UUID) string {
	return fmt.Sprintf("cache:conv:%s:members", convID.String())
}

// IsParticipant checks if a user is a participant in a conversation. It first checks the Redis cache, and if not found, falls back to the database. If the cache is missing, it populates it asynchronously.
func (c *ParticipantCache) IsParticipant(ctx context.Context, conversationID, userID uuid.UUID) (bool, error) {
	start := time.Now()

	if c.redis == nil {
		return c.repo.IsParticipant(ctx, conversationID, userID)
	}

	key := participantKey(conversationID)

	// Try cache first
	exists, err := c.redis.Exists(ctx, key).Result()

	if err != nil {
		// Redis error — log and fallback to DB
		logger.Warn("Redis error while checking participant cache", "error", err)
		metrics.CacheErrorsTotal.WithLabelValues("participant", "check").Inc()

		// fallback to DB query
		return c.repo.IsParticipant(ctx, conversationID, userID)
	}

	if exists > 0 {
		isMember, err := c.redis.SIsMember(ctx, key, userID.String()).Result()
		if err != nil {
			// Redis error — log and fallback to DB
			logger.Warn("Redis error while checking participant membership", "error", err)
			metrics.CacheErrorsTotal.WithLabelValues("participant", "check").Inc()

			// fallback to DB query
			return c.repo.IsParticipant(ctx, conversationID, userID)
		}

		// clean hit
		metrics.CacheHitsTotal.WithLabelValues("participant").Inc()
		metrics.CacheOperationsDuration.WithLabelValues("participant", "check").Observe(time.Since(start).Seconds())

		return isMember, nil
	}

	// exists == 0, cache miss
	metrics.CacheMissesTotal.WithLabelValues("participant").Inc()
	metrics.CacheOperationsDuration.WithLabelValues("participant", "check").Observe(time.Since(start).Seconds())

	// fallback to DB query
	isMember, err := c.repo.IsParticipant(ctx, conversationID, userID)
	if err != nil {
		return false, err
	}

	// cache the result for future requests since it is a miss not redis error
	// Populate cache asynchronously to avoid blocking the request
	go c.populateIfMissing(conversationID)

	return isMember, nil
}

func (c *ParticipantCache) populateIfMissing(conversationID uuid.UUID) {
	if c.redis == nil {
		return
	}

	key := conversationID.String()

	// singleflight ensures only one goroutine populates, others wait or return
	c.populateGroup.Do(key, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		redisKey := participantKey(conversationID)

		// Check if already populated
		exists, _ := c.redis.Exists(ctx, redisKey).Result()
		if exists > 0 {
			return nil, nil
		}

		members, err := c.repo.GetParticipants(ctx, conversationID)
		if err != nil || len(members) == 0 {
			return nil, err
		}

		c.populate(ctx, conversationID, members)
		return nil, nil
	})
}

func (c *ParticipantCache) populate(ctx context.Context, conversationID uuid.UUID, members []uuid.UUID) {
	if c.redis == nil || len(members) == 0 {
		return
	}

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.CacheOperationsDuration.WithLabelValues("participant", "populate").Observe(duration.Seconds())
	}()

	key := participantKey(conversationID)

	// Use a pipeline to set the members and expiration atomically
	pipe := c.redis.Pipeline()
	pipe.Del(ctx, key)

	memberStrs := make([]any, len(members))
	for i, m := range members {
		memberStrs[i] = m.String()
	}

	pipe.SAdd(ctx, key, memberStrs...)
	pipe.Expire(ctx, key, participantCacheTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		logger.Warn("Failed to populate participant cache", "error", err)
	}
}

// Add adds a user to the participant cache for a conversation.
func (c *ParticipantCache) Add(ctx context.Context, conversationID, userID uuid.UUID) {
	if c.redis == nil {
		return
	}

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.CacheOperationsDuration.WithLabelValues("participant", "add").Observe(duration.Seconds())
	}()

	key := participantKey(conversationID)
	exists, _ := c.redis.Exists(ctx, key).Result()
	if exists > 0 {
		c.redis.SAdd(ctx, key, userID.String())
	}
}

// Remove removes a user from the participant cache for a conversation.
func (c *ParticipantCache) Remove(ctx context.Context, conversationID, userID uuid.UUID) {
	if c.redis == nil {
		return
	}

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.CacheOperationsDuration.WithLabelValues("participant", "remove").Observe(duration.Seconds())
	}()

	key := participantKey(conversationID)
	c.redis.SRem(ctx, key, userID.String())
}

// Invalidate invalidates the participant cache for a conversation, forcing a refresh on the next access.
func (c *ParticipantCache) Invalidate(ctx context.Context, conversationID uuid.UUID) {

	if c.redis == nil {
		return
	}

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		metrics.CacheOperationsDuration.WithLabelValues("participant", "invalidate").Observe(duration.Seconds())
	}()

	c.redis.Del(ctx, participantKey(conversationID))
}
