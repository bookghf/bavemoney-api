package account

import (
	"errors"
	"strings"

	"ledger-api/internal/validate"
)

// normalizeCreate trims and checks a POST /accounts payload in place.
func normalizeCreate(req *CreateRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	req.Color = strings.TrimSpace(req.Color)
	if req.InitialBalance == "" {
		req.InitialBalance = "0"
	}
	if err := checkName(req.Name); err != nil {
		return err
	}
	if !types[req.Type] {
		return errors.New("type must be cash, bank, credit_card, or e_wallet")
	}
	if !validate.Currency(req.Currency) {
		return errors.New("currency must be a 3-letter code such as THB")
	}
	if err := checkColor(req.Color); err != nil {
		return err
	}
	return checkBalance(string(req.InitialBalance), req.Type)
}

// checkUpdate validates a PATCH against the stored account.
func checkUpdate(existing Account, req *UpdateRequest) error {
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		req.Name = &trimmed
		if err := checkName(trimmed); err != nil {
			return err
		}
	}
	accountType := existing.Type
	if req.Type != nil {
		if !types[*req.Type] {
			return errors.New("type must be cash, bank, credit_card, or e_wallet")
		}
		accountType = *req.Type
	}
	balance := existing.InitialBalance
	if req.InitialBalance != nil {
		balance = string(*req.InitialBalance)
	}
	if req.InitialBalance != nil || req.Type != nil {
		if err := checkBalance(balance, accountType); err != nil {
			return err
		}
	}
	if req.Currency != nil {
		upper := strings.ToUpper(strings.TrimSpace(*req.Currency))
		req.Currency = &upper
		if !validate.Currency(upper) {
			return errors.New("currency must be a 3-letter code such as THB")
		}
	}
	if req.Color != nil {
		trimmed := strings.TrimSpace(*req.Color)
		req.Color = &trimmed
		if err := checkColor(trimmed); err != nil {
			return err
		}
	}
	return nil
}

// checkReconcile validates the balance the account holds today. It follows
// the opening-balance sign rule: only a credit card can be negative (owed).
// The opening balance worked back from it may still come out negative, see
// Repository.Reconcile.
func checkReconcile(existing Account, req ReconcileRequest) error {
	balance := string(req.Balance)
	if balance == "" {
		return errors.New("balance is required")
	}
	if !validate.SignedAmount(balance) {
		return errors.New("balance must be a decimal with at most 2 decimal places and 16 digits")
	}
	if isNegative(balance) && existing.Type != "credit_card" {
		return errors.New("only credit card accounts can have a negative balance")
	}
	return nil
}

// checkColor allows no color (the app picks one for the type) or a palette key.
func checkColor(color string) error {
	if color != "" && !colors[color] {
		return errors.New("color must be one of orange, amber, lime, cyan, indigo, violet, fuchsia, pink, brown, slate")
	}
	return nil
}

func checkName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	return validate.Name(name, maxNameLength, "name")
}

// checkBalance allows a negative opening balance only on credit cards, where
// it is the amount already owed.
func checkBalance(balance, accountType string) error {
	if !validate.SignedAmount(balance) {
		return errors.New("initial_balance must be a decimal with at most 2 decimal places and 16 digits")
	}
	if isNegative(balance) && accountType != "credit_card" {
		return errors.New("only credit card accounts can start with a negative balance")
	}
	return nil
}

// isNegative reports whether a validated decimal string is below zero ("-0"
// and "-0.00" are not).
func isNegative(balance string) bool {
	return strings.HasPrefix(balance, "-") && strings.Trim(balance, "-0.") != ""
}
