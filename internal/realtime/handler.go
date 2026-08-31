package realtime

import (
	"chat-v2/internal/auth"
	"chat-v2/internal/conversation"
	"chat-v2/internal/message"
	"chat-v2/internal/pkg/logger"
	"chat-v2/internal/storage/redis"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMsgSize = 4 * 1024
)

type Handler struct {
	hub              *Hub
	convRepo         *conversation.Repository
	participantCache *conversation.ParticipantCache
	msgService       *message.CachedService
	presence         *redis.PresenceStore
	upgrader         websocket.Upgrader
}

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

	h.hub.Register(client)
	go h.writePump(client)
	go h.readPump(client)
}

func (h *Handler) writePump(client *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		client.Close()
	}()

	for {
		select {
		case msg, ok := <-client.Send():
			if !ok {
				return
			}
			client.Conn().SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.Conn().WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
			client.UpdateLastActive()

		case <-ticker.C:
			client.Conn().SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.Conn().WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (h *Handler) readPump(client *Client) {
	defer func() {
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
		client.UpdateLastActive()
		if h.presence != nil {
			go h.presence.Update(context.Background(), client.UserID())
		}
		return nil
	})

	for {
		_, data, err := client.Conn().ReadMessage()
		if err != nil {
			return
		}

		if !client.AllowMessage() {
			errMsg, _ := json.Marshal(map[string]string{"type": "error", "message": "Rate limit exceeded"})
			client.SendMessage(errMsg)
			continue
		}

		client.UpdateLastActive()
		h.handleMessage(client, data)
	}
}

type incomingMessage struct {
	Type           string    `json:"type"`
	ConversationID uuid.UUID `json:"conversation_id"`
	Content        string    `json:"content"`
	Username       string    `json:"username"`
	ClientID       string    `json:"client_id"`
}

func (h *Handler) handleMessage(client *Client, data []byte) {
	var msg incomingMessage
	if err := json.Unmarshal(data, &msg); err != nil || msg.ConversationID == uuid.Nil {
		return
	}

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

	case "unsubscribe":
		h.hub.Unsubscribe(client, msg.ConversationID)
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
