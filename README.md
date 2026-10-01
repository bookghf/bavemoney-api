# Ledger API

This workspace now contains a starter Docker + PostgreSQL + Go API setup based on the system design document.

## Run with Docker Compose

Every credential lives in a git-ignored `.env` next to `docker-compose.yml`;
none has a default in the code. Copy the template and fill in the empty values:

```bash
cp .env.example .env
openssl rand -hex 24   # paste as POSTGRES_PASSWORD (and into DATABASE_URL)
openssl rand -hex 32   # paste as JWT_SECRET
docker compose up --build
```

`docker compose`, `go run ./cmd/api`, and the integration tests all read the
same `.env`. `.dockerignore` keeps it out of the image.

Services:
- PostgreSQL: localhost:5432 (bound to 127.0.0.1 only)
- API: http://localhost:8080

`APP_ENV=production` makes the API refuse to start with a short `JWT_SECRET`
or database password, or with `CORS_ALLOWED_ORIGINS=*`.

The Postgres password only takes effect when the volume is first created. To
change it later, run `ALTER USER ledger WITH PASSWORD '…'` in the database and
update `.env`.

## Database migrations

`db/init` only runs when the Postgres volume is first created. Later schema
changes live in `internal/migrate/sql/` and are applied automatically, once
each, when the API starts (tracked in `schema_migrations`). Keep them
idempotent so they also apply on a freshly bootstrapped database.

## Tests

```bash
go test ./...                                   # unit tests
docker compose up -d db
go test -tags integration ./internal/integration/   # end-to-end; reads DATABASE_URL from .env
```

The integration suite creates throwaway `qa-it+…@example.com` users and
deletes them afterwards.

## Health check

```bash
curl http://localhost:8080/health   # "ok" only when the database answers
```

## Security notes

- Access tokens last 15 minutes; clients renew them with the refresh token.
  Replaying an already-rotated refresh token revokes that whole login.
- Admin endpoints (`/api/v1/admin/*`) need an admin token from
  `POST /api/v1/admin/auth/login`; user tokens are rejected. The `support` role
  is read-only; `admin` and `super_admin` can change data, and every change is
  written to `admin_audit_log`.
- Login, register, and admin login are rate limited per client IP.

## API examples

### Register
```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"secret123","display_name":"Demo User"}'
```

### Login
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"secret123"}'
```

### List accounts
```bash
curl -H 'Authorization: Bearer <token>' http://localhost:8080/api/v1/accounts
```

### Transfer between accounts
Both accounts must belong to the user and share a currency; `currency` is
taken from the accounts, and transfers can not have a category.
```bash
curl -X POST http://localhost:8080/api/v1/transactions \
  -H 'Authorization: Bearer <token>' -H 'Content-Type: application/json' \
  -d '{"type":"transfer","account_id":"<from>","to_account_id":"<to>","amount":"300.00","occurred_at":"2026-09-25T10:00:00Z"}'
```

### Refresh
```bash
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"<refresh_token>"}'
```

### Logout
```bash
# revoke this device's session
curl -X POST http://localhost:8080/api/v1/auth/logout \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"<refresh_token>"}'

# revoke every session for the user
curl -X POST http://localhost:8080/api/v1/auth/logout \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"<refresh_token>","all":true}'
```

## Tokens

`register`, `login`, and `refresh` all return the same envelope:

```json
{
  "access_token": "<access token>",
  "expires_in": 86400,
  "refresh_token": "<refresh token>",
  "refresh_token_expires_in": 2592000,
  "user": { "...": "..." }
}
```

- **Access token** — JWT, valid 1 day, sent as `Authorization: Bearer <token>`. Stateless, so it cannot be revoked before it expires.
- **Refresh token** — opaque random string, valid 30 days, stored server-side as a SHA-256 hash in `refresh_tokens`. Rotated on every refresh: the presented token is consumed, and replaying it returns `401`. Logout revokes it, so the next refresh fails.

## Project layout

Follows the [golang-standards/project-layout](https://github.com/golang-standards/project-layout) convention:

```
cmd/api/                 entrypoint: load config, open DB, serve
internal/config/         environment configuration
internal/database/       connection pool + shared SQL helpers
internal/httpx/          JSON responses, request decoding, path parsing
internal/auth/           JWT issuing/verification, per-request user resolution
internal/server/         route registration
internal/user/           register, login, refresh, logout
internal/account/        accounts CRUD
internal/category/       category tree CRUD
internal/budget/         budgets CRUD
internal/transaction/    transactions CRUD
db/init/                 schema migrations applied on first boot only
```

Each domain package holds three files: the models (`<domain>.go`), the SQL
(`repository.go`), and the HTTP layer (`handler.go`).

## Migrations

`db/init` is a Postgres entrypoint directory: the scripts run **only when the
`pgdata` volume is empty**. To apply a new script to a database that already
exists, run it by hand:

```bash
docker exec -i ledger-db psql -U ledger -d ledger -v ON_ERROR_STOP=1 < db/init/002_refresh_tokens.sql
docker exec -i ledger-db psql -U ledger -d ledger -v ON_ERROR_STOP=1 < db/init/003_system_categories.sql
docker exec -i ledger-db psql -U ledger -d ledger -v ON_ERROR_STOP=1 < db/init/004_updated_at.sql
docker exec -i ledger-db psql -U ledger -d ledger -v ON_ERROR_STOP=1 < db/init/005_transfers.sql
```

`003_system_categories.sql` seeds the default categories (Salary, Food >
Groceries, Transport > Fuel, ...) that every user sees. It is safe to re-run.

Or start from scratch with `docker compose down -v && docker compose up --build`.

## Tests

```bash
go test ./...
```
