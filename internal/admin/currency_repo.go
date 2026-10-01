package admin

import (
	"context"
	"database/sql"
	"fmt"
)

type CurrencyRepository struct {
	db *sql.DB
}

func NewCurrencyRepository(db *sql.DB) *CurrencyRepository {
	return &CurrencyRepository{db: db}
}

func (r *CurrencyRepository) List(ctx context.Context, isActive *bool) ([]CurrencyInfo, error) {
	query := `SELECT code, name, symbol, is_active FROM currencies`
	args := []interface{}{}

	if isActive != nil {
		query += ` WHERE is_active = $1`
		args = append(args, *isActive)
	}

	query += ` ORDER BY code`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var currencies []CurrencyInfo
	for rows.Next() {
		var c CurrencyInfo
		if err := rows.Scan(&c.Code, &c.Name, &c.Symbol, &c.IsActive); err != nil {
			return nil, err
		}
		currencies = append(currencies, c)
	}
	return currencies, nil
}

func (r *CurrencyRepository) Create(ctx context.Context, req CreateCurrencyRequest) (CurrencyInfo, error) {
	query := `INSERT INTO currencies (code, name, symbol, is_active) VALUES ($1, $2, $3, $4)
	          RETURNING code, name, symbol, is_active`
	var c CurrencyInfo
	err := r.db.QueryRowContext(ctx, query, req.Code, req.Name, req.Symbol, req.IsActive).
		Scan(&c.Code, &c.Name, &c.Symbol, &c.IsActive)
	return c, err
}

// Update toggles a currency, returning sql.ErrNoRows for an unknown code.
func (r *CurrencyRepository) Update(ctx context.Context, code string, req UpdateCurrencyRequest) error {
	result, err := r.db.ExecContext(ctx, `UPDATE currencies SET is_active = $1 WHERE code = $2`, req.IsActive, code)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *CurrencyRepository) ListExchangeRates(ctx context.Context, base, target string, from, to string, page, limit int) ([]ExchangeRate, int, error) {
	where := " WHERE 1=1"
	args := []interface{}{}
	add := func(condition string, value interface{}) {
		args = append(args, value)
		where += fmt.Sprintf(condition, len(args))
	}
	if base != "" {
		add(" AND base_currency = $%d", base)
	}
	if target != "" {
		add(" AND target_currency = $%d", target)
	}
	if from != "" {
		add(" AND effective_date >= $%d::date", from)
	}
	if to != "" {
		add(" AND effective_date <= $%d::date", to)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM exchange_rates`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, (page-1)*limit)
	query := fmt.Sprintf(`SELECT id, base_currency, target_currency, rate::text, effective_date::text
		FROM exchange_rates%s ORDER BY effective_date DESC, base_currency, target_currency
		LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	rates := []ExchangeRate{}
	for rows.Next() {
		var r ExchangeRate
		if err := rows.Scan(&r.ID, &r.BaseCurrency, &r.TargetCurrency, &r.Rate, &r.EffectiveDate); err != nil {
			return nil, 0, err
		}
		rates = append(rates, r)
	}
	return rates, total, rows.Err()
}

func (r *CurrencyRepository) CreateExchangeRate(ctx context.Context, req CreateExchangeRateRequest) (ExchangeRate, error) {
	query := `INSERT INTO exchange_rates (base_currency, target_currency, rate, effective_date)
	          VALUES ($1, $2, $3, $4)
	          ON CONFLICT (base_currency, target_currency, effective_date) DO UPDATE SET rate = EXCLUDED.rate
	          RETURNING id, base_currency, target_currency, rate::text, effective_date::text`
	var rate ExchangeRate
	err := r.db.QueryRowContext(ctx, query, req.BaseCurrency, req.TargetCurrency, req.Rate, req.EffectiveDate).
		Scan(&rate.ID, &rate.BaseCurrency, &rate.TargetCurrency, &rate.Rate, &rate.EffectiveDate)
	return rate, err
}
