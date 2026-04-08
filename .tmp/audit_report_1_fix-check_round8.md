# Prior-Issue Fix Verification (Round 8, Static Re-check)

Source baseline: `.tmp/audit_report_1_fix-check_round7.md`  
Method: static-only code inspection (no app run, no tests executed).

## Summary
- Total prior findings reviewed: **12**
- **Fixed:** 12
- **Partially Fixed:** 0
- **Not Fixed:** 0
- **Cannot Confirm Statistically:** 0 (for issue status)

## Per-Issue Verification

### 1) Schema upsert conflicts for `ON CONFLICT(...)` (Blocker)
- **Round-7 status:** Fixed
- **Round-8 status:** **Fixed (unchanged)**
- **Evidence:**
  - `repo/migrations/001_schema.sql:33-39`
  - `repo/migrations/001_schema.sql:271-279`
  - `repo/internal/repository/admin.go:35-39`
  - `repo/internal/repository/analytics.go:381-384`

### 2) HTMX/template-route contract broken in core flows (Blocker)
- **Round-7 status:** Fixed
- **Round-8 status:** **Fixed (unchanged)**
- **Evidence:**
  - `repo/cmd/server/main.go:301-307`
  - `repo/internal/handlers/materials.go:459-512`
  - `repo/web/templates/favorites/list.html:78-87`

### 3) Duplicate-detection semantics not prompt-compliant (High)
- **Round-7 status:** Fixed
- **Round-8 status:** **Fixed (unchanged)**
- **Evidence:**
  - Dedicated exact-ID and name fields:
    - `repo/migrations/001_schema.sql:18-19`
    - `repo/internal/models/models.go:27-28`
  - Exact-ID matching + fuzzy(name,DOB) scoring:
    - `repo/internal/repository/admin.go:115-117`
    - `repo/internal/repository/admin.go:146-148`
    - `repo/internal/repository/admin.go:121-124`
    - `repo/internal/repository/admin.go:203-217`

### 4) KPI/dashboard below prompt-required metrics (High)
- **Round-7 status:** Fixed
- **Round-8 status:** **Fixed (unchanged)**
- **Evidence:**
  - KPI model coverage:
    - `repo/internal/services/analytics.go:30-43`
  - Dashboard visualization:
    - `repo/web/templates/analytics/admin_dashboard.html:86-182`

### 5) Documented default admin login not statically credible (High)
- **Round-7 status:** Partially Fixed
- **Round-8 status:** **Fixed**
- **What changed:** startup now performs bcrypt verification of documented default password against seeded hash.
- **Evidence:**
  - README credential: `repo/README.md:104-110`
  - Seeded default hash: `repo/migrations/001_schema.sql:351-354`
  - Runtime bcrypt verification path:
    - `repo/cmd/server/main.go:46-53`
    - `repo/cmd/server/main.go:55-63`
    - `repo/cmd/server/main.go:18` (crypto import)

### 6) Favorites add-from-detail posts incompatible payload/endpoint (High)
- **Round-7 status:** Fixed
- **Round-8 status:** **Fixed (unchanged)**
- **Evidence:**
  - `repo/web/templates/materials/detail.html:146-150`
  - `repo/cmd/server/main.go:305`
  - `repo/internal/handlers/materials.go:407-417`

### 7) Funds-adjustment linkage/audit not materially modeled (Medium)
- **Round-7 status:** Fixed
- **Round-8 status:** **Fixed (unchanged)**
- **Evidence:**
  - `repo/migrations/001_schema.sql:318-330`
  - `repo/internal/repository/orders.go:720-750`

### 8) Offline geospatial delivery incomplete (tile/boundary/index consistency) (Medium)
- **Round-7 status:** Fixed
- **Round-8 status:** **Fixed (unchanged)**
- **Evidence:**
  - `repo/web/static/js/map.js:21-25`
  - Tile assets present:
    - `repo/web/static/tiles/0/0/0.png`
    - `repo/web/static/tiles/1/0/0.png`

### 9) Sensitive-word dictionary not loaded by default (Medium)
- **Round-7 status:** Fixed
- **Round-8 status:** **Fixed (unchanged)**
- **Evidence:**
  - `repo/.env.example:7-10`
  - `repo/cmd/server/main.go:85-90`

### 10) Unprotected `/metrics` endpoint (Medium)
- **Round-7 status:** Fixed
- **Round-8 status:** **Fixed (unchanged)**
- **Evidence:**
  - `repo/cmd/server/main.go:228-230`

### 11) Scheduler auto-close note hardcoded to payment timeout (Medium)
- **Round-7 status:** Fixed
- **Round-8 status:** **Fixed (unchanged)**
- **Evidence:**
  - `repo/internal/scheduler/scheduler.go:177-180`

### 12) CSRF protections not evident for cookie-authenticated write routes (Medium / suspected)
- **Round-7 status:** Fixed
- **Round-8 status:** **Fixed (unchanged)**
- **Evidence:**
  - `repo/cmd/server/main.go:262-270`
  - `repo/web/templates/layouts/base.html:205-214`

## Final Round-8 Verdict
- **Overall:** **All previously listed issues are now fixed under static verification.**
