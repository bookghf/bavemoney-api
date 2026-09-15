// Package httpx holds the HTTP plumbing shared by every handler.
package httpx

import (
	"encoding/json"
	"net/http"
	"strings"
)

// WriteJSON writes payload as a JSON body with the given status.
func WriteJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError writes a `{"error": "..."}` body with the given status.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

// WriteMessage writes a `{"message": "..."}` body with the given status.
func WriteMessage(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"message": message})
}

// DecodeJSON reads the request body into dst, reporting a 400 on malformed JSON.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	return true
}

// PathID returns the resource id that follows prefix in the request path,
// reporting a 404 when it is missing.
func PathID(w http.ResponseWriter, r *http.Request, prefix string) (string, bool) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
	if id == "" {
		w.WriteHeader(http.StatusNotFound)
		return "", false
	}
	return id, true
}

// MethodNotAllowed replies with 405.
func MethodNotAllowed(w http.ResponseWriter) {
	w.WriteHeader(http.StatusMethodNotAllowed)
}
