package user_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"chat-v2/internal/domain/ent"
	"chat-v2/internal/testutil"
	"chat-v2/internal/user"
)

var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	pg, err := testutil.StartPostgres(ctx)
	if err != nil {
		panic("failed to start postgres: " + err.Error())
	}

	testDSN = pg.DSN

	code := m.Run()

	pg.Container.Terminate(ctx)
	os.Exit(code)
}

func setupRepo(t *testing.T) (*user.Repository, *ent.Client) {
	t.Helper()
	client := testutil.NewEntClient(t, testDSN)
	repo := user.NewRepository(client)

	t.Cleanup(func() {
		testutil.CleanupEntData(context.Background(), client)
	})

	return repo, client
}

func TestCreate_Success(t *testing.T) {
	repo, _ := setupRepo(t)
	ctx := context.Background()

	u, err := repo.Create(ctx, "alice", "hashedpass123", "alice@example.com")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if u.Username != "alice" {
		t.Errorf("Username = %q, want %q", u.Username, "alice")
	}
	if u.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", u.Email, "alice@example.com")
	}
	if u.ID == uuid.Nil {
		t.Error("ID should not be nil")
	}
	if u.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestCreate_Duplicate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		first   [3]string // username, hash, email
		second  [3]string
		wantErr error
	}{
		{
			name:    "duplicate username",
			first:   [3]string{"bob", "hash1", "bob@example.com"},
			second:  [3]string{"bob", "hash2", "bob2@example.com"},
			wantErr: user.ErrUserExists,
		},
		{
			name:    "duplicate email",
			first:   [3]string{"user1", "hash1", "same@example.com"},
			second:  [3]string{"user2", "hash2", "same@example.com"},
			wantErr: user.ErrUserExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean slate for each subtest
			localRepo, _ := setupRepo(t)

			_, err := localRepo.Create(ctx, tt.first[0], tt.first[1], tt.first[2])
			if err != nil {
				t.Fatalf("first Create() error: %v", err)
			}

			_, err = localRepo.Create(ctx, tt.second[0], tt.second[1], tt.second[2])
			if err != tt.wantErr {
				t.Errorf("second Create() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetByID(t *testing.T) {
	repo, _ := setupRepo(t)
	ctx := context.Background()

	created, _ := repo.Create(ctx, "charlie", "hash", "charlie@example.com")

	tests := []struct {
		name       string
		id         uuid.UUID
		wantErr    bool
		isNotFound bool
	}{
		{"exists", created.ID, false, false},
		{"not found", uuid.New(), true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.GetByID(ctx, tt.id)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.isNotFound && !ent.IsNotFound(err) {
					t.Errorf("expected NotFound error, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetByID() error: %v", err)
			}
			if got.Username != "charlie" {
				t.Errorf("Username = %q, want %q", got.Username, "charlie")
			}
		})
	}
}

func TestGetByUsername(t *testing.T) {
	repo, _ := setupRepo(t)
	ctx := context.Background()

	_, _ = repo.Create(ctx, "diana", "hash", "diana@example.com")

	tests := []struct {
		name       string
		username   string
		wantEmail  string
		wantErr    bool
		isNotFound bool
	}{
		{"exists", "diana", "diana@example.com", false, false},
		{"not found", "nonexistent", "", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.GetByUsername(ctx, tt.username)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.isNotFound && !ent.IsNotFound(err) {
					t.Errorf("expected NotFound error, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetByUsername() error: %v", err)
			}
			if got.Email != tt.wantEmail {
				t.Errorf("Email = %q, want %q", got.Email, tt.wantEmail)
			}
		})
	}
}

func TestGetByEmail(t *testing.T) {
	repo, _ := setupRepo(t)
	ctx := context.Background()

	_, _ = repo.Create(ctx, "eve", "hash", "eve@example.com")

	tests := []struct {
		name         string
		email        string
		wantUsername string
		wantErr      bool
		isNotFound   bool
	}{
		{"exists", "eve@example.com", "eve", false, false},
		{"not found", "nobody@example.com", "", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.GetByEmail(ctx, tt.email)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.isNotFound && !ent.IsNotFound(err) {
					t.Errorf("expected NotFound error, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetByEmail() error: %v", err)
			}
			if got.Username != tt.wantUsername {
				t.Errorf("Username = %q, want %q", got.Username, tt.wantUsername)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	repo, _ := setupRepo(t)
	ctx := context.Background()

	// Create test users
	_, _ = repo.Create(ctx, "frank", "hash", "frank@example.com")
	_, _ = repo.Create(ctx, "fred", "hash", "fred@example.com")
	_, _ = repo.Create(ctx, "grace", "hash", "grace@example.com")

	tests := []struct {
		name      string
		prefix    string
		limit     int
		wantCount int
		wantFirst string // expected first result username (if any)
	}{
		{"prefix match", "fr", 10, 2, "frank"},
		{"no match", "xyz", 10, 0, ""},
		{"respects limit", "fr", 1, 1, "frank"},
		{"single match", "gra", 10, 1, "grace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := repo.Search(ctx, tt.prefix, tt.limit)
			if err != nil {
				t.Fatalf("Search() error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("result count = %d, want %d", len(results), tt.wantCount)
			}
			if tt.wantFirst != "" && len(results) > 0 && results[0].Username != tt.wantFirst {
				t.Errorf("first result = %q, want %q", results[0].Username, tt.wantFirst)
			}
		})
	}
}

func TestSearch_InvalidLimit_DefaultsTo10(t *testing.T) {
	repo, _ := setupRepo(t)
	ctx := context.Background()

	_, _ = repo.Create(ctx, "user_x", "hash", "x@example.com")

	// Negative limit should default to 10
	results, err := repo.Search(ctx, "user_", -1)
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) > 10 {
		t.Errorf("Search() returned %d results, should cap at 10 for invalid limit", len(results))
	}
}
