package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"chat-v2/internal/auth"
	"chat-v2/internal/conversation"
	"chat-v2/internal/message"
	"chat-v2/internal/metrics"
	"chat-v2/internal/pkg/logger"
	"chat-v2/internal/storage/redis"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMsgSize = 4 * 1024
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

// Handler manages websocket connections and message routing.
type Handler struct {
	hub              *Hub
	convRepo         *conversation.Repository
	participantCache *conversation.ParticipantCache
	msgService       *message.CachedService
	presence         *redis.PresenceStore
	upgrader         websocket.Upgrader
}

// NewHandler creates a new Handler instance with the provided dependencies.
func NewHandler(hub *Hub, convRepo *conversation.Repository, participantCache *conversation.ParticipantCache, msgService *message.CachedService, presence *redis.PresenceStore, allowedOrigins []string) *Handler {
	return &Handler{
		hub:              hub,
		convRepo:         convRepo,
		participantCache: participantCache,
		msgService:       msgService,
		presence:         presence,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				return isAllowedOrigin(origin, allowedOrigins)
			},
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("WebSocket upgrade failed", "error", err)
		return
	}

	username := r.URL.Query().Get("username")
	client := NewClient(conn, userID, username)

	// Increment the active WebSocket connections metric when a new client connects.
	metrics.WSConnectionsActive.Inc()

	// We can't defer the decrement here because we want to ensure it happens when
	// the client disconnects, not when this function exits.
	// decrement is in client.Close() which is called in readPump and writePump when the connection is closed.
	// also it is idempotent, so if it is called multiple times, it will not decrement below 0.

	h.hub.Register(client)
	go h.writePump(client)
	go h.readPump(client)
}

// writePump handles outgoing messages to the client, including ping messages to keep the connection alive.
func (h *Handler) writePump(client *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		client.Close()
	}()

	for {
		select {
		case msg, ok := <-client.Send():
			clientConn := client.Conn()

			// If a write operation takes longer than writeWait, the connection is closed to prevent hanging.
			clientConn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// The channel is closed, so we send a close message to the client and exit.
				clientConn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := clientConn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(msg)

			// Add queued chat messages to the current websocket message.
			n := len(client.send)
			for range n {
				w.Write(newline)
				w.Write(<-client.send)
			}

			if err := w.Close(); err != nil {
				return
			}

			// Count all messages flushed in this frame (the first plus any batched).
			metrics.WSMessagesTotal.WithLabelValues("outbound").Add(float64(n + 1))

			// Update the last active timestamp for the client after sending a message.
			client.UpdateLastActive()

		case <-ticker.C:
			// Send a ping message to the client to keep the connection alive.
			client.Conn().SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.Conn().WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump handles incoming messages from the client.
func (h *Handler) readPump(client *Client) {
	defer func() {
		// any exit from readPump should unregister the client and close the connection
		h.hub.Unregister(client)
		client.Close()
	}()

	if h.presence != nil {
		go h.presence.Update(context.Background(), client.UserID())
	}

	client.Conn().SetReadLimit(maxMsgSize)
	client.Conn().SetReadDeadline(time.Now().Add(pongWait))
	client.Conn().SetPongHandler(func(string) error {
		client.Conn().SetReadDeadline(time.Now().Add(pongWait))
		// Update the last active timestamp for the client when a pong is received.
		client.UpdateLastActive()
		if h.presence != nil {
			go h.presence.Update(context.Background(), client.UserID())
		}
		return nil
	})

	for {
		_, data, err := client.Conn().ReadMessage()
		// errors from ReadMessage indicate that the connection has been closed or an error occurred,
		// so we exit the loop and close the connection.
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error("Unexpected WebSocket close error", "error", err)
			}
			return
		}

		// Count every frame received off the wire (before the rate-limit gate,
		// so rejected/abusive traffic stays visible in inbound throughput).
		metrics.WSMessagesTotal.WithLabelValues("inbound").Inc()

		if !client.AllowMessage() {
			metrics.RateLimitHitsTotal.WithLabelValues("ws").Inc()
			errMsg, _ := json.Marshal(map[string]string{"type": "error", "message": "Rate limit exceeded"})
			client.SendMessage(errMsg)
			continue
		}

		client.UpdateLastActive()
		h.handleMessage(client, data)
	}
}

// incomingMessage represents the structure of messages received from clients over the websocket connection.
type incomingMessage struct {
	Type           string    `json:"type"`
	ConversationID uuid.UUID `json:"conversation_id"`
	Content        string    `json:"content"`
	Username       string    `json:"username"`
	ClientID       string    `json:"client_id"`
}

// handleMessage processes incoming messages based on their type and performs the appropriate actions.
func (h *Handler) handleMessage(client *Client, data []byte) {
	var msg incomingMessage
	if err := json.Unmarshal(data, &msg); err != nil || msg.ConversationID == uuid.Nil {
		return
	}

	// Time how long we take to handle this frame, labeled by type. Only valid,
	// processed frames are observed (malformed ones returned above).
	start := time.Now()
	defer func() {
		metrics.WSMessageProcessingDuration.WithLabelValues(msg.Type).Observe(time.Since(start).Seconds())
	}()

	switch msg.Type {
	case "message":
		if msg.Content == "" {
			return
		}

		_, err := h.msgService.Create(context.Background(), client.UserID(), msg.ConversationID, msg.Content, msg.Username, msg.ClientID)
		if err != nil {
			logger.Error("Failed to create message", "error", err)
		}

	case "subscribe":
		if ok, _ := h.participantCache.IsParticipant(context.Background(), msg.ConversationID, client.UserID()); ok {
			h.hub.Subscribe(client, msg.ConversationID)
		}
		// Send an acknowledgment back to the client for the subscription request.
		// or send an error message if the subscription failed.

	case "unsubscribe":
		h.hub.Unsubscribe(client, msg.ConversationID)
		// Send an acknowledgment back to the client for the unsubscription request.
		// or send an error message if the unsubscription failed.
	}
}

func isAllowedOrigin(origin string, allowed []string) bool {
	origin = strings.TrimSpace(strings.TrimSuffix(origin, "/"))
	if len(allowed) == 0 {
		return strings.HasPrefix(origin, "http://localhost") || strings.HasPrefix(origin, "http://127.0.0.1")
	}
	for _, a := range allowed {
		a = strings.TrimSpace(strings.TrimSuffix(a, "/"))
		if a == "*" || strings.EqualFold(origin, a) {
			return true
		}
	}
	return false
}
