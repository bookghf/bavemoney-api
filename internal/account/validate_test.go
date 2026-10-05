package account

import (
	"testing"

	"ledger-api/internal/validate"
)

func TestNormalizeCreate(t *testing.T) {
	ok := CreateRequest{Name: " Cash ", Type: "cash", Currency: "thb", InitialBalance: "1000.00"}
	if err := normalizeCreate(&ok); err != nil {
		t.Fatalf("valid account rejected: %v", err)
	}
	if ok.Name != "Cash" || ok.Currency != "THB" {
		t.Errorf("not normalized: %+v", ok)
	}

	card := CreateRequest{Name: "Visa", Type: "credit_card", Currency: "THB", InitialBalance: "-1500.50"}
	if err := normalizeCreate(&card); err != nil {
		t.Errorf("negative credit card balance rejected: %v", err)
	}

	for name, req := range map[string]CreateRequest{
		"blank name":            {Name: "   ", Type: "cash", Currency: "THB"},
		"unknown type":          {Name: "x", Type: "spaceship", Currency: "THB"},
		"bad currency":          {Name: "x", Type: "cash", Currency: "TH"},
		"three decimals":        {Name: "x", Type: "cash", Currency: "THB", InitialBalance: "0.001"},
		"exponent":              {Name: "x", Type: "cash", Currency: "THB", InitialBalance: "1e20"},
		"negative cash balance": {Name: "x", Type: "cash", Currency: "THB", InitialBalance: "-99999"},
	} {
		if err := normalizeCreate(&req); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestCheckUpdateRevalidatesBalanceAgainstNewType(t *testing.T) {
	card := Account{Type: "credit_card", InitialBalance: "-500.00"}
	cash := "cash"
	if err := checkUpdate(card, &UpdateRequest{Type: &cash}); err == nil {
		t.Error("turning a negative credit card into cash: want error")
	}
	balance := validate.Decimal("0")
	if err := checkUpdate(card, &UpdateRequest{Type: &cash, InitialBalance: &balance}); err != nil {
		t.Errorf("cash with zero balance: %v", err)
	}
}

func TestCheckReconcile(t *testing.T) {
	bank := Account{Type: "bank"}
	card := Account{Type: "credit_card"}
	for _, ok := range []struct {
		account Account
		balance validate.Decimal
	}{{bank, "1234.56"}, {bank, "0"}, {bank, "-0.00"}, {card, "-1500.50"}} {
		if err := checkReconcile(ok.account, ReconcileRequest{Balance: ok.balance}); err != nil {
			t.Errorf("%s %s rejected: %v", ok.account.Type, ok.balance, err)
		}
	}
	for _, balance := range []validate.Decimal{"", "1.001", "1e5", "-10", "abc"} {
		if err := checkReconcile(bank, ReconcileRequest{Balance: balance}); err == nil {
			t.Errorf("bank %q: want error", balance)
		}
	}
}

func TestColor(t *testing.T) {
	req := CreateRequest{Name: "x", Type: "cash", Currency: "THB", Color: " lime "}
	if err := normalizeCreate(&req); err != nil || req.Color != "lime" {
		t.Errorf("lime: color %q, err %v", req.Color, err)
	}
	bad := CreateRequest{Name: "x", Type: "cash", Currency: "THB", Color: "teal"}
	if err := normalizeCreate(&bad); err == nil {
		t.Error("teal: want error")
	}
	empty := ""
	if err := checkUpdate(Account{Type: "cash"}, &UpdateRequest{Color: &empty}); err != nil {
		t.Errorf("clearing the color: %v", err)
	}
}
