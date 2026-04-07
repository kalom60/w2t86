# System Design Questions – District Materials Commerce & Logistics Portal (Production-Ready)

---

## 1. User Authentication & Access Control

- **Question:** How should users authenticate and how are roles enforced?
- **Assumption:** Fully on-prem, no external identity provider.
- **Solution:**
  - Use session-based auth (secure cookies) with **bcrypt/Argon2 password hashing**
  - Implement **RBAC middleware** in Fiber
  - Store `failed_attempts` and `locked_until` for brute-force protection
  - Encrypt sensitive fields using **AES-256-GCM**

---

## 2. SQLite Concurrency & Write Management

- **Question:** How do we avoid "database is locked" errors under concurrent usage?
- **Assumption:** High concurrent reads, moderate writes (semester peaks).
- **Solution:**
  - Enable **WAL mode**
  - Configure `_busy_timeout`
  - Use **single-writer pattern** (write queue or limited pool)
  - Separate read/write DB access layer in repository

---

## 3. Order Lifecycle State Machine

- **Question:** How are order states enforced and validated?
- **Assumption:** Strict transitions required.
- **Solution:**
  - Implement **state machine pattern** in service layer
  - Validate transitions via domain rules
  - Persist **state transition logs (audit trail)**

---

## 4. Order Expiry & Background Workers

- **Question:** How are auto-cancel rules enforced (30min / 72hr)?
- **Assumption:** Must survive restarts and run automatically.
- **Solution:**
  - Create **worker service** using `time.Ticker`
  - Periodically:
    - find expired orders
    - cancel + rollback inventory in transaction
  - Structure as **worker module (fits your Go architecture)**

---

## 5. Inventory Reservation & Consistency

- **Question:** How do we prevent overselling?
- **Assumption:** Inventory must be reserved during checkout.
- **Solution:**
  - Use **ACID transactions**
  - Maintain `reserved_quantity`
  - Rollback on failure or expiry
  - All inventory updates happen inside domain service

---

## 6. Partial Fulfillment & Backorders

- **Question:** How are unavailable items handled?
- **Assumption:** Orders may be split.
- **Solution:**
  - Use **parent-child order model**
  - Track `parent_order_id`
  - Maintain backorder queue
  - Allow reissue workflows

---

## 7. Distribution Tracking (Ledger System)

- **Question:** How do we track material custody?
- **Assumption:** Must be traceable per individual.
- **Solution:**
  - Append-only **distribution ledger**
  - Scan/type identifiers
  - Immutable event records
  - Link to user + order

---

## 8. Real-Time Notifications (HTMX-Friendly)

- **Question:** How do we push updates without heavy frontend frameworks?
- **Assumption:** HTMX-based UI.
- **Solution:**
  - Use **Server-Sent Events (SSE)**
  - Fiber endpoint streams events
  - HTMX SSE extension updates UI via partial swaps

---

## 9. Unified Inbox & DND Logic

- **Question:** How do we manage notifications with Do-Not-Disturb?
- **Assumption:** DND configurable per user.
- **Solution:**
  - Store notifications in DB
  - Add `deliver_after` field
  - Worker delivers only eligible messages
  - Track read/unread state

---

## 10. Anti-Spam & Rate Limiting

- **Question:** How to enforce comment limits efficiently?
- **Assumption:** Must be fast and lightweight.
- **Solution:**
  - Implement **sliding window rate limiter**
  - Use in-memory cache (LRU) or indexed DB query
  - Return HTTP `429` with HTMX-friendly response

---

## 11. Sensitive Word Filtering

- **Question:** How to filter inappropriate content offline?
- **Assumption:** Local dictionary only.
- **Solution:**
  - Load dictionary into **Trie (prefix tree)**
  - Perform linear scan on input text
  - Avoid regex-heavy operations

---

## 12. Favorites, Sharing & Secure Links

- **Question:** How to generate expiring, permission-safe links?
- **Assumption:** Links must not expose internal IDs.
- **Solution:**
  - Use **HMAC-signed URLs**
  - Include expiry timestamp
  - Validate signature + permissions on access

---

## 13. Duplicate Detection & Entity Merging

- **Question:** How to detect duplicate profiles?
- **Assumption:** Data may be inconsistent.
- **Solution:**
  - Exact match (ID)
  - Fuzzy match using **Levenshtein / Soundex**
  - Admin merge tool with conflict resolution
  - Store merge audit history

---

## 14. Extensible Data Model

- **Question:** How to support custom fields?
- **Assumption:** Schema changes should be minimized.
- **Solution:**
  - Use **JSON columns in SQLite**
  - Validate in service layer
  - Query via `json_extract`

---

## 15. Analytics & Dashboard Performance

- **Question:** How to compute KPIs efficiently?
- **Assumption:** Large datasets degrade performance.
- **Solution:**
  - Use **materialized summary tables**
  - Background job updates aggregates
  - Dashboard reads precomputed data

---

## 16. Geospatial Analytics (Offline)

- **Question:** How to support spatial queries without internet?
- **Assumption:** Local map data available.
- **Solution:**
  - Use **SpatiaLite (SQLite extension)**
  - R-Tree indexes
  - Serve tiles locally via Fiber
  - Return GeoJSON for rendering

---

## 17. Auditing & Change History

- **Question:** How to track all changes?
- **Assumption:** Full traceability required.
- **Solution:**
  - Use **temporal tables pattern**
  - Store previous state before update
  - Include `who/when/why`

---

## 18. Inventory & Financial Consistency

- **Question:** How to ensure inventory and receipts stay consistent?
- **Assumption:** Must never diverge.
- **Solution:**
  - Wrap operations in **single transaction**
  - Fail = full rollback
  - Link ledger entries

---

## 19. HTMX API Design Strategy

- **Question:** How should frontend and backend communicate?
- **Assumption:** HTMX drives UI.
- **Solution:**
  - Return **HTML partials** for most endpoints
  - Use JSON only for charts
  - Keep handlers thin → services handle logic

---

## 20. System Modularity (Hexagonal Architecture)

- **Question:** How should the system be structured for maintainability?
- **Assumption:** Long-term extensibility required.
- **Solution:**
  - Use **hexagonal architecture**
    - Handlers (HTTP)
    - Services (business logic)
    - Repositories (DB)
  - Workers as separate modules
  - Clear domain boundaries (orders, inventory, messaging)
