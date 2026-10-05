package transaction

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	accountA = "11111111-1111-4111-8111-111111111111"
	accountB = "22222222-2222-4222-8222-222222222222"
	category = "33333333-3333-4333-8333-333333333333"
)

func TestValidateCreate(t *testing.T) {
	base := CreateRequest{AccountID: accountA, Type: TypeExpense, Amount: "10.50", Currency: "THB", OccurredAt: "2026-09-25T00:00:00Z"}
	transfer := CreateRequest{AccountID: accountA, ToAccountID: accountB, Type: TypeTransfer, Amount: "100", OccurredAt: "2026-09-25T00:00:00Z"}

	tests := []struct {
		name    string
		edit    func(CreateRequest) CreateRequest
		from    CreateRequest
		wantErr bool
	}{
		{"expense", nil, base, false},
		{"transfer without currency", nil, transfer, false},
		{"expense without currency takes the account's", func(r CreateRequest) CreateRequest { r.Currency = ""; return r }, base, false},
		{"zero amount", func(r CreateRequest) CreateRequest { r.Amount = "0"; return r }, base, true},
		{"negative amount", func(r CreateRequest) CreateRequest { r.Amount = "-5"; return r }, base, true},
		{"three decimals", func(r CreateRequest) CreateRequest { r.Amount = "1.005"; return r }, base, true},
		{"too many digits", func(r CreateRequest) CreateRequest { r.Amount = "99999999999999999999"; return r }, base, true},
		{"lowercase currency is normalized", func(r CreateRequest) CreateRequest { r.Currency = "thb"; return r }, base, false},
		{"bad currency", func(r CreateRequest) CreateRequest { r.Currency = "TH"; return r }, base, true},
		{"impossible date", func(r CreateRequest) CreateRequest { r.OccurredAt = "2026-02-30T10:00:00Z"; return r }, base, true},
		{"not a date", func(r CreateRequest) CreateRequest { r.OccurredAt = "not-a-date"; return r }, base, true},
		{"account not a uuid", func(r CreateRequest) CreateRequest { r.AccountID = "a"; return r }, base, true},
		{"category not a uuid", func(r CreateRequest) CreateRequest { r.CategoryID = "nope"; return r }, base, true},
		{"note too long", func(r CreateRequest) CreateRequest { r.Note = strings.Repeat("x", 501); return r }, base, true},
		{"expense with to_account_id", func(r CreateRequest) CreateRequest { r.ToAccountID = accountB; return r }, base, true},
		{"unknown type", func(r CreateRequest) CreateRequest { r.Type = "refund"; return r }, base, true},
		{"transfer without target", func(r CreateRequest) CreateRequest { r.ToAccountID = ""; return r }, transfer, true},
		{"transfer to same account", func(r CreateRequest) CreateRequest { r.ToAccountID = accountA; return r }, transfer, true},
		{"transfer with category", func(r CreateRequest) CreateRequest { r.CategoryID = category; return r }, transfer, true},
	}
	for _, tt := range tests {
		req := tt.from
		if tt.edit != nil {
			req = tt.edit(req)
		}
		if err := validateCreate(&req); (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestValidateUpdateKeepsShapesValid(t *testing.T) {
	income, expense, transfer, cat, empty, a, b, notUUID := TypeIncome, TypeExpense, TypeTransfer, category, "", accountA, accountB, "nope"
	expenseTx := Transaction{Type: TypeExpense, AccountID: accountA, Category: &CategoryInfo{ID: category}}
	transferTx := Transaction{Type: TypeTransfer, AccountID: accountA, ToAccountID: accountB}

	tests := []struct {
		name     string
		existing Transaction
		req      UpdateRequest
		wantErr  bool
	}{
		{"expense -> income", expenseTx, UpdateRequest{Type: &income}, false},
		{"move to another account", expenseTx, UpdateRequest{AccountID: &b}, false},
		{"account not a uuid", expenseTx, UpdateRequest{AccountID: &notUUID}, true},
		{"expense -> transfer", expenseTx, UpdateRequest{Type: &transfer, ToAccountID: &b}, false},
		{"expense -> transfer drops its category", expenseTx, UpdateRequest{Type: &transfer, ToAccountID: &b, CategoryID: &empty}, false},
		{"expense -> transfer without a target", expenseTx, UpdateRequest{Type: &transfer}, true},
		{"expense -> transfer to itself", expenseTx, UpdateRequest{Type: &transfer, ToAccountID: &a}, true},
		{"expense -> transfer with a category", expenseTx, UpdateRequest{Type: &transfer, ToAccountID: &b, CategoryID: &cat}, true},
		{"to_account_id on an expense", expenseTx, UpdateRequest{ToAccountID: &b}, true},
		{"transfer -> income", transferTx, UpdateRequest{Type: &income}, false},
		{"transfer -> expense with a stale target", transferTx, UpdateRequest{Type: &expense, ToAccountID: &b}, true},
		{"transfer -> expense clearing the target", transferTx, UpdateRequest{Type: &expense, ToAccountID: &empty}, false},
		{"category on a transfer", transferTx, UpdateRequest{CategoryID: &cat}, true},
		{"transfer onto its own target", transferTx, UpdateRequest{AccountID: &b}, true},
		{"transfer swaps direction", transferTx, UpdateRequest{AccountID: &b, ToAccountID: &a}, false},
	}
	for _, tt := range tests {
		if err := validateUpdate(tt.existing, tt.req); (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}

	bad := "2026-02-30T10:00:00Z"
	if err := validateUpdate(expenseTx, UpdateRequest{OccurredAt: &bad}); err == nil {
		t.Error("impossible occurred_at: want error")
	}
	refund := "refund"
	if err := validateUpdate(expenseTx, UpdateRequest{Type: &refund}); err == nil {
		t.Error("unknown type: want error")
	}
}

func TestUpdateRequestEmptyTreatsClearedTagsAsAField(t *testing.T) {
	cleared := []string{}
	if (UpdateRequest{Tags: &cleared}).Empty() {
		t.Error(`{"tags": []} must count as an update that clears the tags`)
	}
	if !(UpdateRequest{}).Empty() {
		t.Error("no fields must be empty")
	}
}

func TestParseListFilterUsesTimeZoneDays(t *testing.T) {
	filter, err := parseListFilter(url.Values{"from": {"2027-01-01"}, "to": {"2027-01-31"}, "tz": {"Asia/Bangkok"}})
	if err != nil {
		t.Fatal(err)
	}
	// 2027-01-01 00:00 in Bangkok is 2026-12-31 17:00 UTC.
	if want := time.Date(2026, 12, 31, 17, 0, 0, 0, time.UTC); !filter.FromTime.Equal(want) {
		t.Errorf("FromTime = %v, want %v", filter.FromTime.UTC(), want)
	}
	// "to" is inclusive: the bound is the start of the next day.
	if want := time.Date(2027, 1, 31, 17, 0, 0, 0, time.UTC); !filter.ToTime.Equal(want) {
		t.Errorf("ToTime = %v, want %v", filter.ToTime.UTC(), want)
	}

	for _, bad := range []url.Values{
		{"tz": {"Mars/Base"}},
		{"from": {"2026-02-30"}},
		{"account": {"not-a-uuid"}},
		{"type": {"refund"}},
		{"page": {"9223372036854775807"}},
		{"tz": {"Local"}},
		{"from": {"2026-02-01"}, "to": {"2026-01-01"}},
		{"sort": {"-bogus"}},
	} {
		if _, err := parseListFilter(bad); err == nil {
			t.Errorf("parseListFilter(%v): want error", bad)
		}
	}
}

func TestParseListFilterAcceptsReportStyleNames(t *testing.T) {
	filter, err := parseListFilter(url.Values{"account_id": {accountA}, "category_id": {category}, "sort": {"-amount"}})
	if err != nil {
		t.Fatal(err)
	}
	if filter.AccountID != accountA || filter.CategoryID != category {
		t.Errorf("aliases not read: %+v", filter)
	}
}

func TestParseListFilterSearch(t *testing.T) {
	filter, err := parseListFilter(url.Values{"q": {"  lunch "}, "search_categories": {category + ", " + accountA}})
	if err != nil {
		t.Fatal(err)
	}
	if filter.Search != "lunch" || len(filter.SearchCategories) != 2 {
		t.Errorf("q alias or search_categories not read: %+v", filter)
	}

	// Category matches only widen a text search.
	filter, err = parseListFilter(url.Values{"search_categories": {category}})
	if err != nil {
		t.Fatal(err)
	}
	if filter.SearchCategories != nil {
		t.Errorf("search_categories without search = %v, want none", filter.SearchCategories)
	}

	for _, bad := range []url.Values{
		{"search": {"x"}, "search_categories": {"not-a-uuid"}},
		{"search": {strings.Repeat("ก", maxSearchLength+1)}},
	} {
		if _, err := parseListFilter(bad); err == nil {
			t.Errorf("parseListFilter(%v): want error", bad)
		}
	}
}

func TestAmountPrefix(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"257", "257", true},
		{"1,500", "1500", true},
		{"1500.5", "1500.5", true},
		{"12.", "12.", true},
		{"0257", "257", true},
		{"0.5", "0.5", true},
		{"0", "0", true},
		{"1.005", "", false},
		{"12a", "", false},
		{",5", "", false},
		{"-5", "", false},
		{"lunch", "", false},
	}
	for _, tt := range tests {
		got, ok := amountPrefix(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("amountPrefix(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}
