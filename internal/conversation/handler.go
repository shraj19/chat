package conversation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"chat-v2/internal/auth"
	"chat-v2/internal/domain/ent/conversation"
	"chat-v2/internal/pkg/logger"
	"chat-v2/internal/storage/redis"
	"chat-v2/internal/user"
)

type Handler struct {
	repo             *Repository
	userRepo         *user.Repository
	presence         *redis.PresenceStore
	participantCache *ParticipantCache
}

func NewHandler(repo *Repository, userRepo *user.Repository, presence *redis.PresenceStore, participantCache *ParticipantCache) *Handler {
	return &Handler{
		repo:             repo,
		userRepo:         userRepo,
		presence:         presence,
		participantCache: participantCache,
	}
}

func (h *Handler) Create() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		userID, ok := auth.GetUserFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		var req struct {
			Type                 string   `json:"type"`
			Title                string   `json:"title"`
			DisplayName          string   `json:"display_name"`
			ParticipantUsernames []string `json:"participant_usernames"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if req.Type == "" {
			req.Type = "group"
		}

		if req.Type == "group" && strings.TrimSpace(req.Title) == "" {
			writeError(w, http.StatusBadRequest, "Title is required for group conversations")
			return
		}

		currentUser, err := h.userRepo.GetByID(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to get user")
			return
		}

		// Dedupe usernames
		usernameSet := make(map[string]string)
		var usernameOrder []string
		for _, u := range req.ParticipantUsernames {
			clean := strings.TrimSpace(u)
			if clean == "" {
				continue
			}
			key := strings.ToLower(clean)
			if _, exists := usernameSet[key]; !exists {
				usernameSet[key] = clean
				usernameOrder = append(usernameOrder, clean)
			}
		}

		// Add current user
		key := strings.ToLower(currentUser.Username)
		if _, exists := usernameSet[key]; !exists {
			usernameOrder = append(usernameOrder, currentUser.Username)
		}

		if req.Type == "private" && len(usernameOrder) != 2 {
			writeError(w, http.StatusBadRequest, "Private conversations must have exactly 2 participants")
			return
		}

		var canonicalName string
		displayName := req.DisplayName

		if req.Type == "private" {
			names := make([]string, len(usernameOrder))
			for i, u := range usernameOrder {
				names[i] = strings.ToLower(u)
			}
			if names[0] > names[1] {
				names[0], names[1] = names[1], names[0]
			}
			canonicalName = names[0] + ":" + names[1]

			if displayName == "" {
				for _, u := range usernameOrder {
					if !strings.EqualFold(u, currentUser.Username) {
						displayName = u
						break
					}
				}
			}

			// Check if exists
			if existing, err := h.repo.GetByCanonicalName(r.Context(), canonicalName); err == nil && existing != nil {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"conversation": existing, "created": false})
				return
			}
		}

		conv, err := h.repo.CreateWithParticipants(r.Context(), req.Type, req.Title, displayName, canonicalName, usernameOrder)
		if err != nil {
			if errors.Is(err, ErrConversationExists) && canonicalName != "" {
				if existing, _ := h.repo.GetByCanonicalName(r.Context(), canonicalName); existing != nil {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"conversation": existing, "created": false})
					return
				}
			}
			logger.Error("Failed to create conversation", "error", err)
			writeError(w, http.StatusInternalServerError, "Failed to create conversation")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"conversation": conv, "created": true})
	})
}

func (h *Handler) List() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		userID, ok := auth.GetUserFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		convs, err := h.repo.GetByUserIDWithDisplay(r.Context(), userID)
		if err != nil {
			logger.Error("Failed to list conversations", "error", err)
			writeError(w, http.StatusInternalServerError, "Failed to list conversations")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"conversations": convs})
	})
}

func (h *Handler) Join() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		userID, ok := auth.GetUserFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		convID, err := extractConversationID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		conv, err := h.repo.GetByID(r.Context(), convID)
		if err != nil {
			writeError(w, http.StatusNotFound, "Conversation not found")
			return
		}

		if conv.Type == conversation.TypePrivate {
			writeError(w, http.StatusForbidden, "Cannot join private conversations")
			return
		}

		if err := h.repo.AddParticipant(r.Context(), convID, userID); err != nil {
			logger.Error("Failed to join conversation", "error", err)
			writeError(w, http.StatusInternalServerError, "Failed to join conversation")
			return
		}

		if h.participantCache != nil {
			h.participantCache.Add(r.Context(), convID, userID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Joined conversation"})
	})
}

func (h *Handler) Leave() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		userID, ok := auth.GetUserFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		convID, err := extractConversationID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		conv, err := h.repo.GetByID(r.Context(), convID)
		if err != nil {
			writeError(w, http.StatusNotFound, "Conversation not found")
			return
		}

		if conv.Type == conversation.TypePrivate {
			writeError(w, http.StatusForbidden, "Cannot leave private conversations")
			return
		}

		if err := h.repo.RemoveParticipant(r.Context(), convID, userID); err != nil {
			logger.Error("Failed to leave conversation", "error", err)
			writeError(w, http.StatusInternalServerError, "Failed to leave conversation")
			return
		}

		if h.participantCache != nil {
			h.participantCache.Remove(r.Context(), convID, userID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Left conversation"})
	})
}

func (h *Handler) Members() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		userID, ok := auth.GetUserFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		convID, err := extractConversationID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		isParticipant, err := h.repo.IsParticipant(r.Context(), convID, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to check membership")
			return
		}
		if !isParticipant {
			writeError(w, http.StatusForbidden, "Not a participant")
			return
		}

		participants, err := h.repo.GetParticipants(r.Context(), convID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to get participants")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"participants": participants})
	})
}

func (h *Handler) Presence() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		userID, ok := auth.GetUserFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		friends, err := h.repo.GetFriends(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to get friends")
			return
		}

		presenceInfo, err := h.presence.GetBulk(r.Context(), friends)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to get presence")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(presenceInfo)
	})
}

func extractConversationID(r *http.Request) (uuid.UUID, error) {
	idStr := r.URL.Query().Get("conversation_id")
	if idStr == "" {
		return uuid.Nil, fmt.Errorf("conversation_id is required")
	}
	return uuid.Parse(idStr)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
