package testutil

import (
	"context"
	stdsql "database/sql"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"chat-v2/internal/domain/ent"
)

// PostgresContainer holds a running PostgreSQL testcontainer and its DSN.
type PostgresContainer struct {
	Container *tcpostgres.PostgresContainer
	DSN       string
}

// RedisContainer holds a running Redis testcontainer and its address.
type RedisContainer struct {
	Container *tcredis.RedisContainer
	Addr      string
}

// StartPostgres spins up a PostgreSQL container for testing.
// Call Terminate() in TestMain cleanup.
func StartPostgres(ctx context.Context) (*PostgresContainer, error) {
	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get postgres DSN: %w", err)
	}

	return &PostgresContainer{
		Container: container,
		DSN:       dsn,
	}, nil
}

// StartRedis spins up a Redis container for testing.
func StartRedis(ctx context.Context) (*RedisContainer, error) {
	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		return nil, fmt.Errorf("failed to start redis container: %w", err)
	}

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get redis endpoint: %w", err)
	}

	return &RedisContainer{
		Container: container,
		Addr:      endpoint,
	}, nil
}

// NewEntClient creates a new Ent client connected to the test PostgreSQL container.
// It runs auto-migration to create all tables.
func NewEntClient(t *testing.T, dsn string) *ent.Client {
	t.Helper()

	// pgx/stdlib registers as driver "pgx", but Ent needs dialect "postgres"
	// Use database/sql to open, then wrap with Ent's dialect driver
	db, err := stdsql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv), ent.Log(t.Log))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Schema.Create(ctx); err != nil {
		client.Close()
		t.Fatalf("failed to create schema: %v", err)
	}

	t.Cleanup(func() {
		client.Close()
	})

	return client
}

// NewRedisClient creates a new Redis client connected to the test Redis container.
func NewRedisClient(t *testing.T, addr string) *redis.Client {
	t.Helper()

	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	t.Cleanup(func() {
		client.FlushAll(context.Background())
		client.Close()
	})

	return client
}

// CleanupEntData truncates all tables for a clean test slate.
// Use between tests that share the same Ent client.
func CleanupEntData(ctx context.Context, client *ent.Client) error {
	// Delete in order respecting foreign keys
	if _, err := client.ConversationParticipant.Delete().Exec(ctx); err != nil {
		return err
	}
	if _, err := client.Message.Delete().Exec(ctx); err != nil {
		return err
	}
	if _, err := client.Conversation.Delete().Exec(ctx); err != nil {
		return err
	}
	if _, err := client.User.Delete().Exec(ctx); err != nil {
		return err
	}
	return nil
}
