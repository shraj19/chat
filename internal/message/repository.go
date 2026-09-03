package message

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"chat-v2/internal/domain/ent"
	"chat-v2/internal/domain/ent/message"
)

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

type Message struct {
	ID             uuid.UUID `json:"id"`
	SenderID       uuid.UUID `json:"sender_id"`
	SenderUsername string    `json:"sender_username"`
	ConversationID uuid.UUID `json:"conversation_id"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

type ListResponse struct {
	Messages   []*Message `json:"messages"`
	NextCursor string     `json:"next_cursor,omitempty"`
	HasMore    bool       `json:"has_more"`
}

type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

func EncodeCursor(c Cursor) string {
	b, _ := json.Marshal(c)
	return base64.StdEncoding.EncodeToString(b)
}

func DecodeCursor(s string) (*Cursor, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var c Cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) Create(ctx context.Context, conversationID, senderID uuid.UUID, content string) (*ent.Message, error) {
	return r.client.Message.Create().
		SetConversationID(conversationID).
		SetSenderID(senderID).
		SetContent(content).
		Save(ctx)
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*ent.Message, error) {
	return r.client.Message.Get(ctx, id)
}

func (r *Repository) List(ctx context.Context, conversationID uuid.UUID, before *Cursor, limit int) (*ListResponse, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}

	query := r.client.Message.Query().
		Where(message.ConversationIDEQ(conversationID)).
		WithSender().
		Order(ent.Desc(message.FieldCreatedAt), ent.Desc(message.FieldID)).
		Limit(limit + 1)

	if before != nil {
		query = query.Where(
			message.Or(
				message.CreatedAtLT(before.CreatedAt),
				message.And(
					message.CreatedAtEQ(before.CreatedAt),
					message.IDLT(before.ID),
				),
			),
		)
	}

	msgs, err := query.All(ctx)
	if err != nil {
		return nil, err
	}

	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}

	// Reverse for oldest-first
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	result := make([]*Message, len(msgs))
	for i, m := range msgs {
		username := ""
		if m.Edges.Sender != nil {
			username = m.Edges.Sender.Username
		}
		result[i] = &Message{
			ID:             m.ID,
			SenderID:       m.SenderID,
			SenderUsername: username,
			ConversationID: m.ConversationID,
			Content:        m.Content,
			CreatedAt:      m.CreatedAt,
		}
	}

	var nextCursor string
	if hasMore && len(msgs) > 0 {
		lastMsg := msgs[0]
		nextCursor = EncodeCursor(Cursor{CreatedAt: lastMsg.CreatedAt, ID: lastMsg.ID})
	}

	return &ListResponse{
		Messages:   result,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
