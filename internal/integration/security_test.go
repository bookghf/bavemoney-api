//go:build integration

package integration

import (
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestAdminAPIRequiresAdminToken(t *testing.T) {
	u := register(t)
	for _, path := range []string{
		"/api/v1/admin/analytics/overview",
		"/api/v1/admin/users",
		"/api/v1/admin/currencies",
		"/api/v1/admin/exchange-rates",
		"/api/v1/admin/categories",
	} {
		expect(t, call(t, "GET", path, "", nil), http.StatusUnauthorized, path+" without token")
		expect(t, call(t, "GET", path, u.Token, nil), http.StatusUnauthorized, path+" with a user token")
	}
	expect(t, call(t, "PATCH", "/api/v1/admin/users/"+u.ID+"/suspend", u.Token, map[string]string{"status": "suspended"}),
		http.StatusUnauthorized, "suspend with a user token")
}

func createAdmin(t *testing.T, role string) string {
	t.Helper()
	email := newEmail()
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin password"), bcrypt.MinCost)
	if _, err := db.Exec(`INSERT INTO admin_users (email, password_hash, role) VALUES ($1, $2, $3)`, email, string(hash), role); err != nil {
		t.Fatal(err)
	}
	res := call(t, "POST", "/api/v1/admin/auth/login", "", map[string]string{"email": email, "password": "admin password"})
	expect(t, res, http.StatusOK, "admin login")
	if res.str("refresh_token") != "" {
		t.Error("admin login must not hand out a guessable refresh token")
	}
	return res.str("access_token")
}

func TestAdminRolesSuspensionAndAudit(t *testing.T) {
	support := createAdmin(t, "support")
	admin := createAdmin(t, "admin")
	u := register(t)

	expect(t, call(t, "GET", "/api/v1/admin/users?search=qa-it&status=active", support, nil), http.StatusOK, "support lists users")
	expect(t, call(t, "GET", "/api/v1/admin/exchange-rates?base=THB", support, nil), http.StatusOK, "support lists rates")
	expect(t, call(t, "PATCH", "/api/v1/admin/users/"+u.ID+"/suspend", support, map[string]string{"status": "suspended"}),
		http.StatusForbidden, "support can not suspend")

	// An admin token is not a user token either.
	expect(t, call(t, "GET", "/api/v1/accounts", admin, nil), http.StatusUnauthorized, "admin token on the user API")

	expect(t, call(t, "PATCH", "/api/v1/admin/users/"+u.ID+"/suspend", admin, map[string]string{"status": "suspended"}),
		http.StatusOK, "admin suspends")
	expect(t, call(t, "POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": u.Refresh}),
		http.StatusUnauthorized, "suspended user's refresh token was revoked")
	expect(t, call(t, "POST", "/api/v1/auth/login", "", map[string]string{"email": u.Email, "password": "correct horse"}),
		http.StatusForbidden, "suspended user can not log in")

	var audited int
	if err := db.QueryRow(`SELECT COUNT(*) FROM admin_audit_log WHERE target_id = $1`, u.ID).Scan(&audited); err != nil || audited != 1 {
		t.Errorf("audit rows for the suspension = %d (err %v), want 1", audited, err)
	}
	expect(t, call(t, "PATCH", "/api/v1/admin/users/00000000-0000-4000-8000-000000000000/suspend", admin, map[string]string{"status": "suspended"}),
		http.StatusNotFound, "suspend unknown user")
}

func TestCategoryOwnershipIsEnforced(t *testing.T) {
	a, b := register(t), register(t)
	accountA := createAccount(t, a, "A", "THB", "0")

	res := call(t, "POST", "/api/v1/categories", b.Token, map[string]string{"name": "B secret medical", "type": "expense"})
	expect(t, res, http.StatusCreated, "B creates a category")
	secret := res.str("id")

	res = createTx(t, a, map[string]interface{}{"account_id": accountA, "category_id": secret, "type": "expense", "amount": "1"})
	expect(t, res, http.StatusBadRequest, "A attaches B's category")

	res = createTx(t, a, map[string]interface{}{"account_id": accountA, "type": "expense", "amount": "1"})
	expect(t, res, http.StatusCreated, "A creates an expense")
	txID := res.str("id")
	expect(t, call(t, "PATCH", "/api/v1/transactions/"+txID, a.Token, map[string]string{"category_id": secret}),
		http.StatusBadRequest, "A patches in B's category")

	// Category type must match the transaction type.
	expect(t, createTx(t, a, map[string]interface{}{"account_id": accountA, "category_id": systemCategory(t, "income"), "type": "expense", "amount": "1"}),
		http.StatusBadRequest, "expense with an income category")
	// Changing the type while keeping a mismatched category is refused too.
	res = createTx(t, a, map[string]interface{}{"account_id": accountA, "category_id": systemCategory(t, "expense"), "type": "expense", "amount": "1"})
	expect(t, res, http.StatusCreated, "expense with an expense category")
	expect(t, call(t, "PATCH", "/api/v1/transactions/"+res.str("id"), a.Token, map[string]string{"type": "income"}),
		http.StatusBadRequest, "flip to income keeping an expense category")
}

