package user

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"chat-v2/internal/domain/ent"
	"chat-v2/internal/domain/ent/user"
)

var ErrUserExists = errors.New("user already exists")

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

func (r *Repository) Create(ctx context.Context, username, passwordHash, email string) (*ent.User, error) {
	u, err := r.client.User.Create().
		SetUsername(username).
		SetPasswordHash(passwordHash).
		SetEmail(email).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return u, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*ent.User, error) {
	return r.client.User.Get(ctx, id)
}

func (r *Repository) GetByUsername(ctx context.Context, username string) (*ent.User, error) {
	return r.client.User.Query().
		Where(user.UsernameEQ(username)).
		Only(ctx)
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*ent.User, error) {
	return r.client.User.Query().
		Where(user.EmailEQ(email)).
		Only(ctx)
}

func (r *Repository) Search(ctx context.Context, query string, limit int) ([]*ent.User, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	return r.client.User.Query().
		Where(user.UsernameHasPrefix(query)).
		Order(ent.Asc(user.FieldUsername)).
		Limit(limit).
		All(ctx)
}
