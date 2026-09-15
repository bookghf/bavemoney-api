package database

import "testing"

func TestUpdateBuilderNumbersPlaceholdersSequentially(t *testing.T) {
	update := NewUpdate("accounts").Set("type", "cash")

	query, args := update.Build("acc-1", "user-1")

	want := "UPDATE accounts SET updated_at = now(), type = $1 WHERE id = $2 AND user_id = $3"
	if query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
	if len(args) != 3 || args[0] != "cash" || args[1] != "acc-1" || args[2] != "user-1" {
		t.Errorf("args = %v, want [cash acc-1 user-1]", args)
	}
}

func TestUpdateBuilderEmpty(t *testing.T) {
	update := NewUpdate("accounts")
	if !update.Empty() {
		t.Fatal("Empty() = false on a builder with no columns")
	}

	query, args := update.Build("acc-1", "user-1")
	want := "UPDATE accounts SET updated_at = now() WHERE id = $1 AND user_id = $2"
	if query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
	if len(args) != 2 {
		t.Errorf("args = %v, want 2 values", args)
	}
}

func TestNullIfEmpty(t *testing.T) {
	if got := NullIfEmpty(""); got != nil {
		t.Errorf(`NullIfEmpty("") = %v, want nil`, got)
	}
	if got := NullIfEmpty("x"); got != "x" {
		t.Errorf(`NullIfEmpty("x") = %v, want "x"`, got)
	}
}
