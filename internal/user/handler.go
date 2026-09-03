package user

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"chat-v2/internal/auth"
	"chat-v2/internal/domain/ent"
	"chat-v2/internal/pkg/httpx"
	"chat-v2/internal/pkg/logger"
	"chat-v2/internal/pkg/validator"
)

type Handler struct {
	repo *Repository
	jwt  *auth.JWTMaker
}

func NewHandler(repo *Repository, jwt *auth.JWTMaker) *Handler {
	return &Handler{repo: repo, jwt: jwt}
}

type SignUpRequest struct {
	Username string `json:"username" validate:"required,min=3,max=20,alphanum_underscore"`
	Password string `json:"password" validate:"required,min=8"`
	Email    string `json:"email" validate:"required,email"`
}

type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

func (h *Handler) SignUp() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		var req SignUpRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "Invalid request payload")
			return
		}

		if err := validator.Validate(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		hashedPassword, err := auth.HashPassword(req.Password)
		if err != nil {
			logger.Error("Failed to hash password", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "Failed to create user")
			return
		}

		user, err := h.repo.Create(r.Context(), req.Username, hashedPassword, req.Email)
		if err != nil {
			if errors.Is(err, ErrUserExists) {
				httpx.WriteError(w, http.StatusConflict, "Username or email already exists")
				return
			}
			logger.Error("Failed to create user", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "Failed to create user")
			return
		}

		logger.Info("User created", "username", req.Username)
		httpx.WriteJSON(w, http.StatusCreated, map[string]string{
			"user_id":  user.ID.String(),
			"username": user.Username,
			"email":    user.Email,
		})
	})
}

func (h *Handler) Login() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "Invalid request payload")
			return
		}

		if err := validator.Validate(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		user, err := h.repo.GetByUsername(r.Context(), req.Username)
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "Invalid username or password")
			return
		}

		if !auth.CheckPassword(req.Password, user.PasswordHash) {
			httpx.WriteError(w, http.StatusUnauthorized, "Invalid username or password")
			return
		}

		now := time.Now()
		token, err := h.jwt.CreateToken(user.ID, 24*time.Hour)
		if err != nil {
			logger.Error("Failed to create token", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "Failed to create token")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "access_token",
			Value:    token,
			Expires:  now.Add(24 * time.Hour),
			Path:     "/api/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"status":     "Login successful",
			"user_id":    user.ID.String(),
			"username":   user.Username,
			"email":      user.Email,
			"created_at": user.CreatedAt.Format(time.RFC3339),
			"expires_at": now.Add(24 * time.Hour).Format(time.RFC3339),
		})
	})
}

func (h *Handler) Logout() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "access_token",
			Value:    "",
			Expires:  time.Unix(0, 0),
			Path:     "/api/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "Logout successful"})
	})
}

func (h *Handler) Me() http.Handler {
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

		user, err := h.repo.GetByID(r.Context(), userID)
		if err != nil {
			if ent.IsNotFound(err) {
				httpx.WriteError(w, http.StatusNotFound, "User not found")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "Failed to get user")
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"user_id":    user.ID.String(),
			"username":   user.Username,
			"email":      user.Email,
			"created_at": user.CreatedAt.Format(time.RFC3339),
		})
	})
}

func (h *Handler) Search() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		_, ok := auth.GetUserFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			httpx.WriteError(w, http.StatusBadRequest, "q query parameter is required")
			return
		}
		if utf8.RuneCountInString(q) > 20 {
			httpx.WriteError(w, http.StatusBadRequest, "q query parameter is too long")
			return
		}
		for _, c := range q {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
				httpx.WriteError(w, http.StatusBadRequest, "q contains invalid characters")
				return
			}
		}

		limit := 10
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 25 {
				limit = v
			}
		}

		users, err := h.repo.Search(r.Context(), q, limit)
		if err != nil {
			logger.Error("Failed to search users", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "Failed to search users")
			return
		}

		type userResponse struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		}
		result := make([]userResponse, len(users))
		for i, u := range users {
			result[i] = userResponse{ID: u.ID.String(), Username: u.Username}
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": result})
	})
}
