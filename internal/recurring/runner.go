package recurring

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/lib/pq"

	"ledger-api/internal/transaction"
	"ledger-api/internal/validate"
)

// RunInterval is how often the API looks for due rules.
const RunInterval = 15 * time.Minute

// maxRunsPerPass bounds the transactions one rule creates in one database
// transaction; a rule further behind is picked up again in the same pass.
const maxRunsPerPass = 100

// TagPrefix starts the tag on every transaction a rule creates
// ("recurring:<rule id>"), so they can be traced back and filtered.
const TagPrefix = "recurring:"

// Run creates due transactions now and then every interval until ctx is
// cancelled.
func (r *Repository) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if created, err := r.RunDue(ctx, time.Now(), ""); err != nil && ctx.Err() == nil {
			slog.Error("recurring run", "error", err)
		} else if created > 0 {
			slog.Info("recurring run", "created", created)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunDue creates the transactions of every rule due as of now (each rule's
// "today" is the local day in its time zone), catching up on missed periods,
// and returns how many it created. userID limits the run to one user; ""
// runs every user's rules.
//
// Each rule is handled in its own database transaction that locks the rule
// row (FOR UPDATE SKIP LOCKED), inserts its transactions, and advances
// next_run_on before committing. Two API instances therefore never create the
// same occurrence, and a crash leaves no half-applied rule.
func (r *Repository) RunDue(ctx context.Context, now time.Time, userID string) (int, error) {
	// skip holds rules this pass must not pick again: ones that failed, or
	// are not due yet in their own time zone.
	skip := pq.StringArray{}
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		id, created, err := r.runNext(ctx, now, userID, skip)
		switch {
		case id == "" && err != nil:
			return total, err
		case id == "":
			return total, nil
		case err != nil:
			slog.Error("recurring rule", "rule_id", id, "error", err)
			skip = append(skip, id)
		case created < 0:
			skip = append(skip, id)
		default:
			total += created
		}
	}
}

// runNext locks one possibly due rule and brings it up to date. It returns
// the rule's id ("" when none is left) and the number of transactions created,
// or -1 when the rule turned out not to be due yet.
func (r *Repository) runNext(ctx context.Context, now time.Time, userID string, skip pq.StringArray) (string, int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// Local days are at most a day ahead of UTC, so this finds every rule
	// due somewhere; whether it is due in its own time zone is checked below.
	var (
		rule      CreateRequest
		id, owner string
		next      time.Time
		endDate   sql.NullTime
	)
	err = tx.QueryRowContext(ctx, `
		SELECT r.id, r.user_id, r.type, r.account_id, COALESCE(r.to_account_id::text, ''),
			COALESCE(r.category_id::text, ''), r.amount::text, r.currency, COALESCE(r.note, ''),
			r.frequency, r.day_of_month, r.weekday, r.next_run_on, r.end_date, r.time_zone
		FROM recurring_rules r
		WHERE r.is_active
		  AND r.next_run_on <= $1::date + 1
		  AND (r.end_date IS NULL OR r.next_run_on <= r.end_date)
		  AND ($2 = '' OR r.user_id::text = $2)
		  AND NOT (r.id = ANY($3::uuid[]))
		  AND EXISTS (SELECT 1 FROM users u WHERE u.id = r.user_id AND u.status = 'active')
		ORDER BY r.next_run_on
		LIMIT 1
		FOR UPDATE OF r SKIP LOCKED
	`, now.UTC().Format(time.DateOnly), userID, skip).Scan(&id, &owner, &rule.Type, &rule.AccountID, &rule.ToAccountID,
		&rule.CategoryID, &rule.Amount, &rule.Currency, &rule.Note,
		&rule.Frequency, &rule.DayOfMonth, &rule.Weekday, &next, &endDate, &rule.TimeZone)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, err
	}

	loc, ok := validate.Location(rule.TimeZone)
	if !ok {
		return id, 0, errors.New("unknown time zone " + rule.TimeZone)
	}
	today := dateOf(now.In(loc))
	next = dateOf(next)
	if next.After(today) {
		return id, -1, nil
	}

	sched := scheduleOf(rule)
	var (
		created     int
		lastRun     *time.Time
		pauseReason string
	)
	for !next.After(today) && (!endDate.Valid || !next.After(dateOf(endDate.Time))) && created < maxRunsPerPass {
		// The transaction is dated at the start of the due day, local time.
		occurredAt := time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, loc).Format(time.RFC3339)
		req := transactionRequest(rule, occurredAt)
		req.Tags = []string{TagPrefix + id}
		if _, err := transaction.CreateInTx(ctx, tx, owner, req); err != nil {
			if !transaction.IsClientError(err) {
				return id, 0, err
			}
			// The rule can no longer be applied (its account was archived,
			// its currency changed...): pause it and say why.
			pauseReason = ReasonInvalid
			if errors.Is(err, transaction.ErrAccountArchived) {
				pauseReason = ReasonAccountArchived
			}
			slog.Info("recurring rule paused", "rule_id", id, "reason", err.Error())
			break
		}
		run := next
		lastRun = &run
		next = sched.after(next)
		created++
	}

	var lastRunOn interface{}
	if lastRun != nil {
		lastRunOn = lastRun.Format(time.DateOnly)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE recurring_rules SET next_run_on = $2, last_run_on = COALESCE($3::date, last_run_on),
			is_active = ($4::text = ''), pause_reason = NULLIF($4::text, ''), updated_at = now()
		WHERE id = $1
	`, id, next.Format(time.DateOnly), lastRunOn, pauseReason); err != nil {
		return id, 0, err
	}
	return id, created, tx.Commit()
}

// scheduleOf reads the schedule of a validated rule.
func scheduleOf(req CreateRequest) schedule {
	s := schedule{frequency: req.Frequency}
	if req.DayOfMonth != nil {
		s.dayOfMonth = *req.DayOfMonth
	}
	if req.Weekday != nil {
		s.weekday = *req.Weekday
	}
	return s
}
