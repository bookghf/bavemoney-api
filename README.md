# Ledger API

This workspace now contains a starter Docker + PostgreSQL + Go API setup based on the system design document.

## Run with Docker Compose

```bash
docker compose up --build
```

Services:
- PostgreSQL: http://localhost:5432
- API: http://localhost:8080

## Health check

```bash
curl http://localhost:8080/health
```

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
  "token": "<access token>",
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
```

Or start from scratch with `docker compose down -v && docker compose up --build`.

## Tests

```bash
go test ./...
```
