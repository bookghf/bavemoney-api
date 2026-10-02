package auth

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestUserAndAdminTokensAreNotInterchangeable(t *testing.T) {
	a := New("test-secret")

	userToken, err := a.IssueAccessToken("user-1", "u@example.com")
	if err != nil {
		t.Fatal(err)
	}
	adminToken, err := a.IssueAdminToken("admin-1", "a@example.com", "super_admin")
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer "+userToken)
	if id, err := a.UserID(r); err != nil || id != "user-1" {
		t.Fatalf("user token rejected by UserID: %v", err)
	}
	if _, err := a.Admin(r); err == nil {
		t.Fatal("user token accepted as admin token")
	}

	r.Header.Set("Authorization", "Bearer "+adminToken)
	if claims, err := a.Admin(r); err != nil || claims.Role != "super_admin" {
		t.Fatalf("admin token rejected by Admin: %v", err)
	}
	if _, err := a.UserID(r); err == nil {
		t.Fatal("admin token accepted as user token")
	}
}

func TestRequireChecksAccountStatus(t *testing.T) {
	a := New("test-secret")
	token, _ := a.IssueAccessToken("user-1", "u@example.com")
	call := func() int {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.Require(w, r)
		return w.Code
	}

	for name, tc := range map[string]struct {
		exists, active bool
		want           int
	}{
		"active":    {true, true, 200},
		"suspended": {true, false, 403},
		"deleted":   {false, false, 401},
	} {
		a.CheckAccountStatus(func(context.Context, string) (bool, bool, error) { return tc.exists, tc.active, nil })
		if got := call(); got != tc.want {
			t.Errorf("%s: status %d, want %d", name, got, tc.want)
		}
	}
}

func TestRejectsForeignSecretAndGarbage(t *testing.T) {
	token, _ := New("other-secret").IssueAccessToken("user-1", "u@example.com")
	a := New("test-secret")
	for _, value := range []string{"", "Bearer", "Bearer garbage", "Bearer " + token} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", value)
		if _, err := a.UserID(r); err == nil {
			t.Errorf("accepted Authorization %q", value)
		}
	}
}
