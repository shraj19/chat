package message

import (
	"chat-v2/internal/conversation"
	"chat-v2/internal/pkg/logger"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrNotParticipant   = errors.New("not a participant")
	ErrInvalidMessage   = errors.New("invalid message")
	ErrDuplicateMessage = errors.New("duplicate message")
)

const dedupTTL = 5 * time.Minute

type Service struct {
	repo             *Repository
	participantCache *conversation.ParticipantCache
	convRepo         *conversation.Repository
	publisher        EventPublisher
	redis            *goredis.Client
}

type EventPublisher interface {
	Publish(ctx context.Context, msg *OutMessage) error
}

type OutMessage struct {
	Type           string    `json:"type"`
	ID             uuid.UUID `json:"id"`
	SenderID       uuid.UUID `json:"sender_id"`
	SenderUsername string    `json:"sender_username"`
	ConversationID uuid.UUID `json:"conversation_id"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

func NewService(repo *Repository, convRepo *conversation.Repository, participantCache *conversation.ParticipantCache, publisher EventPublisher, redis *goredis.Client) *Service {
	return &Service{
		repo:             repo,
		convRepo:         convRepo,
		participantCache: participantCache,
		publisher:        publisher,
		redis:            redis,
	}
}

func (s *Service) Create(ctx context.Context, userID, conversationID uuid.UUID, content, username, clientID string) (*OutMessage, error) {
	if userID == uuid.Nil || conversationID == uuid.Nil || strings.TrimSpace(content) == "" {
		return nil, ErrInvalidMessage
	}

	// Idempotency check
	if clientID != "" && s.redis != nil {
		dedupKey := fmt.Sprintf("dedup:msg:%s", clientID)
		set, err := s.redis.SetNX(ctx, dedupKey, "1", dedupTTL).Result()
		if err != nil {
			logger.Warn("Redis dedup check failed", "error", err)
		} else if !set {
			return nil, ErrDuplicateMessage
		}
	}

	// Participant check
	var isParticipant bool
	var err error
	if s.participantCache != nil {
		isParticipant, err = s.participantCache.IsParticipant(ctx, conversationID, userID)
	} else {
		isParticipant, err = s.convRepo.IsParticipant(ctx, conversationID, userID)
	}
	if err != nil {
		return nil, err
	}
	if !isParticipant {
		return nil, ErrNotParticipant
	}

	msg, err := s.repo.Create(ctx, conversationID, userID, content)
	if err != nil {
		return nil, err
	}

	outMsg := &OutMessage{
		Type:           "message",
		ID:             msg.ID,
		SenderID:       msg.SenderID,
		SenderUsername: username,
		ConversationID: msg.ConversationID,
		Content:        msg.Content,
		CreatedAt:      msg.CreatedAt,
	}

	if s.publisher != nil {
		if err := s.publisher.Publish(ctx, outMsg); err != nil {
			logger.Error("Failed to publish message", "error", err)
		}
	}

	return outMsg, nil
}