func TestAttachmentsAreScopedToTheirTransaction(t *testing.T) {
	a, b := register(t), register(t)
	accountA := createAccount(t, a, "A", "THB", "0")
	accountB := createAccount(t, b, "B", "THB", "0")
	txA := createTx(t, a, map[string]interface{}{"account_id": accountA, "type": "expense", "amount": "1"}).str("id")
	txA2 := createTx(t, a, map[string]interface{}{"account_id": accountA, "type": "expense", "amount": "2"}).str("id")
	txB := createTx(t, b, map[string]interface{}{"account_id": accountB, "type": "expense", "amount": "1"}).str("id")

	upload := map[string]interface{}{"filename": "r.jpg", "content_type": "image/jpeg", "file_size_bytes": 100}
	res := call(t, "POST", "/api/v1/transactions/"+txB+"/attachment", b.Token, upload)
	expect(t, res, http.StatusOK, "B starts an upload")
	attachmentB := res.str("attachment_id")

	// A owns txA but the attachment is B's: confirm and delete must not touch it.
	expect(t, call(t, "POST", "/api/v1/transactions/"+txA+"/attachment/confirm/"+attachmentB, a.Token, nil),
		http.StatusNotFound, "A confirms B's attachment via A's transaction")
	expect(t, call(t, "DELETE", "/api/v1/transactions/"+txA+"/attachment/"+attachmentB, a.Token, nil),
		http.StatusNotFound, "A deletes B's attachment via A's transaction")
	expect(t, call(t, "POST", "/api/v1/transactions/"+txA+"/attachment", a.Token, upload), http.StatusOK, "A uploads to txA")
	res = call(t, "POST", "/api/v1/transactions/"+txA+"/attachment", a.Token, upload)
	attachmentA := res.str("attachment_id")
	expect(t, call(t, "DELETE", "/api/v1/transactions/"+txA2+"/attachment/"+attachmentA, a.Token, nil),
		http.StatusNotFound, "attachment addressed through the wrong transaction")
	expect(t, call(t, "DELETE", "/api/v1/transactions/"+txA+"/attachment/"+attachmentA, a.Token, nil), http.StatusOK, "own attachment")

	var still int
	_ = db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE id = $1`, attachmentB).Scan(&still)
	if still != 1 {
		t.Error("B's attachment was deleted")
	}
}

func TestRefreshTokenReuseRevokesTheFamily(t *testing.T) {
	u := register(t)
	first := call(t, "POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": u.Refresh})
	expect(t, first, http.StatusOK, "first refresh")
	rotated := first.str("refresh_token")

	// Make the original token look rotated long ago, past the race grace window.
	if _, err := db.Exec(`UPDATE refresh_tokens SET revoked_at = now() - interval '1 minute' WHERE user_id = $1 AND revoked_at IS NOT NULL`, u.ID); err != nil {
		t.Fatal(err)
	}
	expect(t, call(t, "POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": u.Refresh}),
		http.StatusUnauthorized, "replayed refresh token")
	expect(t, call(t, "POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": rotated}),
		http.StatusUnauthorized, "the rotated token died with its family")
}

func TestRegistrationRules(t *testing.T) {
	u := register(t)
	upper := map[string]string{"email": "  " + toUpper(u.Email) + " ", "password": "correct horse"}
	expect(t, call(t, "POST", "/api/v1/auth/login", "", upper), http.StatusOK, "login is case-insensitive")
	expect(t, call(t, "POST", "/api/v1/auth/register", "", upper), http.StatusConflict, "same email in another case")

	for name, body := range map[string]map[string]string{
		"not an email":     {"email": "qa-it+notanemail", "password": "correct horse"},
		"short password":   {"email": newEmail(), "password": "1"},
		"unknown currency": {"email": newEmail(), "password": "correct horse", "default_currency": "XYZ"},
	} {
		expect(t, call(t, "POST", "/api/v1/auth/register", "", body), http.StatusBadRequest, name)
	}

	res := call(t, "POST", "/api/v1/auth/register", "", map[string]string{"email": newEmail(), "password": "correct horse"})
	expect(t, res, http.StatusCreated, "register without currency")
	if got := res.str("user", "default_currency"); got != "THB" {
		t.Errorf("default currency = %q, want THB", got)
	}
}

func toUpper(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - 32
		}
	}
	return string(out)
}
