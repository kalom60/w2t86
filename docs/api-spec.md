# District Materials Commerce & Logistics Portal — API Specification

**Version:** 1.0  
**Scope:** HTTP route contracts exposed by the Fiber server in this repository.  
**Architecture:** Server-rendered web app with HTMX partial endpoints and selected JSON APIs.

---

## Conventions

### Transport & Content Types
- Most routes render HTML templates (`text/html`), often with HTMX partials.
- JSON is used for health/metrics/map/KPI/stat endpoints and middleware denials.
- CSV is used for analytics export endpoints.

### Error Semantics
- Shared JSON error shape for handler-level API errors:
```json
{ "code": 400, "msg": "Invalid material ID" }
```
- `RequireAuth` middleware failures return:
```json
{ "error": "unauthorized" }
```
- RBAC failures return:
```json
{ "error": "forbidden" }
```
- Rate-limiter failures return:
```json
{ "error": "rate limit exceeded" }
```
- For HTMX requests (`HX-Request: true`), many handlers return rendered `partials/error_inline` instead of JSON.

### HTMX Behavior
- Mutating endpoints generally support two modes:
1. HTMX request: return a partial (`flash`, badge, row, status badge, etc.)
2. Non-HTMX request: redirect (typically `302 Found`) to a page route

### Authentication
- Session cookie: `session_token` (`HttpOnly`, `SameSite=Strict`, 24h expiry).
- Login sets the cookie; logout clears it and expires it.

### CSRF
- CSRF middleware applies to authenticated mutating routes.
- Token can be provided by header `X-Csrf-Token` or form field `csrf_token`.
- Safe GET/SSE endpoints do not require CSRF token.

### Roles
- `student`
- `instructor`
- `manager`
- `clerk`
- `moderator`
- `admin`

### Date/Time
- Timestamps persisted as RFC3339/SQLite datetime text depending on table column usage.
- DND logic uses configured `TIMEZONE` (IANA name), default `UTC`.

---

## Table of Contents

