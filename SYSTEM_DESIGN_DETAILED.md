# Money Ledger - Detailed System Design & Data Dictionary

## Table of Contents
1. [User](#user)
2. [Account](#account)
3. [Transaction](#transaction)
4. [Category](#category)
5. [Budget](#budget)
6. [Device Token](#device-token)
7. [Attachment](#attachment)
8. [Exchange Rate](#exchange-rate)
9. [Admin User](#admin-user)

---

## User

### Description
Represents a user account in the Money Ledger application. Users can register via email/password or Google OAuth. Each user manages their own accounts, transactions, categories, and budgets.

### Fields

| Field | Type | Required | Constraints | Description | Example |
|-------|------|----------|-------------|-------------|---------|
| `id` | UUID v4 | Yes (Auto) | Read-only | Unique identifier generated on registration | `a1b2c3d4-e5f6-47a8-b9c0-d1e2f3a4b5c6` |
| `email` | String | Yes | Unique, Valid email format | User's email address (login credential) | `jane.doe@example.com` |
| `password_hash` | String | Yes | Min 8 chars, 1 uppercase, 1 number, 1 special char | Bcrypt hashed password (never sent in responses) | `$2a$10$N9qo8uLO...` |
| `display_name` | String | No | Max 100 chars | User's full name or display name | `Jane Doe` |
| `avatar_url` | String | No | Valid URL | Profile picture URL (from Google OAuth or custom) | `https://lh3.googleusercontent.com/a/...` |
| `default_currency` | String | Yes | Valid ISO 4217 code | Default currency for reports and new accounts | `USD`, `THB`, `EUR` |
| `status` | String | Yes | See constraints below | Account status | `active`, `suspended` |
| `google_id` | String | No | Unique (if set) | Google OAuth ID for SSO | `1234567890123456789` |
| `created_at` | Timestamp | Yes (Auto) | ISO 8601 | Account creation date | `2026-08-15T10:00:00Z` |
| `updated_at` | Timestamp | Yes (Auto) | ISO 8601 | Last profile update | `2026-08-15T10:00:00Z` |

### Status Values

| Status | Description | Behavior |
|--------|-------------|----------|
| `active` | Normal user account | Can login, create/modify transactions |
| `suspended` | Account suspended by admin | Cannot login, existing tokens invalidated |

### Example Request (Register)
```json
{
  "email": "jane.doe@example.com",
  "password": "<password>",
  "display_name": "Jane Doe",
  "default_currency": "USD"
}
```

### Example Response
```json
{
  "user": {
    "id": "a1b2c3d4-e5f6-47a8-b9c0-d1e2f3a4b5c6",
    "email": "jane.doe@example.com",
    "display_name": "Jane Doe",
    "default_currency": "USD",
    "status": "active",
    "created_at": "2026-08-15T10:00:00Z"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "dGhpcyBpcyBhIHJlZnJl...",
  "expires_in": 900,
  "refresh_token_expires_in": 2592000
}
```

---

## Account

### Description
Represents a financial account (bank account, wallet, credit card, etc.) that belongs to a user. Accounts track initial balance and current balance (calculated from transactions).

### Fields

| Field | Type | Required | Constraints | Description | Example |
|-------|------|----------|-------------|-------------|---------|
| `id` | UUID v4 | Yes (Auto) | Read-only | Unique account identifier | `b2c3d4e5-f6a7-48b9-c0d1-e2f3a4b5c6d7` |
| `user_id` | UUID v4 | Yes | FK to users table | Owner of the account | `a1b2c3d4-e5f6-47a8-b9c0-d1e2f3a4b5c6` |
| `name` | String | Yes | Max 100 chars, unique per user | Account display name | `Main Wallet`, `Kasikorn Savings` |
| `type` | String | Yes | One of: `cash`, `bank`, `credit_card`, `e_wallet` | Account category | See "Account Types" table below |
| `currency` | String | Yes | Valid ISO 4217 code | Account currency (cannot change) | `THB`, `USD`, `EUR` |
| `initial_balance` | Decimal(18,2) | No | Default: 0.00 | Opening balance when account created | `1000.00`, `50000.00` |
| `current_balance` | Decimal(18,2) | Yes (Calculated) | Read-only | = initial_balance + sum(income) - sum(expense) | `15420.50` |
| `is_archived` | Boolean | No | Default: false | Archive account without deleting | `true`, `false` |
| `created_at` | Timestamp | Yes (Auto) | ISO 8601 | Account creation date | `2026-08-01T08:00:00Z` |

### Account Types

| Type | Description | Use Case | Example |
|------|-------------|----------|---------|
| `cash` | Physical cash or wallet | Money in hand, petty cash | "Wallet", "Pocket Money" |
| `bank` | Bank account | Savings account, checking account | "Bangkok Bank", "Kasikorn Savings" |
| `credit_card` | Credit or debit card | Card spending tracking | "VISA Card", "Mastercard" |
| `e_wallet` | Digital payment wallet | Apple Pay, Google Pay, WeChat Pay | "Grab Wallet", "Line Pay" |

### Example Request (Create)
```json
{
  "name": "Main Wallet",
  "type": "e_wallet",
  "currency": "THB",
  "initial_balance": "5000.00"
}
```

### Example Response
```json
{
  "account": {
    "id": "b2c3d4e5-f6a7-48b9-c0d1-e2f3a4b5c6d7",
    "name": "Main Wallet",
    "type": "e_wallet",
    "currency": "THB",
    "initial_balance": "5000.00",
    "current_balance": "5000.00",
    "is_archived": false,
    "created_at": "2026-08-15T10:30:00Z"
  }
}
```

---

## Transaction

### Description
Represents a financial transaction (income, expense, or transfer) recorded against an account. Transactions are linked to categories and can have tags and attachments.

### Fields

| Field | Type | Required | Constraints | Description | Example |
|-------|------|----------|-------------|-------------|---------|
| `id` | UUID v4 | Yes (Auto) | Read-only | Transaction identifier | `e5f6a7b8-c9d0-49e1-d2e3-f4a5b6c7d8e9` |
| `user_id` | UUID v4 | Yes | FK to users table | Transaction owner | `a1b2c3d4-e5f6-47a8-b9c0-d1e2f3a4b5c6` |
| `account_id` | UUID v4 | Yes | FK to accounts table | Account affected | `b2c3d4e5-f6a7-48b9-c0d1-e2f3a4b5c6d7` |
| `category_id` | UUID v4 | No | FK to categories table | Transaction category | `f6a7b8c9-d0e1-4af2-e3f4-a5b6c7d8e9f0` |
| `type` | String | Yes | One of: `income`, `expense`, `transfer` | Transaction kind | See "Transaction Types" table |
| `amount` | Decimal(18,2) | Yes | > 0.00 | Transaction amount (always positive) | `350.00`, `5000.00` |
| `currency` | String | Yes | Valid ISO 4217 code | Transaction currency | `THB`, `USD` |
| `amount_in_default_currency` | Decimal(18,2) | Yes (Auto) | Using exchange rate for date | Auto-converted to user's default currency | `350.00` |
| `note` | String | No | Max 500 chars | Transaction description/memo | `Weekly groceries at Tops` |
| `tags` | String[] | No | Max 10 tags, each 50 chars | Custom transaction tags | `["weekly", "essentials", "food"]` |
| `occurred_at` | Timestamp | Yes | ISO 8601 | When transaction happened | `2026-08-15T10:45:00Z` |
| `deleted_at` | Timestamp | No | ISO 8601 | Soft-delete timestamp (null if active) | `null` or `2026-08-16T14:30:00Z` |
| `created_at` | Timestamp | Yes (Auto) | ISO 8601 | Record creation time | `2026-08-15T10:50:00Z` |
| `updated_at` | Timestamp | Yes (Auto) | ISO 8601 | Last modification time | `2026-08-15T10:50:00Z` |

### Transaction Types

| Type | Description | Effect on Balance | Use Case | Example |
|------|-------------|-------------------|----------|---------|
| `income` | Money received | Increases balance | Salary, bonus, refund | Salary payment, freelance earnings |
| `expense` | Money spent | Decreases balance | Shopping, dining, utilities | Groceries, rent, electricity bill |
| `transfer` | Movement between accounts | Decreases from source, increases to destination | Moving money between own accounts | Transfer to savings account |

### Example Request (Create)
```json
{
  "account_id": "b2c3d4e5-f6a7-48b9-c0d1-e2f3a4b5c6d7",
  "category_id": "f6a7b8c9-d0e1-4af2-e3f4-a5b6c7d8e9f0",
  "type": "expense",
  "amount": "350.00",
  "currency": "THB",
  "note": "Weekly groceries at Tops",
  "tags": ["weekly", "essentials"],
  "occurred_at": "2026-08-15T10:45:00Z"
}
```

### Example Response
```json
{
  "id": "e5f6a7b8-c9d0-49e1-d2e3-f4a5b6c7d8e9",
  "account_id": "b2c3d4e5-f6a7-48b9-c0d1-e2f3a4b5c6d7",
  "account_name": "Main Wallet",
  "category": {
    "id": "f6a7b8c9-d0e1-4af2-e3f4-a5b6c7d8e9f0",
    "name": "Groceries",
    "parent": {
      "id": "a7b8c9d0-e1f2-4a03-f4a5-b6c7d8e9f0a1",
      "name": "Food"
    }
  },
  "type": "expense",
  "amount": "350.00",
  "currency": "THB",
  "amount_in_default_currency": "350.00",
  "note": "Weekly groceries at Tops",
  "tags": ["weekly", "essentials"],
  "attachments": [],
  "occurred_at": "2026-08-15T10:45:00Z",
  "created_at": "2026-08-15T10:50:00Z",
  "updated_at": "2026-08-15T10:50:00Z"
}
```

---

## Category

### Description
Categories organize transactions hierarchically (parent-child). System categories are predefined globally; users can create custom categories. Categories can be `income` or `expense` type.

### Fields

| Field | Type | Required | Constraints | Description | Example |
|-------|------|----------|-------------|-------------|---------|
| `id` | UUID v4 | Yes (Auto) | Read-only | Category identifier | `a7b8c9d0-e1f2-4a03-f4a5-b6c7d8e9f0a1` |
| `user_id` | UUID v4 | No | FK to users table | Owner (null = system category) | `a1b2c3d4-e5f6-47a8-b9c0-d1e2f3a4b5c6` or `null` |
| `parent_id` | UUID v4 | No | FK to categories table (same user) | Parent category (for subcategories) | `a7b8c9d0-e1f2-4a03-f4a5-b6c7d8e9f0a1` |
| `name` | String | Yes | Max 50 chars, unique per parent | Category name | `Groceries`, `Dining Out` |
| `type` | String | Yes | One of: `income`, `expense` | Category type | See "Category Types" table |
| `icon` | String | No | Icon identifier | Icon name from design system | `shopping-cart`, `coffee`, `briefcase` |
| `color` | String | No | Hex color code | Display color | `#FF6B35`, `#2EC4B6` |
| `is_system` | Boolean | Yes | Default: false | System-defined vs user-created | `true` (system), `false` (custom) |
| `created_at` | Timestamp | Yes (Auto) | ISO 8601 | Creation date | `2026-08-15T11:00:00Z` |

### Category Types

| Type | Description | Subcategories | Example |
|------|-------------|----------------|---------|
| `income` | Income/revenue categories | Optional | Salary, Bonus, Freelance |
| `expense` | Spending/cost categories | Optional | Groceries, Dining, Utilities |

### Category Hierarchy Rules

- **System Categories**: Created by admin, available to all users
  - Can have subcategories
  - Cannot be modified by regular users
  - Examples: Food > Groceries, Food > Dining Out
  
- **Custom Categories**: Created by individual users
  - Max 2 levels (parent + child, no 3rd level)
  - Cannot be deleted if transactions reference them
  - Example: Salary (parent) > Bonus (child)

### Example System Categories

```
INCOME
├── Salary
├── Bonus
├── Freelance
└── Interest

EXPENSES
├── Food
│   ├── Groceries
│   └── Dining Out
├── Transport
│   ├── Fuel
│   ├── Parking
│   └── Public Transport
├── Utilities
│   ├── Electricity
│   ├── Water
│   └── Internet
└── Entertainment
    ├── Movies
    └── Games
```

### Example Request (Create)
```json
{
  "name": "Bubble Tea",
  "type": "expense",
  "parent_id": "a7b8c9d0-e1f2-4a03-f4a5-b6c7d8e9f0a1",
  "icon": "cup",
  "color": "#E88D67"
}
```

---

## Budget

### Description
Spending limit for a category or overall spending during a specific period (weekly/monthly/yearly). Tracks current spend and alerts when threshold exceeded.

### Fields

| Field | Type | Required | Constraints | Description | Example |
|-------|------|----------|-------------|-------------|---------|
| `id` | UUID v4 | Yes (Auto) | Read-only | Budget identifier | `f2a3b4c5-d6e7-4af8-b9c0-d1e2f3a4b5c6` |
| `user_id` | UUID v4 | Yes | FK to users table | Budget owner | `a1b2c3d4-e5f6-47a8-b9c0-d1e2f3a4b5c6` |
| `category_id` | UUID v4 | No | FK to categories table | Budget scope (null = overall budget) | `a7b8c9d0-e1f2-4a03-f4a5-b6c7d8e9f0a1` or `null` |
| `amount` | Decimal(18,2) | Yes | > 0.00 | Budget limit | `8000.00`, `30000.00` |
| `currency` | String | Yes | Valid ISO 4217 code | Budget currency | `THB`, `USD` |
| `period` | String | Yes | One of: `weekly`, `monthly`, `yearly` | Budget cycle | See "Budget Periods" table |
| `start_date` | Date | Yes | YYYY-MM-DD format | First day of budget period | `2026-08-01`, `2026-08-10` |
| `alert_threshold_pct` | Integer | No | 1-100, default: 80 | % of budget to trigger alert | `80`, `90` |
| `current_spend` | Decimal(18,2) | Yes (Calculated) | Read-only | Spending so far in period | `6540.00` |
| `remaining` | Decimal(18,2) | Yes (Calculated) | Read-only | = amount - current_spend | `1460.00` |
| `percent_used` | Float | Yes (Calculated) | Read-only | (current_spend / amount) × 100 | `81.75` |
| `is_over_budget` | Boolean | Yes (Calculated) | Read-only | current_spend > amount | `false` |
| `created_at` | Timestamp | Yes (Auto) | ISO 8601 | Creation date | `2026-08-01T00:00:00Z` |

### Budget Periods

| Period | Description | Cycle | Start/Reset | Example Dates |
|--------|-------------|-------|------------|----------------|
| `weekly` | Week budget | 7 days | Monday | Aug 10-16, Aug 17-23 |
| `monthly` | Month budget | Calendar month | 1st of month | Aug 1-31, Sep 1-30 |
| `yearly` | Year budget | Calendar year | Jan 1 | Jan 1 - Dec 31 |

### Budget Rules

1. **One budget per category per period** - Cannot create duplicate
2. **Overall budget** - `category_id` = null for spending limit across all categories
3. **Alert threshold** - Sends push notification when threshold exceeded
4. **Period calculation** - Spending calculated from `start_date` to period end

### Example: Monthly Food Budget
```json
{
  "category_id": "a7b8c9d0-e1f2-4a03-f4a5-b6c7d8e9f0a1",
  "amount": "8000.00",
  "currency": "THB",
  "period": "monthly",
  "start_date": "2026-08-01",
  "alert_threshold_pct": 80
}
```

### Example Response
```json
{
  "id": "f2a3b4c5-d6e7-4af8-b9c0-d1e2f3a4b5c6",
  "category": {
    "id": "a7b8c9d0-e1f2-4a03-f4a5-b6c7d8e9f0a1",
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
}
```

---

## Device Token

### Description
Firebase Cloud Messaging (FCM) token for push notifications. Users register device tokens to receive alerts (budget exceeded, etc.).

### Fields

| Field | Type | Required | Constraints | Description | Example |
|-------|------|----------|-------------|-------------|---------|
| `id` | UUID v4 | Yes (Auto) | Read-only | Token record identifier | `2b3c4d5e-f6a7-48b9-c0d1-e2f3a4b5c6d7` |
| `user_id` | UUID v4 | Yes | FK to users table | Token owner | `a1b2c3d4-e5f6-47a8-b9c0-d1e2f3a4b5c6` |
| `fcm_token` | String | Yes | Unique per user | Firebase token | `dGhpcyBpcyBhIGZjbSB0b2tlbg...` |
| `platform` | String | Yes | One of: `ios`, `android` | Device OS | See "Device Platforms" table |
| `created_at` | Timestamp | Yes (Auto) | ISO 8601 | Registration date | `2026-08-15T10:00:00Z` |

### Device Platforms

| Platform | Description | SDK | Example |
|----------|-------------|-----|---------|
| `ios` | Apple iOS devices | Firebase Messaging for iOS | iPhone, iPad |
| `android` | Android devices | Firebase Messaging for Android | Android phone, tablet |

### Notes

- **Upsert behavior**: Registering same token again updates existing record
- **Push notifications sent for**: Budget alerts, transaction confirmations
- **Unregister**: Delete token on logout or app uninstall

### Example Request
```json
{
  "fcm_token": "dGhpcyBpcyBhIGZjbSB0b2tlbg...",
  "platform": "ios"
}
```

---

## Attachment

### Description
File attachments (receipts, invoices) linked to transactions. Uses presigned URLs for secure direct upload to cloud storage.

### Fields

| Field | Type | Required | Constraints | Description | Example |
|-------|------|----------|-------------|-------------|---------|
| `id` | UUID v4 | Yes (Auto) | Read-only | Attachment identifier | `1a2b3c4d-e5f6-47a8-b9c0-d1e2f3a4b5c6` |
| `transaction_id` | UUID v4 | Yes | FK to transactions table | Associated transaction | `e5f6a7b8-c9d0-49e1-d2e3-f4a5b6c7d8e9` |
| `file_url` | String | Yes | Valid HTTPS URL | CDN/cloud storage URL | `https://cdn.example.com/receipts/...` |
| `file_size_bytes` | Integer | No | Max 10,485,760 (10 MB) | File size in bytes | `245120` |
| `uploaded_at` | Timestamp | Yes (Auto) | ISO 8601 | Upload completion time | `2026-08-15T11:00:00Z` |

### Attachment Upload Process

1. **Request presigned URL**
   - POST `/transactions/:id/attachment`
   - Provide: filename, content_type, file_size_bytes

2. **Receive upload details**
   - GET presigned URL valid for 5 minutes
   - Upload method (PUT)
   - Required headers

3. **Client uploads file**
   - Direct upload to presigned URL (avoids server overhead)

4. **Confirm upload**
   - POST `/transactions/:id/attachment/:attachment_id/confirm`
   - Server marks attachment as completed

### Supported File Types

| Content Type | Extension | Max Size | Use |
|--------------|-----------|----------|-----|
| `image/jpeg` | .jpg | 10 MB | Receipt photo |
| `image/png` | .png | 10 MB | Invoice scan |
| `image/webp` | .webp | 10 MB | Modern image format |
| `application/pdf` | .pdf | 10 MB | Document scan |

### Example: Request Presigned URL
```json
{
  "filename": "receipt.jpg",
  "content_type": "image/jpeg",
  "file_size_bytes": 245120
}
```

### Example: Presigned URL Response
```json
{
  "attachment_id": "1a2b3c4d-e5f6-47a8-b9c0-d1e2f3a4b5c6",
  "upload_url": "https://s3.amazonaws.com/ledger-receipts/...?X-Amz-Signature=...",
  "upload_method": "PUT",
  "upload_headers": {
    "Content-Type": "image/jpeg"
  },
  "expires_in_seconds": 300
}
```

---

## Exchange Rate

### Description
Currency conversion rate for a specific date pair. Used to calculate `amount_in_default_currency` for transactions.

### Fields

| Field | Type | Required | Constraints | Description | Example |
|-------|------|----------|-------------|-------------|---------|
| `id` | UUID v4 | Yes (Auto) | Read-only | Rate record identifier | `5e6f7a8b-c9d0-49e1-d2e3-f4a5b6c7d8e9` |
| `base_currency` | String | Yes | Valid ISO 4217 code, FK to currencies | Source currency | `USD` |
| `target_currency` | String | Yes | Valid ISO 4217 code, FK to currencies | Target currency | `THB` |
| `rate` | Decimal(20,8) | Yes | > 0.00000000 | Exchange rate | `34.85000000` |
| `effective_date` | Date | Yes | YYYY-MM-DD, unique per pair | Date this rate applies | `2026-08-15` |

### Unique Constraint
Only one rate per (base, target, date) combination. Creating duplicate overwrites existing.

### Example Rates

| From | To | Rate | Date | Meaning |
|------|----|----|------|---------|
| USD | THB | 34.85 | 2026-08-15 | 1 USD = 34.85 THB |
| EUR | USD | 1.10 | 2026-08-15 | 1 EUR = 1.10 USD |
| THB | USD | 0.02873 | 2026-08-15 | 1 THB = 0.02873 USD |

### Admin-only Operations
- Only admins can create/update exchange rates
- Regular users cannot modify rates
- Rates are used automatically for conversion

### Example Request
```json
{
  "base_currency": "USD",
  "target_currency": "THB",
  "rate": "34.85",
  "effective_date": "2026-08-15"
}
```

---

## Admin User

### Description
Administrative user account with elevated permissions. Separate from regular users; cannot manage personal finances.

### Fields

| Field | Type | Required | Constraints | Description | Example |
|-------|------|----------|-------------|-------------|---------|
| `id` | UUID v4 | Yes (Auto) | Read-only | Admin user identifier | `4d5e6f7a-b8c9-49d0-e1f2-a3b4c5d6e7f8` |
| `email` | String | Yes | Unique | Admin login email | `admin@ledgerapp.com` |
| `password_hash` | String | Yes | Bcrypt hashed | Admin password | `$2a$10$N9qo8uLO...` |
| `role` | String | Yes | One of: `superadmin`, `moderator`, `support` | Permission level | See "Admin Roles" table |
| `created_at` | Timestamp | Yes (Auto) | ISO 8601 | Account creation | `2026-01-01T00:00:00Z` |

### Admin Roles

| Role | Description | Permissions |
|------|-------------|-------------|
| `superadmin` | Full system access | All operations, user suspension, system settings |
| `moderator` | Content moderation | User suspension, category management |
| `support` | Customer support | View users, read-only analytics |

### Admin Permissions

| Resource | Superadmin | Moderator | Support |
|----------|-----------|-----------|---------|
| View all users | ✅ | ✅ | ✅ |
| Suspend users | ✅ | ✅ | ❌ |
| Manage system categories | ✅ | ✅ | ❌ |
| Manage currencies | ✅ | ❌ | ❌ |
| Manage exchange rates | ✅ | ❌ | ❌ |
| View analytics | ✅ | ❌ | ✅ |

### Audit Trail
All admin actions logged to `admin_audit_log`:
- User suspension
- Category modifications
- Rate changes
- etc.

---

## Summary Table

| Entity | Primary Key | User-Scoped | Soft Delete | Has History |
|--------|------------|-------------|------------|------------|
| User | `id` | N/A | No (status) | Yes |
| Account | `id` | Yes (user_id) | Yes (is_archived) | No |
| Transaction | `id` | Yes (user_id) | Yes (deleted_at) | No |
| Category | `id` | Optional (user_id) | No | No |
| Budget | `id` | Yes (user_id) | No | No |
| Device Token | `id` | Yes (user_id) | No | No |
| Attachment | `id` | Via transaction | No | No |
| Exchange Rate | `id` | N/A (admin only) | No | No |
| Admin User | `id` | N/A | No | Yes |

---

## API Response Format Standards

### Success Response (2xx)
```json
{
  "data": { ... },
  "pagination": {
    "page": 1,
    "limit": 20,
    "total_items": 100,
    "total_pages": 5
  }
}
```

### Error Response (4xx, 5xx)
```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable message",
    "details": { ... }
  }
}
```

### HTTP Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| 200 | Success | GET /accounts returns list |
| 201 | Created | POST /accounts returns new account |
| 400 | Bad request | Missing required field |
| 401 | Unauthorized | Missing/invalid token |
| 403 | Forbidden | User suspended or permission denied |
| 404 | Not found | Account ID doesn't exist |
| 409 | Conflict | Duplicate email, category already exists |
| 429 | Rate limited | Too many requests |
| 500 | Server error | Database error, unhandled exception |

---

## Data Validation Rules

### Email
- Format: `user@example.com`
- Unique per user
- Case-insensitive comparison

### Password
- Minimum 8 characters
- At least 1 uppercase letter
- At least 1 number
- At least 1 special character (!@#$%^&*)

### Currency Code
- ISO 4217 3-letter code
- Uppercase (USD, THB, EUR)
- Must exist in currencies table

### Decimal Numbers
- Format: Decimal(18,2)
- Up to 18 digits total
- Exactly 2 decimal places
- Examples: 1.00, 1000.50, 999999999999999.99

### UUID Format
- RFC 4122 version 4
- Hexadecimal with hyphens
- Format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
- Example: a1b2c3d4-e5f6-47a8-b9c0-d1e2f3a4b5c6

### Timestamp Format
- ISO 8601
- UTC timezone (Z)
- Format: YYYY-MM-DDTHH:mm:ssZ
- Example: 2026-08-15T10:45:00Z

---

## Constraints & Relationships

### Foreign Key Relationships
```
User (1) ──┬─ (M) Account
           ├─ (M) Transaction
           ├─ (M) Category (user custom only)
           ├─ (M) Budget
           └─ (M) Device Token

Account (1) ─ (M) Transaction

Category (1) ──┬─ (M) Transaction
              └─ (M) Budget

Transaction (1) ─ (M) Attachment

Currency (1) ──┬─ (M) Account
              ├─ (M) Exchange Rate (both directions)
              └─ (M) Budget
```

### Business Rules

1. **Account currency is immutable** - Cannot change currency after creation
2. **One budget per category per period** - Prevents duplicate budgets
3. **No circular categories** - Parent cannot be child of itself
4. **Max 2 category levels** - No 3rd level nesting
5. **Transactions are immutable** (mostly) - Only amount, note, tags can change
6. **Soft-delete only** - Transactions never permanently deleted (for audit)
7. **Exchange rates must exist** - Cannot record transaction with missing rate pair

---

## Examples & Use Cases

### Use Case 1: Thai User Tracking USD Expenses
```
User: default_currency = THB
Transaction: amount=100, currency=USD, occurred_at=2026-08-15
Exchange Rate: USD→THB on 2026-08-15 = 34.85
Result: amount_in_default_currency = 100 × 34.85 = 3485.00 THB
```

### Use Case 2: Monthly Budget Alert
```
Budget: category=Food, amount=8000 THB, period=monthly
Start Date: 2026-08-01
Current Spend: 6600 THB (82.5%)
Alert Threshold: 80%
Action: Push notification sent "Food budget exceeded: 82.5% used"
```

### Use Case 3: Account Balance Calculation
```
Account: initial_balance = 5000 THB
Transactions:
  - Income: +45000 (salary)
  - Expense: -18200 (living costs)
  - Expense: -6540 (food)
Result: current_balance = 5000 + 45000 - 18200 - 6540 = 25260 THB
```

### Use Case 4: Receipt Upload
```
1. User takes photo of receipt
2. POST /attachment → Get presigned URL (valid 5 min)
3. App uploads directly to S3 (bypass server)
4. App confirms upload
5. Receipt linked to transaction permanently
```

---

End of Detailed System Design
