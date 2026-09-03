package realtime

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"

	"chat-v2/internal/pkg/logger"
)

// client contains the state for a single websocket connection.
type Client struct {
	conn       *websocket.Conn
	send       chan []byte // buffered channel for outbound messages
	userID     uuid.UUID
	username   string
	closeOnce  sync.Once
	mu         sync.Mutex
	lastActive time.Time
	limiter    *rate.Limiter
}

// NewClient creates a new Client instance with the given websocket connection, user ID, and username.
func NewClient(conn *websocket.Conn, userID uuid.UUID, username string) *Client {
	return &Client{
		conn:       conn,
		send:       make(chan []byte, 256),
		userID:     userID,
		username:   username,
		lastActive: time.Now(),
		limiter:    rate.NewLimiter(10, 20),
	}
}

// Close closes the websocket connection and cleans up resources.
// It is safe to call Close multiple times; subsequent calls will have no effect.
func (c *Client) Close() {
	// sync.once ensures that close logic is executed only once, even if Close is called multiple times.
	c.closeOnce.Do(func() {
		if c.conn != nil {
			c.conn.Close()
		}
	})
}

// SendMessage sends a message to the client through the send channel of client. 
// If the send buffer is full, it logs a warning and drops the message.
func (c *Client) SendMessage(msg []byte) {
	select {
	case c.send <- msg:
	default:
		logger.Warn("Client send buffer full", "user_id", c.userID)
	}
}

// UpdateLastActive updates the last active timestamp for the client to the current time.
func (c *Client) UpdateLastActive() {
	c.mu.Lock()
	c.lastActive = time.Now()
	c.mu.Unlock()
}

// LastActive returns the last active timestamp for the client. It is safe for concurrent use.
func (c *Client) LastActive() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastActive
}

func (c *Client) UserID() uuid.UUID     { return c.userID }
func (c *Client) Username() string      { return c.username }
func (c *Client) Send() <-chan []byte   { return c.send }
func (c *Client) Conn() *websocket.Conn { return c.conn }

// AllowMessage returns true if the client is allowed to send a message, false otherwise.
// It uses a rate limiter to prevent clients from sending messages too frequently.
func (c *Client) AllowMessage() bool {
	return c.limiter == nil || c.limiter.Allow()
}
