# Ledger App — System Design Document

## 1. Requirements Summary

**Product**: Personal finance ledger — track income, receipts, and spending.

**Confirmed scope:**
- Single-user personal finance (not multi-tenant business/team)
- Categories & tags for transactions
- Budgets & spending limits
- Reports/analytics & charts
- Receipt/attachment uploads (photo of receipt)
- Multi-currency support
- Login: email/password + Google social login
- Online-only mobile app (requires an internet connection; no offline recording/sync)
- Push notifications (recommended below)

**Clients:**
| Client | Stack | Users |
|---|---|---|
| Mobile app | React Native | End users |
| Admin backoffice | Next.js | Admins/support staff |
| Backend API | Go | Serves both clients |
| Database | PostgreSQL | Single source of truth |

**Admin backoffice scope:**
- User accounts: view, suspend, support
- Master data: categories, currencies & exchange rates
- Global analytics: total users, activity, growth

---

## 2. Recommendation: Push Notifications

Recommended — **yes**, scoped narrowly at first:
- Budget threshold alerts (e.g. "80% of Groceries budget used")
- Recurring bill/income reminders (phase 2, if you add recurring transactions later)
- Sync conflict alerts (rare, but useful — see §6)

Use **Firebase Cloud Messaging (FCM)** — free, works for both iOS and Android from one React Native integration, and the Go backend just calls FCM's HTTP API. No need to build your own push infra.

---

## 3. System Architecture

*(See diagram above.)* Both the mobile app and admin backoffice talk to the same Go REST API — there is one source of truth, one auth system, one business-logic layer. The API connects to:
- **PostgreSQL** — all structured data (users, transactions, budgets, categories, exchange rates)
- **Object storage** (S3-compatible — AWS S3, Cloudflare R2, or MinIO if self-hosting) — receipt photos
- **FCM** — push notifications

---

## 4. Database Schema (PostgreSQL)