1. [Platform Endpoints](#1-platform-endpoints)
2. [Auth Endpoints](#2-auth-endpoints)
3. [Dashboard & Navigation Endpoints](#3-dashboard--navigation-endpoints)
4. [Materials & Engagement Endpoints](#4-materials--engagement-endpoints)
5. [Orders & Returns Endpoints](#5-orders--returns-endpoints)
6. [Distribution Endpoints](#6-distribution-endpoints)
7. [Inbox & Messaging Endpoints](#7-inbox--messaging-endpoints)
8. [Moderation Endpoints](#8-moderation-endpoints)
9. [Courses Endpoints](#9-courses-endpoints)
10. [Admin Endpoints](#10-admin-endpoints)
11. [Analytics & Geospatial Endpoints](#11-analytics--geospatial-endpoints)
12. [Status Models & Enumerations](#12-status-models--enumerations)

---

## 1. Platform Endpoints

### 1.1 `GET /health`
**Purpose:** Liveness/readiness check with DB ping.  
**Auth:** Public.  
**Response 200:**
```json
{ "status": "ok", "uptime": "1h2m3s" }
```
**Response 503:**
```json
{ "status": "unhealthy" }
```

### 1.2 `GET /metrics`
**Purpose:** Exposes in-memory app counters.  
**Auth:** `admin` only (auth + RBAC).  
**Response 200:** JSON map containing keys such as `requests_total`, `orders_created`, `login_success`, `uptime`, etc.

### 1.3 `GET /`
**Purpose:** Root redirect.  
**Auth:** Public.  
**Response:** `302` -> `/dashboard`.

---

## 2. Auth Endpoints

### 2.1 `GET /login`
**Purpose:** Render login page.  
**Auth:** Public.  
**Response:** HTML (`login` template).

### 2.2 `POST /login`
**Purpose:** Authenticate and issue session cookie.  
**Auth:** Public (rate-limited by IP).  
**Form fields:**
- `username` (required)
- `password` (required)

**Success behavior:**
- Sets `session_token` cookie.
- If `must_change_password=1`: redirect to `/account/change-password`.
- Else redirect to `/dashboard`.

**Failure behavior:**
- Status `401` with fixed message "Invalid username or password".
- HTMX: renders `partials/login_form`.
- Non-HTMX: renders full `login` page.

### 2.3 `GET /account/change-password`
**Purpose:** Render forced/manual password change page.  
**Auth:** Any authenticated role.  
**Response:** HTML.

### 2.4 `POST /account/change-password`
**Purpose:** Update password hash and clear `must_change_password`.  
**Auth:** Any authenticated role.  
**Form fields:**
- `new_password`
- `confirm_password`

**Validation:** passwords must match; service also enforces minimum length.  
**Success:** `302` -> `/dashboard`.  
**Failure:** `422` with re-rendered form and error text.

### 2.5 `POST /logout`
**Purpose:** End session.  
**Auth:** Any authenticated role.  
**Behavior:** Best-effort server session deletion, cookie expiration, redirect to `/login`.

---

## 3. Dashboard & Navigation Endpoints

### 3.1 `GET /dashboard`
**Purpose:** Role-based dashboard redirect/router.  
**Auth:** Any authenticated role.  
**Behavior:**
- `admin` -> `/dashboard/admin`
- `instructor` -> `/dashboard/instructor`
- others -> generic `dashboard` page

### 3.2 `GET /history`
**Purpose:** Browsing history page.  
**Auth:** Any authenticated role.  
**Response:** HTML (`history/list`) with last visited materials.

---

## 4. Materials & Engagement Endpoints

### 4.1 `GET /materials`
**Purpose:** Catalog page shell.  
**Auth:** Any authenticated role.

### 4.2 `GET /materials/search`
**Purpose:** HTMX search/list partial.  
**Auth:** Any authenticated role.  
**Query params:**
- `q` (optional)
- `subject` (optional)
- `grade` (optional)
- `page` (optional, default 1)

**Response:** HTML partial `partials/material_cards`.

### 4.3 `GET /materials/:id`
**Purpose:** Material detail page.  
**Auth:** Any authenticated role.  
**Behavior:** records browse history (best effort), loads ratings/comments/favorites-list selector.  
**Errors:** invalid ID -> `400`; not found -> rendered 404-style page.

### 4.4 `POST /materials/:id/rate`
**Purpose:** Submit star rating.  
**Auth:** Any authenticated role.  
**Form fields:** `stars` (1..5).  
**Responses:**
- HTMX success: `partials/star_widget`
- non-HTMX success: redirect to material page
- duplicate rating: `409`

### 4.5 `POST /materials/:id/comments`
**Purpose:** Add comment.  
**Auth:** Any authenticated role + comment rate limit.  
**Form fields:** `body`.  
**Validation/rules:** max length, link-count cap, banned-word filtering, per-user throttling.  
**Responses:**
- HTMX success: refreshed `partials/comments_list`
- failure: `422` and inline form error partial

### 4.6 `POST /comments/:id/report`
**Purpose:** Report comment abuse.  
**Auth:** Any authenticated role.  
**Form fields:** `reason` (optional).  
**Responses:**
- HTMX: `partials/comment_reported`
- non-HTMX: `{ "msg": "Report submitted" }`

### 4.7 Favorites Endpoints
- `GET /favorites`: list current user's favorite lists
- `POST /favorites`: create list (`name`, `visibility`)
- `GET /favorites/:id`: list detail (owner only)
- `GET /favorites/:id/items`: items partial/detail (owner only)
- `POST /favorites/:id/items`: add item (`material_id`)
- `DELETE /favorites/:id/items/:materialID`: remove item
- `GET /favorites/:id/share`: generate share URL
- `GET /share/:token`: public shared-list page

**Share-token behavior:** expired token returns `410 Gone` rendered page.

### 4.8 Admin Material Management
- `GET /admin/materials/new`
- `POST /admin/materials`
- `GET /admin/materials/:id/edit`
- `PUT /admin/materials/:id`
- `DELETE /admin/materials/:id`

**Auth:** `admin` only.  
**Create/Update form fields:** bibliographic fields + `price`, quantities, status.

---

## 5. Orders & Returns Endpoints

### 5.1 User Order Endpoints
- `GET /orders`: current user orders
- `GET /orders/cart`: checkout page
- `GET /orders/:id`: order detail (owner or operational roles)

### 5.2 `POST /orders`
**Purpose:** Place order from form arrays.  
**Auth:** Any authenticated role (typically student).  
**Form arrays:**
- `material_id` (multi)
- `qty` (multi)

**Important rule:** server ignores client price; authoritative price comes from catalog.  
**Failures:** bad cart -> `400`; business-rule failure -> `422`.

### 5.3 `POST /orders/:id/pay`
**Purpose:** Confirm payment.  
**Auth:** Order owner.  
**Success:** HTMX status badge partial or redirect to order detail.

### 5.4 `POST /orders/:id/cancel`
**Purpose:** Cancel own order per role/state rules.  
**Auth:** authenticated; service enforces stricter role/state policy.

### 5.5 Staff Order Endpoints
- `GET /admin/orders` (clerk/admin): list/filter all orders (`status`, `date_from`, `date_to`, `page`)
- `POST /admin/orders/:id/ship` (clerk/admin)
- `POST /admin/orders/:id/deliver` (clerk/admin)
- `POST /admin/orders/:id/cancel` (instructor/manager/admin)

### 5.6 Returns Endpoints
- `POST /orders/:id/returns`: create return/exchange/refund request
- `GET /returns`: current user's return requests
- `GET /admin/returns`: pending returns queue (instructor/manager/admin)
- `POST /admin/returns/:id/approve`
- `POST /admin/returns/:id/reject`

**Return request form fields:**
- `type` (`return|exchange|refund`)
- `reason`
- `replacement_material_id` (optional; exchange)

**Core rules:** completed-order only, 14-day window from `completed_at`, one pending request per order.

---

## 6. Distribution Endpoints

**Auth:** `clerk` or `admin`.

### 6.1 `GET /distribution`
Renders picklist and backorder stats.

### 6.2 `POST /distribution/issue`
**Form fields:**
- `order_id`
- `scan_id`
- `material_id[]`
- `qty[]`
- `issued_qty[]` (optional, partial issue)

Records issue events, fulfills/backorders items, and advances order state when appropriate.

### 6.3 `POST /distribution/return`
**Form fields:** `order_id`, `material_id`, `return_request_id`, `scan_id`, `qty`.

Requires approved return request of type `return`.

### 6.4 `POST /distribution/exchange`
**Form fields:** `order_id`, `old_material_id`, `new_material_id`, `return_request_id`, `scan_id`, `qty`.

Requires approved return request of type `exchange`.

### 6.5 `GET /distribution/reissue`
Render reissue form.

### 6.6 `POST /distribution/reissue`
**Form fields:** `order_id`, `material_id`, `old_scan_id`, `new_scan_id`, `reason`.

### 6.7 Ledger/Custody
- `GET /distribution/ledger`
- `GET /distribution/ledger/search` (HTMX refresh)
- `GET /distribution/custody/:scanID`

**Ledger query filters:** `scan_id`, `event_type`, `date_from`, `date_to`, `material_id`, `actor_id`, `page`.

---

## 7. Inbox & Messaging Endpoints

**Auth:** Any authenticated role.

### 7.1 Inbox pages
- `GET /inbox`
- `GET /inbox/items` (partial)

### 7.2 SSE stream
- `GET /inbox/sse`

Pushes `event: inbox-update` when unread count changes.

### 7.3 Read state endpoints
- `POST /inbox/:id/read`
- `POST /inbox/read-all`

Returns inbox badge partial.

### 7.4 Settings endpoints
- `GET /inbox/settings`
- `POST /inbox/settings/dnd` (`start_hour`, `end_hour` in range 0..23)
- `POST /inbox/subscribe` (`topic`)
- `POST /inbox/unsubscribe` (`topic`)

### 7.5 Badge endpoints
- `GET /inbox/badge`
- `GET /api/inbox/unread-count` (same handler)

Both return unread badge partial.

---

## 8. Moderation Endpoints

**Auth:** `moderator` or `admin`.

- `GET /moderation`
- `GET /moderation/items` (partial)
- `POST /moderation/:id/approve`
- `POST /moderation/:id/remove`

For HTMX approve/remove, success returns empty `200` so row is removed via `hx-swap`.

---

## 9. Courses Endpoints

**Auth:** `instructor`/`manager`/`admin` route group.

- `GET /courses`
- `GET /courses/new`
- `POST /courses`
- `GET /courses/:id`
- `POST /courses/:id/plan`
- `POST /courses/:id/plan/:planID/approve`
- `POST /courses/:id/sections`

### 9.1 Create Course (`POST /courses`)
**Form fields:** `name`, `subject`, `grade_level`, `academic_year`.

### 9.2 Add Plan Item (`POST /courses/:id/plan`)
**Form fields:**
- `material_id` (required int)
- `requested_qty` (required positive int)
- `section_id` (optional int)
- `notes` (optional)

### 9.3 Approve Plan Item
**Form field:** `approved_qty` (required non-negative int).

### 9.4 Add Section
**Form fields:** `name`, `period`, `room`.

---

## 10. Admin Endpoints

**Auth:** `admin` only.

### 10.1 User management
- `GET /admin/users`
- `GET /admin/users/new`
- `POST /admin/users`
- `GET /admin/users/:id`
- `POST /admin/users/:id/role`
- `POST /admin/users/:id/unlock`

### 10.2 Generic custom fields
- `GET /admin/fields/:entity_type/:entity_id`
- `POST /admin/fields/:entity_type/:entity_id`
- `DELETE /admin/fields/:entity_type/:entity_id/:name`

Valid `entity_type`: `user|course|material|location`.

**Set field form fields:**
- `field_name`
- `field_value`
- `encrypt` (`true|1` to enable)
- `reason` (optional; defaults server-side)

### 10.3 Legacy custom field aliases
- `GET /admin/users/:id/fields`
- `POST /admin/users/:id/fields`
- `DELETE /admin/users/:id/fields/:name`

### 10.4 Duplicate detection and merge
- `GET /admin/duplicates`
- `POST /admin/duplicates/merge` with `primary_id`, `duplicate_id`

### 10.5 Audit
- `GET /admin/audit`
- `GET /admin/audit/:entityType/:entityID`

---

## 11. Analytics & Geospatial Endpoints

### 11.1 Dashboard pages
- `GET /dashboard/admin` (`admin`)
- `GET /dashboard/instructor` (`instructor|manager|admin`)

### 11.2 Geospatial pages/APIs (`admin` only)
- `GET /analytics/map` (HTML page)
- `GET /analytics/map/data` (JSON)
- `POST /analytics/map/compute` (JSON)
- `GET /analytics/map/buffer` (JSON)
- `GET /analytics/map/poi-density` (JSON)
- `GET /analytics/map/trajectory/:materialID` (JSON)
- `GET /analytics/map/regions` (JSON)
- `POST /analytics/map/regions/compute` (JSON)

### 11.3 Exports (`admin` only)
- `GET /analytics/export/orders` -> CSV attachment (`orders_export.csv`)
- `GET /analytics/export/distribution` -> CSV attachment (`distribution_export.csv`)

### 11.4 KPI/stat APIs
- `GET /api/stats/:stat` (HTML stat-card partial)
- `GET /analytics/kpi/:name` (JSON KPI history)

`/api/stats/:stat` has additional admin-only stat-name gating (`403` JSON when denied).

---

## 12. Status Models & Enumerations

### 12.1 Order statuses
- `pending_payment`
- `pending_shipment`
- `in_transit`
- `completed`
- `canceled`

### 12.2 Return request types
- `return`
- `exchange`
- `refund`

### 12.3 Return request statuses
- `pending`
- `approved`
- `rejected`

### 12.4 Comment moderation statuses
- `active`
- `collapsed`
- `removed`

### 12.5 Notification topics
- `orders`
- `returns`
- `distribution`
- `announcements`
- `moderation`

---

## Appendix A: Global Middleware Contract

### RequireAuth
- Reads `session_token` cookie.
- Looks up hashed token in sessions table.
- Checks expiry.
- Loads user into request locals.
- On failure: `401 {"error":"unauthorized"}`.

### RequireRole
- Verifies current user role is in allowed set.
- On failure: `403 {"error":"forbidden"}`.

### Login Rate Limit
- `POST /login`: 10 requests/minute per IP.

### Comment Rate Limit
- `POST /materials/:id/comments`: 5 requests/10 minutes per authenticated user.

---

## Appendix B: Response Pattern Summary

- `Render(...)`: HTML full-page or partial contract.
- `Redirect(...)`: browser navigation contract (`302 Found`).
- `c.JSON(...)`: structured API payload contract.
- `c.Send(...)` with CSV headers: file-download contract.

---

*End of API Specification.*
