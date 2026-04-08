# Recheck of Prior Findings from `.tmp/audit_report_2_fix-check_round1.md`

Date: 2026-04-08  
Method: Static-only verification (no app run, no tests executed)

## Overall Recheck Verdict
- Fixed: 9
- Partially Fixed: 0
- Not Fixed: 0

## Per-Issue Status

1) **Real environment secrets committed in repository**  
- Previous status: Partially Fixed  
- Status now: **Fixed**  
- Evidence:
  - `.env` now uses placeholders, not real secret values: `repo/.env:3-4`
  - `.env` remains ignored: `repo/.gitignore:8`
- Note: historical Git exposure/rotation cannot be confirmed from static snapshot alone.

2) **Admin/instructor cancel action pointed to non-existent endpoint**  
- Previous status: Fixed  
- Status now: **Fixed**  
- Evidence:
  - Route present: `repo/cmd/server/main.go:387`
  - Handler present: `repo/internal/handlers/orders.go:200-222`
  - Template endpoint matches: `repo/web/templates/orders/detail.html:119`, `repo/web/templates/orders/detail.html:161`

3) **Partial fulfillment and backorder split missing**  
- Previous status: Fixed  
- Status now: **Fixed**  
- Evidence:
  - Backorder path implemented for partial issue: `repo/internal/services/distribution.go:121-150`
  - Covered by service tests: `repo/internal/services/distribution_test.go:153-231`

4) **Exchange approval did not enforce inventory availability**  
- Previous status: Fixed  
- Status now: **Fixed**  
- Evidence:
  - Exchange approval enforces replacement stock check: `repo/internal/services/orders.go:238-249`
  - Tests validate fail/success paths: `repo/internal/services/orders_test.go:400-452`

5) **Instructor/course planning replaced by approximation**  
- Previous status: Partially Fixed  
- Status now: **Fixed**  
- Evidence:
  - `course_plans` now includes section dimension: `repo/migrations/001_schema.sql:359-363`
  - Plan item flow accepts `section_id`: `repo/internal/handlers/courses.go:124-132`
  - Repository stores `section_id`: `repo/internal/repository/courses.go:128-139`
  - UI supports section selection: `repo/web/templates/courses/detail.html:95-104`
  - Analytics comment/query now state and use non-approximate course-plan joins: `repo/internal/repository/analytics.go:288-312`

6) **Role-sensitive KPI stat endpoint lacked authorization granularity**  
- Previous status: Fixed  
- Status now: **Fixed**  
- Evidence:
  - Admin-only stat allowlist remains enforced: `repo/internal/handlers/analytics.go:200-205`, `repo/internal/handlers/analytics.go:214-218`

7) **Encrypted custom field could be stored plaintext if key invalid/missing**  
- Previous status: Fixed  
- Status now: **Fixed**  
- Evidence:
  - Encryption request fails on invalid key length: `repo/internal/services/admin.go:90-93`
  - Config validation requires secrets: `repo/internal/config/config.go:34-43`
  - Validation called at startup: `repo/cmd/server/main.go:32-38`

8) **Browse-history resume capability not exposed to users**  
- Previous status: Fixed  
- Status now: **Fixed**  
- Evidence:
  - Route exists: `repo/cmd/server/main.go:300-301`
  - Handler exists: `repo/internal/handlers/materials.go:587-604`
  - Template exists: `repo/web/templates/history/list.html:1-40`

9) **Integration tests permissive (allowing weak outcomes)**  
- Previous status: Not Fixed  
- Status now: **Fixed**  
- Evidence:
  - Test policy now requires strict 200 for full-page success (500 explicitly unacceptable): `repo/internal/integration/helpers_test.go:13-15`
  - Inbox notification test now asserts exact 200: `repo/internal/integration/messaging_test.go:57-60`
  - Mark-all-read no longer weak auth-only assertion; checks success codes explicitly: `repo/internal/integration/messaging_test.go:138-140`

## Final Notes
- This update rechecks only the issues listed in `.tmp/audit_report_2_fix-check_round1.md`.
- Runtime behavior remains outside static proof boundary.