```sql
-- Users
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT,               -- null if user only uses Google login
    google_id TEXT UNIQUE,
    display_name TEXT,
    avatar_url TEXT,
    default_currency CHAR(3) NOT NULL DEFAULT 'USD',
    status TEXT NOT NULL DEFAULT 'active', -- active, suspended
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Admin users (separate table — admins are not app users)
CREATE TABLE admin_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'support', -- support, superadmin
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Categories (system defaults + user-created custom ones)
-- Subcategories reference a parent category via parent_id (2 levels: category -> subcategory)
CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE, -- NULL = global/system category
    parent_id UUID REFERENCES categories(id) ON DELETE CASCADE, -- NULL = top-level category
    name TEXT NOT NULL,
    type TEXT NOT NULL, -- income, expense
    icon TEXT,
    color TEXT,
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_no_self_parent CHECK (id IS DISTINCT FROM parent_id)
);
CREATE INDEX idx_categories_parent ON categories(parent_id);

-- Currencies & exchange rates (managed by admin)
CREATE TABLE currencies (
    code CHAR(3) PRIMARY KEY,   -- USD, EUR, THB...
    name TEXT NOT NULL,
    symbol TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE exchange_rates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    base_currency CHAR(3) NOT NULL REFERENCES currencies(code),
    target_currency CHAR(3) NOT NULL REFERENCES currencies(code),
    rate NUMERIC(20,8) NOT NULL,
    effective_date DATE NOT NULL,
    UNIQUE (base_currency, target_currency, effective_date)
);

-- Accounts (wallet, bank, cash, credit card — a ledger can have multiple)
CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL, -- cash, bank, credit_card, e_wallet
    currency CHAR(3) NOT NULL REFERENCES currencies(code),
    initial_balance NUMERIC(18,2) NOT NULL DEFAULT 0,
    is_archived BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Transactions (the core ledger entries: income / expense / transfer)
CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id),
    category_id UUID REFERENCES categories(id),
    type TEXT NOT NULL, -- income, expense, transfer
    amount NUMERIC(18,2) NOT NULL,
    currency CHAR(3) NOT NULL REFERENCES currencies(code),
    amount_in_default_currency NUMERIC(18,2), -- pre-converted for reporting
    note TEXT,
    tags TEXT[],
    occurred_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ,            -- soft delete, keeps history recoverable
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_transactions_user_date ON transactions(user_id, occurred_at);

-- Receipt attachments
CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    file_url TEXT NOT NULL,
    file_size_bytes INT,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Budgets
CREATE TABLE budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID REFERENCES categories(id), -- NULL = overall budget
    amount NUMERIC(18,2) NOT NULL,
    currency CHAR(3) NOT NULL REFERENCES currencies(code),
    period TEXT NOT NULL, -- weekly, monthly, yearly
    start_date DATE NOT NULL,
    alert_threshold_pct INT NOT NULL DEFAULT 80,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Push notification device tokens
CREATE TABLE device_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    fcm_token TEXT NOT NULL,
    platform TEXT NOT NULL, -- ios, android
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Admin audit log
CREATE TABLE admin_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id UUID NOT NULL REFERENCES admin_users(id),
    action TEXT NOT NULL,
    target_table TEXT,
    target_id UUID,
    detail JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**Design notes:**
- `categories.parent_id` makes a category a subcategory of another (e.g. "Food" → "Groceries", "Food" → "Dining out"). `transactions.category_id` can point to either a top-level category or a subcategory — reports roll subcategory spend up into its parent for the summary view, and can also break it out.
- Keep this to **2 levels only** (category → subcategory, no sub-subcategories) — enough for real-world use and simple to query. Postgres can't cleanly enforce "no more than 2 levels" with a check constraint, so enforce it in the Go API when creating/updating a category (reject if `parent_id` refers to a category that itself already has a `parent_id`).
- `deleted_at` is a soft delete rather than a hard `DELETE` — keeps a recoverable history and makes admin support/audit easier.
- `amount_in_default_currency` is pre-computed at write time using the exchange rate on that date, so reports don't need to re-convert historical data every time rates change.

---

## 5. API Design (Go REST API)

Base path: `/api/v1`

**Auth**
```
POST   /auth/register
POST   /auth/login
POST   /auth/google
POST   /auth/refresh
POST   /auth/logout
```

**Ledger (mobile app)**
```
GET    /accounts
POST   /accounts
GET    /transactions?page=&limit=&category=&account=&from=&to=
POST   /transactions
PATCH  /transactions/:id
DELETE /transactions/:id
GET    /categories                           # returns tree: top-level categories with nested subcategories
GET    /budgets
POST   /budgets
GET    /reports/summary?period=month&date=2026-08
POST   /transactions/:id/attachment          # presigned upload URL flow
POST   /device-tokens                        # register for push
```

**Admin (Next.js backoffice)**
```
POST   /admin/auth/login
GET    /admin/users?status=&search=
PATCH  /admin/users/:id/suspend
GET    /admin/categories
POST   /admin/categories
GET    /admin/currencies
POST   /admin/exchange-rates
GET    /admin/analytics/overview             # total users, active users, growth
GET    /admin/analytics/activity
```

**Auth mechanism**: JWT access token (short-lived, ~15 min) + refresh token (long-lived, stored securely on device). Admin backoffice uses a separate JWT scope so an admin token can never call end-user endpoints and vice versa.

---

## 6. Data Flow (Online-Only)

The app is online-only, so the mobile client talks directly to the API for every action — no local database, outbox queue, or sync/conflict logic to build:

1. User records a transaction → `POST /transactions` is called immediately.
2. If the request fails (no connection, server error), show a clear inline error and let the user retry — don't silently queue it.
3. Lists and reports are fetched live from the API (`GET /transactions`, `GET /reports/summary`) with normal pagination and pull-to-refresh, not a local cache that needs reconciling.
4. Use a lightweight in-memory/query cache on the client (e.g. **TanStack Query** / React Query) purely for snappy UI (avoiding refetches on every screen focus) — this is a performance nicety, not a data-integrity mechanism, so it needs no conflict resolution.

This removes one of the highest-risk, highest-effort parts of the original design (local DB, outbox pattern, conflict handling) — worth keeping in mind if you ever revisit offline support later, since it would touch the schema, API, and mobile data layer all at once.

---

## 7. Non-Functional Requirements

| Area | Recommendation |
|---|---|
| Security | Passwords hashed with bcrypt/argon2; JWT with short expiry + refresh rotation; HTTPS only; rate-limit auth endpoints |
| Scalability | Stateless Go API behind a load balancer; Postgres read replica once reporting queries grow heavy |
| Backups | Daily automated Postgres backups; point-in-time recovery enabled |
| Observability | Structured logging (e.g. `zerolog`), request tracing, error alerting (Sentry) |
| Currency data | Pull daily exchange rates from a provider (e.g. exchangerate.host, Open Exchange Rates) via a scheduled Go job; admin can override manually |

---

## 8. Suggested Build Phases

1. **Phase 1 — Core ledger**: Auth, accounts, transactions, categories, basic reports.
2. **Phase 2 — Budgets & alerts**: Budget CRUD, threshold checks, push notifications.
3. **Phase 3 — Multi-currency polish**: Exchange rate job, admin currency management, converted reporting.
4. **Phase 4 — Admin backoffice**: User management, master data, analytics dashboard.
5. **Phase 5 — Receipts**: Presigned upload flow, image compression on-device, attachment gallery.
