// httpx handles HTTP responses and errors in a structured way.
package httpx

import (
	"encoding/json"
	"net/http"

	"chat-v2/internal/pkg/logger"
)

// WriteJSON writes the given data as a JSON response with the specified HTTP status code.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if data == nil {
		return
	}

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		// Headers have already been written, so we cannot change the status code.
		// We can only log the error and return.

		logger.Error("Failed to write JSON response", "error", err)
		return
	}
}

// WriteError writes an error response as JSON with the specified HTTP status code and message.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}
