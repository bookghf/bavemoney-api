//go:build integration

package integration

import (
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// knownResetCode replaces u's emailed code with one the test knows, since the
// test can not read the email.
func knownResetCode(t *testing.T, u user, code string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	result, err := db.Exec(`UPDATE password_reset_codes SET code_hash = $2 WHERE user_id = $1`, u.ID, string(hash))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		t.Fatalf("no reset code stored for %s", u.Email)
	}
}

func resetWith(t *testing.T, u user, code, password string) response {
	t.Helper()
	return call(t, "POST", "/api/v1/auth/reset-password", "", map[string]string{
		"email": u.Email, "code": code, "new_password": password,
	})
}

func TestForgotPassword(t *testing.T) {
	u := register(t)

	expect(t, call(t, "POST", "/api/v1/auth/forgot-password", "", map[string]string{"email": "not an email"}),
		http.StatusBadRequest, "invalid email")
	expect(t, call(t, "POST", "/api/v1/auth/forgot-password", "", map[string]string{"email": newEmail()}),
		http.StatusAccepted, "unknown email looks the same")
	expect(t, call(t, "POST", "/api/v1/auth/forgot-password", "", map[string]string{"email": "  " + u.Email + " "}),
		http.StatusAccepted, "forgot")

	var firstHash string
	if err := db.QueryRow(`SELECT code_hash FROM password_reset_codes WHERE user_id = $1`, u.ID).Scan(&firstHash); err != nil {
		t.Fatalf("no code stored: %v", err)
	}
	expect(t, call(t, "POST", "/api/v1/auth/forgot-password", "", map[string]string{"email": u.Email}),
		http.StatusAccepted, "second forgot")
	var secondHash string
	_ = db.QueryRow(`SELECT code_hash FROM password_reset_codes WHERE user_id = $1`, u.ID).Scan(&secondHash)
	if secondHash != firstHash {
		t.Error("a second request within the cooldown replaced the code (and sent another email)")
	}

	knownResetCode(t, u, "042137")
	expect(t, resetWith(t, u, "000000", "brand new pass"), http.StatusBadRequest, "wrong code")
	expect(t, resetWith(t, u, "042137", "short"), http.StatusBadRequest, "weak password")

	res := resetWith(t, u, "042137", "brand new pass")
	expect(t, res, http.StatusOK, "reset")
	if res.str("access_token") == "" || res.str("refresh_token") == "" {
		t.Fatal("reset did not sign in")
	}
	expect(t, call(t, "GET", "/api/v1/me", res.str("access_token"), nil), http.StatusOK, "new session works")
	expect(t, call(t, "POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": u.Refresh}),
		http.StatusUnauthorized, "old sessions are signed out")
	expect(t, call(t, "POST", "/api/v1/auth/login", "", map[string]string{"email": u.Email, "password": "correct horse"}),
		http.StatusUnauthorized, "old password")
	expect(t, call(t, "POST", "/api/v1/auth/login", "", map[string]string{"email": u.Email, "password": "brand new pass"}),
		http.StatusOK, "new password")
	expect(t, resetWith(t, u, "042137", "another new pass"), http.StatusBadRequest, "a code works once")

	expect(t, resetWith(t, user{Email: newEmail()}, "042137", "brand new pass"), http.StatusBadRequest, "unknown email")
}

func TestResetCodeLimits(t *testing.T) {
	u := register(t)
	expect(t, call(t, "POST", "/api/v1/auth/forgot-password", "", map[string]string{"email": u.Email}),
		http.StatusAccepted, "forgot")
	knownResetCode(t, u, "123456")
	for i := 0; i < 5; i++ {
		expect(t, resetWith(t, u, "654321", "brand new pass"), http.StatusBadRequest, "wrong code")
	}
	expect(t, resetWith(t, u, "123456", "brand new pass"), http.StatusBadRequest, "right code after 5 wrong tries")

	expired := register(t)
	expect(t, call(t, "POST", "/api/v1/auth/forgot-password", "", map[string]string{"email": expired.Email}),
		http.StatusAccepted, "forgot")
	knownResetCode(t, expired, "123456")
	if _, err := db.Exec(`UPDATE password_reset_codes SET expires_at = now() - interval '1 minute' WHERE user_id = $1`, expired.ID); err != nil {
		t.Fatal(err)
	}
	expect(t, resetWith(t, expired, "123456", "brand new pass"), http.StatusBadRequest, "expired code")
}
