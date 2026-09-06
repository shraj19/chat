package user

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"chat-v2/internal/domain/ent"
	"chat-v2/internal/domain/ent/user"
	"chat-v2/internal/metrics"
)

var ErrUserExists = errors.New("user already exists")

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

func (r *Repository) Create(ctx context.Context, username, passwordHash, email string) (*ent.User, error) {
	start := time.Now()
	u, err := r.client.User.Create().
		SetUsername(username).
		SetPasswordHash(passwordHash).
		SetEmail(email).
		Save(ctx)
	metrics.ObserveDBQuery("create_user", start, err)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return u, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*ent.User, error) {
	start := time.Now()
	u, err := r.client.User.Get(ctx, id)
	metrics.ObserveDBQuery("get_user", start, err)
	return u, err
}

func (r *Repository) GetByUsername(ctx context.Context, username string) (*ent.User, error) {
	start := time.Now()
	u, err := r.client.User.Query().
		Where(user.UsernameEQ(username)).
		Only(ctx)
	metrics.ObserveDBQuery("get_user_by_username", start, err)
	return u, err
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*ent.User, error) {
	start := time.Now()
	u, err := r.client.User.Query().
		Where(user.EmailEQ(email)).
		Only(ctx)
	metrics.ObserveDBQuery("get_user_by_email", start, err)
	return u, err
}

func (r *Repository) Search(ctx context.Context, query string, limit int) ([]*ent.User, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	start := time.Now()
	users, err := r.client.User.Query().
		Where(user.UsernameHasPrefix(query)).
		Order(ent.Asc(user.FieldUsername)).
		Limit(limit).
		All(ctx)
	metrics.ObserveDBQuery("search_users", start, err)
	return users, err
}
