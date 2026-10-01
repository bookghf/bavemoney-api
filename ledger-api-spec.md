# Ledger App — API Specification

Base URL: `/api/v1`

All endpoints return JSON. Dates use ISO 8601 (`2026-08-15T10:30:00Z`). Monetary amounts are decimal strings with 2 decimal places. UUIDs are v4.

**Common error response:**

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable description",
    "details": {}
  }
}
```

**Common HTTP status codes:**

| Code | Meaning |
|------|---------|
| 200 | Success |
| 201 | Created |
| 400 | Validation error / bad request |
| 401 | Missing or invalid token |
| 403 | Forbidden (wrong role, suspended account) |
| 404 | Resource not found |
| 409 | Conflict (duplicate email, etc.) |
| 429 | Rate limited |
| 500 | Internal server error |

**Auth header (all protected endpoints):**

```
Authorization: Bearer <access_token>
```

---

## Auth

### POST /auth/register

Create a new user account with email and password.

**Request body:**

```json
{
  "email": "user@example.com",
  "password": "<password>",
  "display_name": "Jane Doe",
  "default_currency": "USD"
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| email | string | yes | Must be valid email, unique |
| password | string | yes | Min 8 chars, at least 1 uppercase, 1 number, 1 special char |
| display_name | string | no | Max 100 chars |
| default_currency | string | no | ISO 4217 code. Defaults to `USD` if omitted |

**Response: `201 Created`**

```json
{
  "user": {
    "id": "a1b2c3d4-...",
    "email": "user@example.com",
    "display_name": "Jane Doe",
    "default_currency": "USD",
    "status": "active",
    "created_at": "2026-08-15T10:00:00Z"
  },
  "access_token": "eyJhbG...",
  "refresh_token": "dGhpcyBpc..."
}
```

**Errors:** `409` if email already exists. `400` if password too weak.

---

### POST /auth/login

Authenticate with email and password.

**Request body:**

```json
{
  "email": "user@example.com",
  "password": "<password>"
}
```

| Field | Type | Required |
|-------|------|----------|
| email | string | yes |
| password | string | yes |

**Response: `200 OK`**

```json
{
  "user": {
    "id": "a1b2c3d4-...",
    "email": "user@example.com",
    "display_name": "Jane Doe",
    "default_currency": "USD",
    "status": "active",
    "created_at": "2026-08-15T10:00:00Z"
  },
  "access_token": "eyJhbG...",
  "refresh_token": "dGhpcyBpc..."
}
```

**Errors:** `401` if credentials are invalid. `403` if account is suspended.

---

### POST /auth/google

Authenticate or register via Google OAuth. The mobile app obtains a Google ID token via the Google Sign-In SDK and sends it here.

**Request body:**

```json
{
  "id_token": "eyJhbGciOi...",
  "default_currency": "THB"
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| id_token | string | yes | Google-issued ID token |
| default_currency | string | no | Used only on first-time registration. Defaults to `USD` |

**Response: `200 OK`** (existing user) or **`201 Created`** (new user)

```json
{
  "user": {
    "id": "a1b2c3d4-...",
    "email": "jane@gmail.com",
    "display_name": "Jane Doe",
    "avatar_url": "https://lh3.googleusercontent.com/...",
    "default_currency": "THB",
    "status": "active",
    "created_at": "2026-08-15T10:00:00Z"
  },
  "access_token": "eyJhbG...",
  "refresh_token": "dGhpcyBpc...",
  "is_new_user": true
}
```

**Errors:** `401` if Google token verification fails. `403` if account is suspended.

---

### POST /auth/refresh

Exchange a valid refresh token for a new access/refresh token pair. Old refresh token is invalidated (rotation).

**Request body:**

```json
{
  "refresh_token": "dGhpcyBpc..."
}
```

**Response: `200 OK`**

```json
{
  "access_token": "eyJhbG...",
  "refresh_token": "bmV3IHJlZnJl..."
}
```

**Errors:** `401` if refresh token is expired, revoked, or invalid.

---

### POST /auth/logout

Revoke the current refresh token. Access token remains valid until it expires naturally (~15 min).

**Request header:** `Authorization: Bearer <access_token>`

**Request body:**

```json
{
  "refresh_token": "dGhpcyBpc..."
}
```

**Response: `200 OK`**

```json
{
  "message": "Logged out successfully"
}
```

---

## Accounts

### GET /accounts

List all accounts for the authenticated user.

**Query parameters:**

| Param | Type | Default | Notes |
|-------|------|---------|-------|
| include_archived | boolean | false | If `true`, also returns archived accounts |

**Response: `200 OK`**

```json
{
  "accounts": [
    {
      "id": "b2c3d4e5-...",
      "name": "Main Wallet",
      "type": "e_wallet",
      "currency": "THB",
      "initial_balance": "0.00",
      "current_balance": "15420.50",
      "is_archived": false,
      "created_at": "2026-08-01T08:00:00Z"
    },
    {
      "id": "c3d4e5f6-...",
      "name": "Kasikorn Savings",
      "type": "bank",
      "currency": "THB",
      "initial_balance": "50000.00",
      "current_balance": "42350.00",
      "is_archived": false,
      "created_at": "2026-08-01T08:05:00Z"
    }
  ]
}
```

`current_balance` is computed: `initial_balance + SUM(income) - SUM(expense)` for that account.

---

### POST /accounts

Create a new account.

**Request body:**

```json
{
  "name": "Cash",
  "type": "cash",
  "currency": "THB",
  "initial_balance": "1000.00"
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| name | string | yes | Max 100 chars |
| type | string | yes | One of: `cash`, `bank`, `credit_card`, `e_wallet` |
| currency | string | yes | ISO 4217 code, must exist in `currencies` table and be active |
| initial_balance | string | no | Decimal string. Defaults to `"0.00"` |

**Response: `201 Created`**

```json
{
  "account": {
    "id": "d4e5f6a7-...",
    "name": "Cash",
    "type": "cash",
    "currency": "THB",
    "initial_balance": "1000.00",
    "current_balance": "1000.00",
    "is_archived": false,
    "created_at": "2026-08-15T10:30:00Z"
  }
}
```

---

### PATCH /accounts/:id

Update an account's name, type, or archive status.

**Request body (all fields optional):**

```json
{
  "name": "Petty Cash",
  "is_archived": true
}
```

| Field | Type | Notes |
|-------|------|-------|
| name | string | Max 100 chars |
| type | string | `cash`, `bank`, `credit_card`, `e_wallet` |
| is_archived | boolean | Archiving hides the account from the default list but preserves history |

**Response: `200 OK`** — returns the updated account object (same shape as POST response).

**Errors:** `404` if account doesn't belong to user.

---

## Transactions

### GET /transactions

List transactions for the authenticated user with filtering and pagination.

**Query parameters:**

| Param | Type | Default | Notes |
|-------|------|---------|-------|
| page | int | 1 | Page number (1-indexed) |
| limit | int | 20 | Items per page. Max 100 |
| account | uuid | — | Filter by account ID |
| category | uuid | — | Filter by category ID (includes subcategory transactions if a parent is given) |
| type | string | — | `income`, `expense`, or `transfer` |
| tags | string | — | Comma-separated. Returns transactions matching *any* of the given tags |
| from | date | — | Inclusive start date (`YYYY-MM-DD`) |
| to | date | — | Inclusive end date (`YYYY-MM-DD`) |
| search | string | — | Full-text search on `note` field |
| sort | string | `-occurred_at` | Prefix `-` for descending. Allowed fields: `occurred_at`, `amount`, `created_at` |

**Response: `200 OK`**

```json
{
  "transactions": [
    {
      "id": "e5f6a7b8-...",
      "account_id": "b2c3d4e5-...",
      "account_name": "Main Wallet",
      "category": {
        "id": "f6a7b8c9-...",
        "name": "Groceries",
        "parent": {
          "id": "a7b8c9d0-...",
          "name": "Food"
        }
      },
      "type": "expense",
      "amount": "350.00",
      "currency": "THB",
      "amount_in_default_currency": "350.00",
      "note": "Weekly groceries at Tops",
      "tags": ["weekly", "essentials"],
      "attachments": [
        {
          "id": "1a2b3c4d-...",
          "file_url": "https://storage.example.com/receipts/...",
          "file_size_bytes": 245120,
          "uploaded_at": "2026-08-15T11:00:00Z"
        }
      ],
      "occurred_at": "2026-08-15T10:45:00Z",
      "created_at": "2026-08-15T10:50:00Z",
      "updated_at": "2026-08-15T10:50:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total_items": 142,
    "total_pages": 8
  }
}
```

---

### POST /transactions

Create a new transaction.

**Request body:**

```json
{
  "account_id": "b2c3d4e5-...",
  "category_id": "f6a7b8c9-...",
  "type": "expense",
  "amount": "350.00",
  "currency": "THB",
  "note": "Weekly groceries at Tops",
  "tags": ["weekly", "essentials"],
  "occurred_at": "2026-08-15T10:45:00Z"
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| account_id | uuid | yes | Must belong to the user |
| category_id | uuid | no | Can be top-level or subcategory. Null for uncategorized |
| type | string | yes | `income`, `expense`, or `transfer` |
| amount | string | yes | Positive decimal string |
| currency | string | yes | Must match the account's currency |
| note | string | no | Max 500 chars |
| tags | string[] | no | Max 10 tags, each max 50 chars |
| occurred_at | datetime | yes | When the transaction actually happened |

**Response: `201 Created`** — returns the full transaction object (same shape as in the list response, with nested category and empty attachments array).

**Side effects:**
- `amount_in_default_currency` is auto-computed using the exchange rate for `occurred_at`'s date.
- If a budget exists for the transaction's category and period, the API checks spend against the threshold. If threshold is exceeded, a push notification is sent via FCM.

---

### PATCH /transactions/:id

Update an existing transaction. Only the provided fields are changed.

**Request body (all fields optional):**

```json
{
  "category_id": "a7b8c9d0-...",
  "amount": "400.00",
  "note": "Weekly groceries at Tops — updated",
  "tags": ["weekly", "essentials", "food"]
}
```

| Field | Type | Notes |
|-------|------|-------|
| category_id | uuid | |
| type | string | `income`, `expense`, `transfer` |
| amount | string | Positive decimal |
| note | string | Max 500 chars |
| tags | string[] | Replaces the entire tags array |
| occurred_at | datetime | |

**Response: `200 OK`** — returns the updated transaction object.

**Side effects:** `amount_in_default_currency` is recalculated if `amount` or `occurred_at` changes. Budget threshold re-checked.

**Errors:** `404` if transaction doesn't belong to user or is soft-deleted.

---

### DELETE /transactions/:id

Soft-delete a transaction (sets `deleted_at`). The transaction is excluded from all queries and reports but remains in the database for audit/recovery.

**Response: `200 OK`**

```json
{
  "message": "Transaction deleted",
  "id": "e5f6a7b8-..."
}
```

**Errors:** `404` if transaction doesn't belong to user or is already deleted.

---

## Categories

### GET /categories

Returns all categories available to the user: system-defined globals plus the user's custom categories. Response is a tree structure (parents with nested children).

**Response: `200 OK`**

```json
{
  "categories": [
    {
      "id": "a7b8c9d0-...",
      "name": "Food",
      "type": "expense",
      "icon": "utensils",
      "color": "#FF6B35",
      "is_system": true,
      "subcategories": [
        {
          "id": "f6a7b8c9-...",
          "name": "Groceries",
          "type": "expense",
          "icon": "shopping-cart",
          "color": "#FF6B35",
          "is_system": true
        },
        {
          "id": "b8c9d0e1-...",
          "name": "Dining Out",
          "type": "expense",
          "icon": "coffee",
          "color": "#FF6B35",
          "is_system": true
        }
      ]
    },
    {
      "id": "c9d0e1f2-...",
      "name": "Salary",
      "type": "income",
      "icon": "briefcase",
      "color": "#2EC4B6",
      "is_system": true,
      "subcategories": []
    },
    {
      "id": "d0e1f2a3-...",
      "name": "Side Hustle",
      "type": "income",
      "icon": "star",
      "color": "#9B5DE5",
      "is_system": false,
      "subcategories": []
    }
  ]
}
```

---

### POST /categories

Create a custom category or subcategory for the authenticated user.

**Request body:**

```json
{
  "name": "Bubble Tea",
  "type": "expense",
  "parent_id": "a7b8c9d0-...",
  "icon": "cup",
  "color": "#E88D67"
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| name | string | yes | Max 50 chars, unique per user within the same parent |
| type | string | yes | `income` or `expense` |
| parent_id | uuid | no | If set, creates a subcategory. The parent must be a top-level category (no 3rd-level nesting) |
| icon | string | no | Icon identifier from the app's icon set |
| color | string | no | Hex color code |

**Response: `201 Created`**

```json
{
  "category": {
    "id": "e1f2a3b4-...",
    "name": "Bubble Tea",
    "type": "expense",
    "parent_id": "a7b8c9d0-...",
    "icon": "cup",
    "color": "#E88D67",
    "is_system": false,
    "created_at": "2026-08-15T11:00:00Z"
  }
}
```

**Errors:** `400` if `parent_id` refers to a category that already has a parent (would create a 3rd level). `409` if name is duplicate under the same parent for this user.

---

## Budgets

### GET /budgets

List all budgets for the authenticated user, including current spend progress.

**Query parameters:**

| Param | Type | Default | Notes |
|-------|------|---------|-------|
| period | string | — | Filter by `weekly`, `monthly`, or `yearly` |

**Response: `200 OK`**

```json
{
  "budgets": [
    {
      "id": "f2a3b4c5-...",
      "category": {
        "id": "a7b8c9d0-...",
        "name": "Food"
      },
      "amount": "8000.00",
      "currency": "THB",
      "period": "monthly",
      "start_date": "2026-08-01",
      "alert_threshold_pct": 80,
      "current_spend": "6540.00",
      "remaining": "1460.00",
      "percent_used": 81.75,
      "is_over_budget": false,
      "created_at": "2026-08-01T00:00:00Z"
    },
    {
      "id": "a3b4c5d6-...",
      "category": null,
      "amount": "30000.00",
      "currency": "THB",
      "period": "monthly",
      "start_date": "2026-08-01",
      "alert_threshold_pct": 80,
      "current_spend": "18200.00",
      "remaining": "11800.00",
      "percent_used": 60.67,
      "is_over_budget": false,
      "created_at": "2026-08-01T00:00:00Z"
    }
  ]
}
```

When `category` is `null`, the budget applies to overall spending across all categories.

---

### POST /budgets

Create a new budget.

**Request body:**

```json
{
  "category_id": "a7b8c9d0-...",
  "amount": "8000.00",
  "currency": "THB",
  "period": "monthly",
  "start_date": "2026-08-01",
  "alert_threshold_pct": 80
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| category_id | uuid | no | Null for an overall budget. Must be a category the user has access to |
| amount | string | yes | Positive decimal |
| currency | string | yes | ISO 4217 |
| period | string | yes | `weekly`, `monthly`, or `yearly` |
| start_date | date | yes | `YYYY-MM-DD`. Budget period starts from this date |
| alert_threshold_pct | int | no | 1–100. Defaults to `80`. Triggers push notification when spend crosses this % |

**Response: `201 Created`** — returns the budget object (same shape as in the list, with `current_spend`, `remaining`, and `percent_used` computed as of now).

**Errors:** `409` if a budget for the same category + period already exists.

---

### PATCH /budgets/:id

Update an existing budget.

**Request body (all fields optional):**

```json
{
  "amount": "10000.00",
  "alert_threshold_pct": 90
}
```

| Field | Type | Notes |
|-------|------|-------|
| amount | string | Positive decimal |
| period | string | `weekly`, `monthly`, `yearly` |
| alert_threshold_pct | int | 1–100 |

**Response: `200 OK`** — returns the updated budget object.

---

### DELETE /budgets/:id

Permanently delete a budget.

**Response: `200 OK`**

```json
{
  "message": "Budget deleted",
  "id": "f2a3b4c5-..."
}
```

---

## Reports

### GET /reports/summary

Aggregated spending and income summary for a given period.

**Query parameters:**

| Param | Type | Required | Notes |
|-------|------|----------|-------|
| period | string | yes | `day`, `week` (Sunday–Saturday), `month`, `year`, or `custom` |
| date | string | unless custom | A date within the desired period. For `day`/`week`: `2026-08-12`. For `month`: `2026-08` or `2026-08-01`. For `year`: `2026` |
| from, to | string | custom only | Inclusive `YYYY-MM-DD` bounds; `to` ≥ `from`, at most 731 days |
| account_id | uuid | no | Filter to a specific account |
| category_id | uuid | no | Filter to one category. A top-level category includes its subcategories; a subcategory matches only itself |
| type | string | no | `expense` (default) or `income`: which transactions `by_category` breaks down |
| tz | string | no | IANA time zone (e.g. `Asia/Bangkok`) deciding which calendar day a transaction falls on. Defaults to `UTC` |
| currency | string | no | Report currency. Defaults to user's `default_currency`. All amounts are converted |

**Response: `200 OK`**

```json
{
  "period": "month",
  "start_date": "2026-08-01",
  "end_date": "2026-08-31",
  "currency": "THB",
  "total_income": "45000.00",
  "total_expense": "18200.00",
  "net": "26800.00",
  "by_category": [
    {
      "category": {
        "id": "a7b8c9d0-...",
        "name": "Food",
        "type": "expense"
      },
      "total": "6540.00",
      "percentage": 35.93,
      "subcategories": [
        { "name": "Groceries", "total": "4200.00", "percentage": 23.08 },
        { "name": "Dining Out", "total": "2340.00", "percentage": 12.86 }
      ]
    },
    {
      "category": {
        "id": "d1e2f3a4-...",
        "name": "Transport",
        "type": "expense"
      },
      "total": "3500.00",
      "percentage": 19.23,
      "subcategories": []
    }
  ],
  "daily_breakdown": [
    { "date": "2026-08-01", "income": "0.00", "expense": "1250.00" },
    { "date": "2026-08-02", "income": "45000.00", "expense": "580.00" }
  ]
}
```

Transfers are excluded. `daily_breakdown` has one row per day in the range, zero days included. `by_category` is sorted largest first and also carries `transaction_count`; transactions without a category are grouped under `"Uncategorized"` (empty `id`), and when a category has subcategory spending, entries filed on the parent itself appear as an `"Other"` subcategory (empty `id`). The response also echoes `time_zone`, `type`, and the overall `transaction_count`.

`percentage` values within `by_category` are relative to `total_expense` for expense categories and `total_income` for income categories.

---

## Attachments

### POST /transactions/:id/attachment

Upload a receipt image to a transaction. Uses a presigned-URL flow so the file goes directly from the mobile app to object storage (S3/R2) without passing through the API server.

**Step 1 — Request a presigned upload URL from the API:**

**Request body:**

```json
{
  "filename": "receipt.jpg",
  "content_type": "image/jpeg",
  "file_size_bytes": 245120
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| filename | string | yes | Original filename. Max 255 chars |
| content_type | string | yes | MIME type. Allowed: `image/jpeg`, `image/png`, `image/webp`, `application/pdf` |
| file_size_bytes | int | yes | Max 10 MB (10,485,760 bytes) |

**Response: `200 OK`**

```json
{
  "attachment_id": "1a2b3c4d-...",
  "upload_url": "https://s3.amazonaws.com/ledger-receipts/...?X-Amz-Signature=...",
  "upload_method": "PUT",
  "upload_headers": {
    "Content-Type": "image/jpeg"
  },
  "expires_in_seconds": 300
}
```

**Step 2 — Client uploads directly to the presigned URL:**

```
PUT <upload_url>
Content-Type: image/jpeg

<binary file data>
```

**Step 3 — Client confirms the upload:**

```
POST /transactions/:id/attachment/:attachment_id/confirm
```

**Response: `200 OK`**

```json
{
  "attachment": {
    "id": "1a2b3c4d-...",
    "transaction_id": "e5f6a7b8-...",
    "file_url": "https://cdn.example.com/receipts/...",
    "file_size_bytes": 245120,
    "uploaded_at": "2026-08-15T11:00:00Z"
  }
}
```

**Errors:** `400` if file too large or unsupported content type. `404` if transaction doesn't belong to user.

---

### DELETE /transactions/:id/attachment/:attachment_id

Remove an attachment. Deletes the database record and the file from object storage.

**Response: `200 OK`**

```json
{
  "message": "Attachment deleted",
  "id": "1a2b3c4d-..."
}
```

---

## Device Tokens (Push Notifications)

### POST /device-tokens

Register or update an FCM device token for push notifications.

**Request body:**

```json
{
  "fcm_token": "dGhpcyBpcyBhIGZjbSB0b2tlbg...",
  "platform": "ios"
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| fcm_token | string | yes | Firebase Cloud Messaging token |
| platform | string | yes | `ios` or `android` |

**Response: `201 Created`**

```json
{
  "id": "2b3c4d5e-...",
  "fcm_token": "dGhpcyBpc...",
  "platform": "ios",
  "created_at": "2026-08-15T10:00:00Z"
}
```

If the same `fcm_token` already exists for this user, it's updated (upsert) rather than duplicated.

---

### DELETE /device-tokens/:token

Unregister a device token (e.g., on logout).

**Response: `200 OK`**

```json
{
  "message": "Device token removed"
}
```

---

## Admin — Auth

### POST /admin/auth/login

Authenticate an admin user. Returns a JWT with an admin-scoped claim that cannot access end-user endpoints.

**Request body:**

```json
{
  "email": "admin@ledgerapp.com",
  "password": "<admin-password>"
}
```

**Response: `200 OK`**

```json
{
  "admin": {
    "id": "4d5e6f7a-...",
    "email": "admin@ledgerapp.com",
    "role": "superadmin",
    "created_at": "2026-01-01T00:00:00Z"
  },
  "access_token": "eyJhbG...",
  "refresh_token": "YWRtaW4gcmVm..."
}
```

---

## Admin — User Management

### GET /admin/users

List and search app users. Admin-only.

**Query parameters:**

| Param | Type | Default | Notes |
|-------|------|---------|-------|
| page | int | 1 | |
| limit | int | 20 | Max 100 |
| status | string | — | `active` or `suspended` |
| search | string | — | Searches email and display_name |
| sort | string | `-created_at` | Allowed: `created_at`, `email`, `display_name` |

**Response: `200 OK`**

```json
{
  "users": [
    {
      "id": "a1b2c3d4-...",
      "email": "user@example.com",
      "display_name": "Jane Doe",
      "default_currency": "THB",
      "status": "active",
      "transaction_count": 142,
      "last_active_at": "2026-08-15T09:30:00Z",
      "created_at": "2026-07-01T08:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total_items": 1523,
    "total_pages": 77
  }
}
```

---

### GET /admin/users/:id

Get detailed info for a single user, including their accounts and recent activity.

**Response: `200 OK`**

```json
{
  "user": {
    "id": "a1b2c3d4-...",
    "email": "user@example.com",
    "display_name": "Jane Doe",
    "avatar_url": "https://...",
    "default_currency": "THB",
    "status": "active",
    "google_id": "1234567890",
    "created_at": "2026-07-01T08:00:00Z",
    "updated_at": "2026-08-15T09:30:00Z"
  },
  "stats": {
    "total_accounts": 3,
    "total_transactions": 142,
    "total_budgets": 2,
    "last_transaction_at": "2026-08-15T09:25:00Z"
  }
}
```

---

### PATCH /admin/users/:id/suspend

Suspend or reactivate a user account. Suspended users cannot log in; existing tokens are invalidated.

**Request body:**

```json
{
  "status": "suspended",
  "reason": "Terms of service violation"
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| status | string | yes | `suspended` or `active` |
| reason | string | no | Stored in admin audit log |

**Response: `200 OK`**

```json
{
  "user": {
    "id": "a1b2c3d4-...",
    "email": "user@example.com",
    "status": "suspended"
  },
  "message": "User suspended"
}
```

**Side effects:** An entry is written to `admin_audit_log`.

---

## Admin — Categories

### GET /admin/categories

List all system (global) categories. Does not include user-created custom categories.

**Response: `200 OK`** — same tree structure as `GET /categories`, but filtered to `is_system = true` only.

---

### POST /admin/categories

Create or update a system-wide category.

**Request body:**

```json
{
  "name": "Healthcare",
  "type": "expense",
  "icon": "heart-pulse",
  "color": "#E63946",
  "parent_id": null
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| name | string | yes | Max 50 chars |
| type | string | yes | `income` or `expense` |
| icon | string | no | |
| color | string | no | Hex color |
| parent_id | uuid | no | To create a system subcategory |

**Response: `201 Created`** — returns the category object.

---

### PATCH /admin/categories/:id

Update a system category.

**Request body (all fields optional):**

```json
{
  "name": "Health & Medical",
  "icon": "stethoscope"
}
```

**Response: `200 OK`** — returns the updated category object.

---

### DELETE /admin/categories/:id

Delete a system category. Only allowed if no transactions reference it. Otherwise returns `409 Conflict` with the count of affected transactions.

**Response: `200 OK`**

```json
{
  "message": "Category deleted",
  "id": "..."
}
```

**Error: `409 Conflict`**

```json
{
  "error": {
    "code": "CATEGORY_IN_USE",
    "message": "Cannot delete category: 234 transactions reference it",
    "details": { "transaction_count": 234 }
  }
}
```

---

## Admin — Currencies & Exchange Rates

### GET /admin/currencies

List all currencies.

**Query parameters:**

| Param | Type | Default | Notes |
|-------|------|---------|-------|
| is_active | boolean | — | Filter by active/inactive |

**Response: `200 OK`**

```json
{
  "currencies": [
    {
      "code": "THB",
      "name": "Thai Baht",
      "symbol": "฿",
      "is_active": true
    },
    {
      "code": "USD",
      "name": "US Dollar",
      "symbol": "$",
      "is_active": true
    }
  ]
}
```

---

### POST /admin/currencies

Add a new currency.

**Request body:**

```json
{
  "code": "JPY",
  "name": "Japanese Yen",
  "symbol": "¥",
  "is_active": true
}
```

**Response: `201 Created`** — returns the currency object.

**Errors:** `409` if currency code already exists.

---

### PATCH /admin/currencies/:code

Update a currency (e.g. deactivate it).

**Request body:**

```json
{
  "is_active": false
}
```

**Response: `200 OK`** — returns the updated currency object.

---

### GET /admin/exchange-rates

List exchange rates with optional filtering.

**Query parameters:**

| Param | Type | Default | Notes |
|-------|------|---------|-------|
| base | string | — | Base currency code (e.g. `USD`) |
| target | string | — | Target currency code |
| from | date | — | Start date |
| to | date | — | End date |
| page | int | 1 | |
| limit | int | 50 | Max 200 |

**Response: `200 OK`**

```json
{
  "rates": [
    {
      "id": "5e6f7a8b-...",
      "base_currency": "USD",
      "target_currency": "THB",
      "rate": "34.85000000",
      "effective_date": "2026-08-15"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total_items": 365,
    "total_pages": 8
  }
}
```

---

### POST /admin/exchange-rates

Manually set an exchange rate for a specific date. Overwrites any existing rate for the same pair + date.

**Request body:**

```json
{
  "base_currency": "USD",
  "target_currency": "THB",
  "rate": "34.85",
  "effective_date": "2026-08-15"
}
```

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| base_currency | string | yes | Must exist in `currencies` |
| target_currency | string | yes | Must exist in `currencies` |
| rate | string | yes | Positive decimal, up to 8 decimal places |
| effective_date | date | yes | `YYYY-MM-DD` |

**Response: `201 Created`**

```json
{
  "rate": {
    "id": "6f7a8b9c-...",
    "base_currency": "USD",
    "target_currency": "THB",
    "rate": "34.85000000",
    "effective_date": "2026-08-15"
  }
}
```

---

## Admin — Analytics

### GET /admin/analytics/overview

High-level platform stats.

**Response: `200 OK`**

```json
{
  "total_users": 1523,
  "active_users_30d": 892,
  "new_users_30d": 134,
  "total_transactions_30d": 28456,
  "growth_rate_pct": 8.7,
  "top_currencies": [
    { "code": "THB", "user_count": 980 },
    { "code": "USD", "user_count": 312 },
    { "code": "EUR", "user_count": 145 }
  ]
}
```

---

### GET /admin/analytics/activity

User activity and transaction volume over time.

**Query parameters:**

| Param | Type | Default | Notes |
|-------|------|---------|-------|
| period | string | `daily` | `daily`, `weekly`, or `monthly` |
| from | date | 30 days ago | Start date |
| to | date | today | End date |

**Response: `200 OK`**

```json
{
  "period": "daily",
  "data_points": [
    {
      "date": "2026-08-14",
      "active_users": 245,
      "new_users": 5,
      "transactions_created": 1823,
      "total_expense_usd": "52340.00",
      "total_income_usd": "78120.00"
    },
    {
      "date": "2026-08-15",
      "active_users": 258,
      "new_users": 7,
      "transactions_created": 1956,
      "total_expense_usd": "55890.00",
      "total_income_usd": "45200.00"
    }
  ]
}
```

Monetary totals are normalized to USD using the exchange rate on each respective date.
