package conversation

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"chat-v2/internal/domain/ent"
	"chat-v2/internal/domain/ent/conversation"
	"chat-v2/internal/domain/ent/conversationparticipant"
	"chat-v2/internal/domain/ent/user"
	"chat-v2/internal/metrics"
)

var ErrConversationExists = errors.New("conversation already exists")
var ErrUsersNotFound = errors.New("one or more usernames not found")

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

func (r *Repository) Create(ctx context.Context, convType, title, displayName, canonicalName string) (*ent.Conversation, error) {
	builder := r.client.Conversation.Create().
		SetType(conversation.Type(convType))

	if title != "" {
		builder.SetTitle(title)
	}
	if displayName != "" {
		builder.SetDisplayName(displayName)
	}
	if canonicalName != "" {
		builder.SetCanonicalName(canonicalName)
	}

	start := time.Now()
	conv, err := builder.Save(ctx)
	metrics.ObserveDBQuery("create_conversation", start, err)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, ErrConversationExists
		}
		return nil, err
	}
	return conv, nil
}

func (r *Repository) CreateWithParticipants(ctx context.Context, convType, title, displayName, canonicalName string, usernames []string) (*ent.Conversation, error) {
	start := time.Now()
	conv, err := r.createWithParticipants(ctx, convType, title, displayName, canonicalName, usernames)
	metrics.ObserveDBQuery("create_conversation_tx", start, err)
	return conv, err
}

func (r *Repository) createWithParticipants(ctx context.Context, convType, title, displayName, canonicalName string, usernames []string) (*ent.Conversation, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch users
	users, err := tx.User.Query().
		Where(user.UsernameIn(usernames...)).
		All(ctx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if len(users) != len(usernames) {
		tx.Rollback()
		return nil, ErrUsersNotFound
	}

	// Create conversation
	builder := tx.Conversation.Create().
		SetType(conversation.Type(convType))

	if title != "" {
		builder.SetTitle(title)
	}
	if displayName != "" {
		builder.SetDisplayName(displayName)
	}
	if canonicalName != "" {
		builder.SetCanonicalName(canonicalName)
	}

	conv, err := builder.Save(ctx)
	if err != nil {
		tx.Rollback()
		if ent.IsConstraintError(err) {
			return nil, ErrConversationExists
		}
		return nil, err
	}

	// Add participants
	for _, u := range users {
		_, err := tx.ConversationParticipant.Create().
			SetConversationID(conv.ID).
			SetUserID(u.ID).
			Save(ctx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return conv, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*ent.Conversation, error) {
	start := time.Now()
	c, err := r.client.Conversation.Get(ctx, id)
	metrics.ObserveDBQuery("get_conversation", start, err)
	return c, err
}

func (r *Repository) GetByCanonicalName(ctx context.Context, canonicalName string) (*ent.Conversation, error) {
	start := time.Now()
	c, err := r.client.Conversation.Query().
		Where(
			conversation.CanonicalNameEQ(canonicalName),
			conversation.TypeEQ(conversation.TypePrivate),
		).
		Only(ctx)
	metrics.ObserveDBQuery("get_conversation_by_canonical_name", start, err)
	return c, err
}

func (r *Repository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*ent.Conversation, error) {
	start := time.Now()
	convs, err := r.client.Conversation.Query().
		Where(conversation.HasParticipantsWith(conversationparticipant.UserIDEQ(userID))).
		Order(ent.Asc(conversation.FieldCreatedAt)).
		All(ctx)
	metrics.ObserveDBQuery("list_conversations", start, err)
	return convs, err
}

type ConversationWithDisplay struct {
	*ent.Conversation
	DisplayName *string `json:"display_name,omitempty"`
}

func (r *Repository) GetByUserIDWithDisplay(ctx context.Context, userID uuid.UUID) ([]*ConversationWithDisplay, error) {
	start := time.Now()
	convs, err := r.client.Conversation.Query().
		Where(conversation.HasParticipantsWith(conversationparticipant.UserIDEQ(userID))).
		WithParticipants(func(q *ent.ConversationParticipantQuery) {
			q.WithUser()
		}).
		Order(ent.Asc(conversation.FieldCreatedAt)).
		All(ctx)
	metrics.ObserveDBQuery("list_conversations_with_display", start, err)
	if err != nil {
		return nil, err
	}

	result := make([]*ConversationWithDisplay, 0, len(convs))
	for _, conv := range convs {
		cwd := &ConversationWithDisplay{
			Conversation: conv,
			DisplayName:  conv.DisplayName,
		}

		if conv.Type == conversation.TypePrivate {
			for _, p := range conv.Edges.Participants {
				if p.UserID != userID && p.Edges.User != nil {
					cwd.DisplayName = &p.Edges.User.Username
					break
				}
			}
		}

		result = append(result, cwd)
	}

	return result, nil
}

func (r *Repository) AddParticipant(ctx context.Context, conversationID, userID uuid.UUID) error {
	start := time.Now()
	_, err := r.client.ConversationParticipant.Create().
		SetConversationID(conversationID).
		SetUserID(userID).
		Save(ctx)
	metrics.ObserveDBQuery("add_participant", start, err)
	return err
}

func (r *Repository) RemoveParticipant(ctx context.Context, conversationID, userID uuid.UUID) error {
	start := time.Now()
	_, err := r.client.ConversationParticipant.Delete().
		Where(
			conversationparticipant.ConversationIDEQ(conversationID),
			conversationparticipant.UserIDEQ(userID),
		).
		Exec(ctx)
	metrics.ObserveDBQuery("remove_participant", start, err)
	return err
}

func (r *Repository) GetParticipants(ctx context.Context, conversationID uuid.UUID) ([]uuid.UUID, error) {
	start := time.Now()
	participants, err := r.client.ConversationParticipant.Query().
		Where(conversationparticipant.ConversationIDEQ(conversationID)).
		All(ctx)
	metrics.ObserveDBQuery("get_participants", start, err)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, len(participants))
	for i, p := range participants {
		ids[i] = p.UserID
	}
	return ids, nil
}

func (r *Repository) IsParticipant(ctx context.Context, conversationID, userID uuid.UUID) (bool, error) {
	start := time.Now()
	ok, err := r.client.ConversationParticipant.Query().
		Where(
			conversationparticipant.ConversationIDEQ(conversationID),
			conversationparticipant.UserIDEQ(userID),
		).
		Exist(ctx)
	metrics.ObserveDBQuery("is_participant", start, err)
	return ok, err
}

func (r *Repository) GetFriends(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	start := time.Now()
	convs, err := r.client.Conversation.Query().
		Where(
			conversation.TypeEQ(conversation.TypePrivate),
			conversation.HasParticipantsWith(conversationparticipant.UserIDEQ(userID)),
		).
		WithParticipants().
		All(ctx)
	metrics.ObserveDBQuery("get_friends", start, err)
	if err != nil {
		return nil, err
	}

	var friendIDs []uuid.UUID
	for _, conv := range convs {
		for _, p := range conv.Edges.Participants {
			if p.UserID != userID {
				friendIDs = append(friendIDs, p.UserID)
			}
		}
	}

	return friendIDs, nil
}
