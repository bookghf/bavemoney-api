package user

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"ledger-api/internal/httpx"
	"ledger-api/internal/mail"
	"ledger-api/internal/validate"
)

// Reset codes are short, so they are guarded by a short life and few tries.
const (
	resetCodeTTL      = 15 * time.Minute
	resetCodeAttempts = 5
	// resetCodeCooldown stops one address from being flooded with emails.
	resetCodeCooldown = time.Minute
	mailTimeout       = 15 * time.Second
)

// errResetCode is the single answer for a wrong, expired, used-up, or unknown
// code, so the endpoint does not reveal which accounts exist.
const errResetCode = "code is invalid or expired"

// ForgotPasswordRequest is the POST /auth/forgot-password payload.
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// ResetPasswordRequest is the POST /auth/reset-password payload.
type ResetPasswordRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

// forgotPassword emails a 6-digit reset code to an active account. The reply
// is the same whether or not the email has an account, and the email is sent
// in the background so response time does not tell either.
func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}
	var req ForgotPasswordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	email := validate.NormalizeEmail(req.Email)
	if !validate.Email(email) {
		httpx.WriteError(w, http.StatusBadRequest, "email is not valid")
		return
	}

	accepted := map[string]string{"message": "if this email has an account, a code was sent"}
	owner, _, err := h.repo.ByEmail(r.Context(), email)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteJSON(w, http.StatusAccepted, accepted)
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not send code")
		return
	}
	if owner.Status != statusActive {
		httpx.WriteJSON(w, http.StatusAccepted, accepted)
		return
	}

	code, err := newResetCode()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not send code")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not send code")
		return
	}
	saved, err := h.repo.SaveResetCode(r.Context(), owner.ID, string(hash), resetCodeTTL, resetCodeCooldown)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not send code")
		return
	}
	if saved {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), mailTimeout)
		go func() {
			defer cancel()
			if err := h.mail.Send(ctx, resetCodeMessage(owner.Email, code)); err != nil {
				slog.Error("send reset code", "error", err)
			}
		}()
	}
	httpx.WriteJSON(w, http.StatusAccepted, accepted)
}

// resetPassword sets a new password with a code from forgotPassword. Every
// existing session is signed out and a fresh one is returned for this device.
func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.MethodNotAllowed(w)
		return
	}
	var req ResetPasswordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := validate.Password(req.NewPassword); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	owner, _, err := h.repo.ByEmail(r.Context(), validate.NormalizeEmail(req.Email))
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusBadRequest, errResetCode)
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not reset password")
		return
	}
	// The try is counted before the check, so parallel guesses share the limit.
	codeHash, err := h.repo.UseResetCodeAttempt(r.Context(), owner.ID, resetCodeAttempts)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.WriteError(w, http.StatusBadRequest, errResetCode)
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not reset password")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(req.Code)) != nil {
		httpx.WriteError(w, http.StatusBadRequest, errResetCode)
		return
	}
	if owner.Status != statusActive {
		httpx.WriteError(w, http.StatusForbidden, "this account is suspended")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	if err := h.repo.ResetPassword(r.Context(), owner.ID, string(newHash)); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not reset password")
		return
	}
	if err := h.refresh.RevokeAll(r.Context(), owner.ID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not sign out other devices")
		return
	}
	h.writeSession(w, r, owner, http.StatusOK)
}

// newResetCode returns a random 6-digit code, keeping leading zeros.
func newResetCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// resetCodeMessage is the reset email, in Thai first (the app's main market)
// and then English.
func resetCodeMessage(to, code string) mail.Message {
	minutes := int(resetCodeTTL.Minutes())
	return mail.Message{
		To:      to,
		Subject: fmt.Sprintf("รหัสรีเซ็ตรหัสผ่าน / Password reset code: %s", code),
		Text: fmt.Sprintf(`รหัสสำหรับตั้งรหัสผ่านใหม่ของคุณคือ %[1]s
รหัสนี้ใช้ได้ %[2]d นาที หากคุณไม่ได้ขอรีเซ็ตรหัสผ่าน ไม่ต้องทำอะไร รหัสผ่านเดิมยังใช้ได้

Your password reset code is %[1]s
It expires in %[2]d minutes. If you did not ask to reset your password, you can ignore this email.
`, code, minutes),
	}
}
