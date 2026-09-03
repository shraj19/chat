package message

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"chat-v2/internal/auth"
	"chat-v2/internal/conversation"
	"chat-v2/internal/metrics"
	"chat-v2/internal/pkg/httpx"
	"chat-v2/internal/pkg/logger"
)

type Handler struct {
	repo             *Repository
	convRepo         *conversation.Repository
	msgCache         MsgCache
	participantCache *conversation.ParticipantCache
}

func NewHandler(
	repo *Repository,
	convRepo *conversation.Repository,
	msgCache MsgCache,
	participantCache *conversation.ParticipantCache,
) *Handler {
	return &Handler{
		repo:             repo,
		convRepo:         convRepo,
		msgCache:         msgCache,
		participantCache: participantCache,
	}
}

func (h *Handler) List() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		userID, ok := auth.GetUserFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		convID, err := extractConversationID(r)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		isParticipant, err := h.participantCache.IsParticipant(r.Context(), convID, userID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Failed to check membership")
			return
		}
		if !isParticipant {
			httpx.WriteError(w, http.StatusForbidden, "Not a participant")
			return
		}

		var before *Cursor
		if beforeStr := r.URL.Query().Get("before"); beforeStr != "" {
			before, err = DecodeCursor(beforeStr)
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "Invalid cursor")
				return
			}
		}

		limit := 30
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if v, err := strconv.Atoi(limitStr); err == nil {
				limit = v
			}
		}

		// Try cache for first page
		if before == nil && h.msgCache != nil {

			start := time.Now()
			defer func() {
				metrics.CacheOperationsDuration.WithLabelValues("message").Observe(time.Since(start).Seconds())
			}()
			if cached, err := h.msgCache.GetRecent(r.Context(), convID); err == nil && len(cached) > 0 {
				httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": cached})

				// Cache hit
				metrics.CacheHitsTotal.WithLabelValues("message").Inc()
				return
			}

			// Cache miss
			metrics.CacheMissesTotal.WithLabelValues("message").Inc()
		}

		resp, err := h.repo.List(r.Context(), convID, before, limit)
		if err != nil {
			logger.Error("Failed to list messages", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "Failed to list messages")
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": resp})
	})
}

func extractConversationID(r *http.Request) (uuid.UUID, error) {
	idStr := r.URL.Query().Get("conversation_id")
	if idStr == "" {
		return uuid.Nil, fmt.Errorf("conversation_id is required")
	}
	return uuid.Parse(idStr)
}
