package admin

import (
	"context"
	"database/sql"
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

func (r *CurrencyRepository) Update(ctx context.Context, code string, req UpdateCurrencyRequest) error {
	query := `UPDATE currencies SET is_active = $1 WHERE code = $2`
	_, err := r.db.ExecContext(ctx, query, req.IsActive, code)
	return err
}

func (r *CurrencyRepository) ListExchangeRates(ctx context.Context, base, target string, from, to string, page, limit int) ([]ExchangeRate, int, error) {
	query := `SELECT id, base_currency, target_currency, rate, effective_date FROM exchange_rates WHERE 1=1`
	args := []interface{}{}
	idx := 1

	if base != "" {
		query += ` AND base_currency = $` + string(rune(idx))
		args = append(args, base)
		idx++
	}
	if target != "" {
		query += ` AND target_currency = $` + string(rune(idx))
		args = append(args, target)
		idx++
	}
	if from != "" {
		query += ` AND effective_date >= $` + string(rune(idx))
		args = append(args, from)
		idx++
	}
	if to != "" {
		query += ` AND effective_date <= $` + string(rune(idx))
		args = append(args, to)
		idx++
	}

	countQuery := query
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil && err != sql.ErrNoRows {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	query += ` ORDER BY effective_date DESC LIMIT $` + string(rune(idx)) + ` OFFSET $` + string(rune(idx+1))
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var rates []ExchangeRate
	for rows.Next() {
		var r ExchangeRate
		if err := rows.Scan(&r.ID, &r.BaseCurrency, &r.TargetCurrency, &r.Rate, &r.EffectiveDate); err != nil {
			return nil, 0, err
		}
		rates = append(rates, r)
	}
	return rates, total, nil
}

func (r *CurrencyRepository) CreateExchangeRate(ctx context.Context, req CreateExchangeRateRequest) (ExchangeRate, error) {
	query := `INSERT INTO exchange_rates (base_currency, target_currency, rate, effective_date)
	          VALUES ($1, $2, $3, $4)
	          ON CONFLICT (base_currency, target_currency, effective_date) DO UPDATE SET rate = EXCLUDED.rate
	          RETURNING id, base_currency, target_currency, rate, effective_date`
	var rate ExchangeRate
	err := r.db.QueryRowContext(ctx, query, req.BaseCurrency, req.TargetCurrency, req.Rate, req.EffectiveDate).
		Scan(&rate.ID, &rate.BaseCurrency, &rate.TargetCurrency, &rate.Rate, &rate.EffectiveDate)
	return rate, err
}
