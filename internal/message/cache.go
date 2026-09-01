package message

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	gocache "github.com/patrickmn/go-cache"
	goredis "github.com/redis/go-redis/v9"

	"chat-v2/internal/metrics"
	"chat-v2/internal/pkg/logger"
)

const maxCachedMessages = 100

// MsgCache caches recent messages per conversation. Implementations must be
// safe for concurrent use. GetRecent returns (nil, nil) on a miss so callers
// can fall back to the database.
type MsgCache interface {

	// GetRecent returns cached messages oldest-to-newest, capped at
	// maxCachedMessages. It returns (nil, nil) when nothing is cached.
	GetRecent(ctx context.Context, conversationID uuid.UUID) ([]*Message, error)

	// SetRecent replaces the cached message set for the conversation.
	SetRecent(ctx context.Context, conversationID uuid.UUID, messages []*Message) error

	// AddMessage appends a single message, evicting oldest entries past the cap.
	AddMessage(ctx context.Context, conversationID uuid.UUID, msg *Message) error

	// Invalidate drops all cached messages for the conversation.
	Invalidate(ctx context.Context, conversationID uuid.UUID) error
}

func cacheKey(convID uuid.UUID) string {
	return fmt.Sprintf("cache:conv:%s:messages", convID.String())
}

// RedisCache stores recent messages in a Redis sorted set, scored by
// creation time and capped at maxCachedMessages.
type RedisCache struct {
	client *goredis.Client
	ttl    time.Duration
}

// NewRedisCache returns a RedisCache. ttl defaults to 24h when <= 0.
func NewRedisCache(client *goredis.Client, ttl time.Duration) *RedisCache {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &RedisCache{client: client, ttl: ttl}
}

