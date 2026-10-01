# Money Ledger API - Postman Collection Guide

## Overview
The `money.postman_collection.json` contains a complete collection of all API endpoints for the Money Ledger application with proper request/response examples and environment variables.

## Quick Start

### 1. Import the Collection
- Open Postman
- Click **Import** in the top-left corner
- Select the `money.postman_collection.json` file
- The collection will appear in the left sidebar

### 2. Set Environment Variables
Before making requests, configure these variables in Postman:

| Variable | Value | Description |
|----------|-------|-------------|
| `base_url` | `http://localhost:8080` | API server URL |
| `user_email` / `user_password` | (empty) | Login used by Register and Login |
| `admin_email` / `admin_password` | (empty) | Admin login used by Admin Login |
| `access_token` | (empty) | JWT access token - fill after login |
| `refresh_token` | (empty) | Refresh token - fill after login |
| `admin_access_token` | (empty) | Admin JWT token - fill after admin login |
| `account_id` | (empty) | Account ID - fill with test account ID |
| `transaction_id` | (empty) | Transaction ID - fill with test transaction ID |
| `category_id` | (empty) | Category ID - fill with test category ID |
| `budget_id` | (empty) | Budget ID - fill with test budget ID |
| `user_id` | (empty) | User ID - fill with test user ID |

Keep logins and tokens in a local Postman **environment** (its "Current value"
column is never synced or exported), not in the collection file, so they can not
end up in git.

**To set variables:**
1. Click the **Environment** icon (eye) in the top-right
2. Click **Edit** next to the environment
3. Add/update the variables

## API Organization

### 📝 Auth (4 endpoints)
- **Register** - Create new user account
- **Login** - Authenticate with email/password
- **Refresh Token** - Get new access token
- **Logout** - Revoke refresh token

### 💰 Accounts (5 endpoints)
- **List Accounts** - Get all user accounts
- **Create Account** - Add new account
- **Get Account** - Get specific account details
- **Update Account** - Modify account (name, archive status)
- **Delete Account** - Archive account

### 📊 Transactions (5 endpoints)
- **List Transactions** - Get transactions with pagination & filtering
- **Create Transaction** - Record income/expense/transfer
- **Get Transaction** - Get transaction details
- **Update Transaction** - Modify transaction
- **Delete Transaction** - Soft-delete transaction

### 🏷️ Categories (5 endpoints)
- **List Categories** - Get system & user categories
- **Create Category** - Add custom category
- **Get Category** - Get category details
- **Update Category** - Modify category
- **Delete Category** - Remove category

### 💳 Budgets (5 endpoints)
- **List Budgets** - Get all budgets
- **Create Budget** - Set spending limit
- **Get Budget** - Get budget details with spend progress
- **Update Budget** - Modify budget
- **Delete Budget** - Remove budget

### 📈 Reports (1 endpoint)
- **Get Summary Report** - Aggregated income/expense summary

### 📎 Attachments (3 endpoints)
- **Request Presigned URL** - Get URL for file upload
- **Confirm Upload** - Confirm upload completion
- **Delete Attachment** - Remove attachment

### 📱 Device Tokens (2 endpoints)
- **Register Device Token** - Register FCM token for push notifications
- **Delete Device Token** - Unregister device token

### 👨‍💼 Admin (12 endpoints)
#### Auth
- **Admin Login** - Authenticate as admin

#### Users
- **List Users** - Get all users with pagination
- **Get User Details** - Get user with stats
- **Suspend User** - Suspend/reactivate user

#### Categories
- **List System Categories** - Get global categories
- **Create System Category** - Add system category
- **Update System Category** - Modify system category
- **Delete System Category** - Remove system category

#### Currencies
- **List Currencies** - Get all currencies
- **Create Currency** - Add new currency
- **Update Currency** - Activate/deactivate currency

#### Exchange Rates
- **List Exchange Rates** - Get rates with filtering
- **Create Exchange Rate** - Set exchange rate

#### Analytics
- **Platform Overview** - High-level platform stats
- **Activity Over Time** - User activity trends

## Typical Workflow

### 1. Register & Login
```
1. POST /auth/register → Get access_token & refresh_token
2. Set access_token in environment variables
```

### 2. Create Account
```
1. POST /accounts with name, type, currency, initial_balance
2. Copy account_id from response
3. Set account_id in environment variables
```

### 3. Create Transaction
```
1. POST /transactions with account_id, type, amount, currency, etc.
2. Copy transaction_id from response
3. Set transaction_id in environment variables
```

### 4. View Reports
```
1. GET /reports/summary with period, date, currency
```

### 5. Admin Operations
```
1. POST /admin/auth/login → Get admin_access_token
2. Set admin_access_token in environment variables
3. Use admin endpoints
```

## Common Patterns

### Authentication
All protected endpoints require the `Authorization` header:
```
Authorization: Bearer {{access_token}}
```

### Query Parameters
List endpoints support pagination and filtering:
```
?page=1&limit=20&type=expense&sort=-occurred_at
```

### Request Body
Most POST/PATCH requests use JSON:
```json
{
  "name": "value",
  "amount": "100.00",
  "currency": "THB"
}
```

### Response Format
All responses follow this pattern:
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

## Tips & Best Practices

✅ **Always set `base_url`** before making requests
✅ **Save tokens** to environment after login
✅ **Use {{variable}}** syntax for dynamic values
✅ **Check response status** (200, 201, 400, 401, 404, etc.)
✅ **Test with sample data** before production
✅ **Review request/response** in the tabs below each request

## Troubleshooting

| Issue | Solution |
|-------|----------|
| 401 Unauthorized | Check `access_token` is set and not expired |
| 404 Not Found | Verify resource ID is correct |
| 400 Bad Request | Check request body format and required fields |
| 409 Conflict | Resource already exists or constraint violation |
| Variables showing as blank | Click Environment button and verify variables are set |

## Import Notes

- All variables are pre-configured with placeholders
- Request body examples use realistic test data
- Modify values as needed for your testing
- Use Postman's Test feature to automate validation

Enjoy using the Money Ledger API! 🚀
