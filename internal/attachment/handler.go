package attachment

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"ledger-api/internal/auth"
	"ledger-api/internal/httpx"
)

const basePath = "/api/v1/transactions"

type Handler struct {
	repo *Repository
	auth *auth.Authenticator
}

func NewHandler(repo *Repository, authenticator *auth.Authenticator) *Handler {
	return &Handler{repo: repo, auth: authenticator}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc(basePath+"/", h.attachmentRouter)
}

func (h *Handler) attachmentRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, basePath+"/")
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")

	if len(parts) < 2 || len(parts) > 4 {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	transactionID := parts[0]
	if transactionID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "transaction_id is required")
		return
	}

	if len(parts) >= 2 && parts[1] == "attachment" {
		if len(parts) == 2 {
			h.presignedURL(w, r, transactionID)
		} else if len(parts) == 4 && parts[2] == "confirm" {
			h.confirm(w, r, transactionID, parts[3])
		} else if len(parts) == 3 {
			h.deleteAttachment(w, r, transactionID, parts[2])
		} else {
			httpx.WriteError(w, http.StatusNotFound, "not found")
		}
	} else {
		httpx.WriteError(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) presignedURL(w http.ResponseWriter, r *http.Request, transactionID string) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}

	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}

	owns, err := h.repo.VerifyOwnership(r.Context(), userID, transactionID)
	if err != nil || !owns {
		httpx.WriteError(w, http.StatusNotFound, "transaction not found")
		return
	}

	var req PresignedURLRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	if req.Filename == "" || req.ContentType == "" || req.FileSizeBytes <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "filename, content_type, and file_size_bytes are required")
		return
	}

	if req.FileSizeBytes > 10485760 {
		httpx.WriteError(w, http.StatusBadRequest, "file too large (max 10MB)")
		return
	}

	allowedTypes := map[string]bool{
		"image/jpeg":      true,
		"image/png":       true,
		"image/webp":      true,
		"application/pdf": true,
	}
	if !allowedTypes[req.ContentType] {
		httpx.WriteError(w, http.StatusBadRequest, "unsupported content type")
		return
	}

	attachmentID, err := h.repo.Create(r.Context(), transactionID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not create attachment")
		return
	}

	presignedURL := fmt.Sprintf("https://storage.example.com/receipts/%s/%s", transactionID, attachmentID)

	resp := PresignedURLResponse{
		AttachmentID:     attachmentID,
		UploadURL:        presignedURL,
		UploadMethod:     "PUT",
		UploadHeaders:    map[string]string{"Content-Type": req.ContentType},
		ExpiresInSeconds: 300,
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) confirm(w http.ResponseWriter, r *http.Request, transactionID, attachmentID string) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}

	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}

	owns, err := h.repo.VerifyOwnership(r.Context(), userID, transactionID)
	if err != nil || !owns {
		httpx.WriteError(w, http.StatusNotFound, "transaction not found")
		return
	}

	att, err := h.repo.Get(r.Context(), attachmentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "attachment not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "could not fetch attachment")
		return
	}

	fileURL := fmt.Sprintf("https://cdn.example.com/receipts/%s", attachmentID)
	if err := h.repo.UpdateURL(r.Context(), attachmentID, fileURL, 0); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not confirm upload")
		return
	}

	att.FileURL = fileURL
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"attachment": att})
}

func (h *Handler) deleteAttachment(w http.ResponseWriter, r *http.Request, transactionID, attachmentID string) {
	userID, ok := h.auth.Require(w, r)
	if !ok {
		return
	}

	if r.Method != http.MethodDelete {
		httpx.MethodNotAllowed(w)
		return
	}

	owns, err := h.repo.VerifyOwnership(r.Context(), userID, transactionID)
	if err != nil || !owns {
		httpx.WriteError(w, http.StatusNotFound, "transaction not found")
		return
	}

	if err := h.repo.Delete(r.Context(), attachmentID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not delete attachment")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Attachment deleted",
		"id":      attachmentID,
	})
}