// GetRecent implements MsgCache using a Redis sorted set (ZRANGE). A Redis
// error is returned to the caller rather than treated as a miss.
func (c *RedisCache) GetRecent(ctx context.Context, conversationID uuid.UUID) ([]*Message, error) {
	if c.client == nil {
		return nil, nil
	}

	start := time.Now()
	key := cacheKey(conversationID)
	vals, err := c.client.ZRange(ctx, key, 0, -1).Result()
	if err != nil {
		// Redis error: not a hit, not a clean miss — surface the error.
		return nil, err
	}

	metrics.CacheOperationsDuration.WithLabelValues("message", "get_recent").Observe(time.Since(start).Seconds())

	if len(vals) == 0 {
		// Clean miss: cache is up but has nothing for this conversation.
		metrics.CacheMissesTotal.WithLabelValues("message").Inc()
		return nil, nil
	}

	metrics.CacheHitsTotal.WithLabelValues("message").Inc()

	messages := make([]*Message, 0, len(vals))
	for _, val := range vals {
		var msg Message
		if err := json.Unmarshal([]byte(val), &msg); err != nil {
			continue
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}

func (c *RedisCache) SetRecent(ctx context.Context, conversationID uuid.UUID, messages []*Message) error {
	if c.client == nil || len(messages) == 0 {
		return nil
	}

	key := cacheKey(conversationID)
	pipe := c.client.Pipeline()
	pipe.Del(ctx, key)

	for _, msg := range messages {
		payload, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		score := float64(msg.CreatedAt.UnixNano() / int64(time.Millisecond))
		pipe.ZAdd(ctx, key, goredis.Z{Score: score, Member: string(payload)})
	}

	pipe.ZRemRangeByRank(ctx, key, 0, -(maxCachedMessages + 1))
	pipe.Expire(ctx, key, c.ttl)

	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisCache) AddMessage(ctx context.Context, conversationID uuid.UUID, msg *Message) error {
	if c.client == nil || msg == nil {
		return nil
	}

	key := cacheKey(conversationID)
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	score := float64(msg.CreatedAt.UnixNano() / int64(time.Millisecond))
	pipe := c.client.Pipeline()
	pipe.ZAdd(ctx, key, goredis.Z{Score: score, Member: string(payload)})
	pipe.ZRemRangeByRank(ctx, key, 0, -(maxCachedMessages + 1))
	pipe.Expire(ctx, key, c.ttl)

	_, err = pipe.Exec(ctx)
	return err
}

func (c *RedisCache) Invalidate(ctx context.Context, conversationID uuid.UUID) error {
	if c.client == nil {
		return nil
	}
	return c.client.Del(ctx, cacheKey(conversationID)).Err()
}

// MemoryCache is an in-memory MsgCache used as a fallback when Redis is
// unavailable. It relies on go-cache for automatic expiration.
type MemoryCache struct {
	cache *gocache.Cache
	ttl   time.Duration
}

// NewMemoryCache returns a MemoryCache. ttl defaults to 10m when <= 0; the
// cleanup interval is 1.5x ttl.
func NewMemoryCache(ttl time.Duration) *MemoryCache {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	// Create cache with default expiration and cleanup interval (1.5x TTL)
	return &MemoryCache{
		cache: gocache.New(ttl, ttl*3/2),
		ttl:   ttl,
	}
}

// GetRecent implements MsgCache using an in-memory go-cache store.
func (c *MemoryCache) GetRecent(ctx context.Context, conversationID uuid.UUID) ([]*Message, error) {
	start := time.Now()
	key := conversationID.String()
	val, found := c.cache.Get(key)
	metrics.CacheOperationsDuration.WithLabelValues("message", "get_recent").Observe(time.Since(start).Seconds())

	if found {
		metrics.CacheHitsTotal.WithLabelValues("message").Inc()
		return val.([]*Message), nil
	}

	metrics.CacheMissesTotal.WithLabelValues("message").Inc()
	return nil, nil
}

func (c *MemoryCache) SetRecent(ctx context.Context, conversationID uuid.UUID, messages []*Message) error {
	key := conversationID.String()
	// Cap at maxCachedMessages
	if len(messages) > maxCachedMessages {
		messages = messages[len(messages)-maxCachedMessages:]
	}
	c.cache.Set(key, messages, gocache.DefaultExpiration)
	return nil
}

func (c *MemoryCache) AddMessage(ctx context.Context, conversationID uuid.UUID, msg *Message) error {
	key := conversationID.String()

	var messages []*Message
	if val, found := c.cache.Get(key); found {
		messages = val.([]*Message)
	}

	messages = append(messages, msg)
	if len(messages) > maxCachedMessages {
		messages = messages[len(messages)-maxCachedMessages:]
	}

	c.cache.Set(key, messages, gocache.DefaultExpiration)
	return nil
}

func (c *MemoryCache) Invalidate(ctx context.Context, conversationID uuid.UUID) error {
	c.cache.Delete(conversationID.String())
	return nil
}

// CachedService decorates Service, writing newly created messages through to
// the cache so subsequent reads stay warm.
type CachedService struct {
	*Service
	cache MsgCache
}

// NewCachedService wraps svc with write-through caching via cache.
func NewCachedService(svc *Service, cache MsgCache) *CachedService {
	return &CachedService{Service: svc, cache: cache}
}

// Create delegates to the underlying Service, then best-effort writes the new
// message to the cache. A cache write failure is logged, not returned.
func (s *CachedService) Create(ctx context.Context, userID, conversationID uuid.UUID, content, username, clientID string) (*OutMessage, error) {
	outMsg, err := s.Service.Create(ctx, userID, conversationID, content, username, clientID)
	if err != nil {
		return nil, err
	}

	if s.cache != nil && outMsg != nil {
		cacheMsg := &Message{
			ID:             outMsg.ID,
			SenderID:       outMsg.SenderID,
			SenderUsername: outMsg.SenderUsername,
			ConversationID: outMsg.ConversationID,
			Content:        outMsg.Content,
			CreatedAt:      outMsg.CreatedAt,
		}
		if err := s.cache.AddMessage(ctx, conversationID, cacheMsg); err != nil {
			logger.Warn("Failed to cache message", "error", err)
		}
	}

	return outMsg, nil
}
