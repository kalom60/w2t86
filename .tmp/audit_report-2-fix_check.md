# Recheck Report: `audit_report-2-fix_check_round1.md`

Date: 2026-04-13
Input: `./.tmp/audit_report-2-fix_check_round1.md`
Method: static code review + targeted tests

## Overall Verdict

**Pass — all previously reported issues are fixed in the current codebase.**

Compared with round 1, the former comment-only inconsistency is now corrected.

## Issue-by-Issue Verification

### 1) High — Refund approval authorization deviated from prompt "manager role" constraint

**Status: Fixed**

Evidence:
- Route guard includes `manager` explicitly: `repo/cmd/server/main.go` (`RequireRole("instructor", "manager", "admin")`), and is applied to:
  - `GET /admin/returns`
  - `POST /admin/returns/:id/approve`
  - `POST /admin/returns/:id/reject`
- Service-layer approval check accepts manager explicitly:
  - `repo/internal/services/orders.go` (`ApproveReturn` allows `admin`, `instructor`, `manager`)
- README documents manager semantics and aliasing:
  - `repo/README.md` (Available Roles + Manager role note)

Assessment:
- Manager-role requirement is implemented in both route and service layers, and docs align.

### 2) High — Tests codified incorrect refund-role policy

**Status: Fixed**

Evidence:
- API permissions include explicit manager coverage:
  - `repo/API_tests/permissions_test.go` (`TestPermission_AdminReturns_ManagerAllowed`)
- Negative coverage remains for disallowed roles:
  - `TestPermission_AdminReturns_ClerkForbidden`
  - `TestPermission_AdminReturns_StudentForbidden`
  - `TestPermission_AdminReturns_ModeratorForbidden`
  - `TestPermission_ApproveReturn_ClerkForbidden`
  - `TestPermission_ApproveReturn_StudentForbidden`

Assessment:
- Tests now enforce intended manager/instructor/admin policy rather than legacy-only wording.

### 3) Medium — DND pinned to UTC semantics

**Status: Fixed**

Evidence:
- Timezone-aware DND support exists:
  - `repo/internal/repository/messaging.go` (`SetTimezone`, timezone-backed `IsInDND` evaluation)
- Startup applies configured timezone:
  - `repo/cmd/server/main.go` (`time.LoadLocation(cfg.Timezone)` + `messagingRepo.SetTimezone(loc)`)
- Config + docs expose `TIMEZONE`:
  - `repo/internal/config/config.go`
  - `repo/README.md` (Environment Variables)

Assessment:
- UTC-only behavior is removed; DND evaluation is deployment-configurable.

### 4) Medium — README lacked clear non-Docker test guidance and had stale structure map

**Status: Fixed**

Evidence:
- README includes dedicated section:
  - `repo/README.md` → `## Running Tests (no Docker)` with package and single-test examples.
- Structure section reflects current modules/files.

Assessment:
- Documentation gap is resolved.

### 5) Medium — Seeded known admin credential risk

**Status: Fixed**

Evidence:
- Seed uses non-functional placeholder:
  - `repo/migrations/001_schema.sql` (`BOOTSTRAP_PENDING_ROTATION`)
- Startup rotates bootstrap/legacy credential to random password and enforces password change:
  - `repo/cmd/server/main.go` (auto-rotation + `must_change_password = 1`)
- Credential-hardening tests are present:
  - `repo/internal/repository/admin_credential_test.go`
- Round-1 stale migration comment has now been corrected:
  - `repo/migrations/009_must_change_password.sql` now matches bootstrap-rotation behavior.

Assessment:
- Original security risk and the prior documentation inconsistency are both resolved.

## Targeted Test Execution (Executed)

All passed with `-tags sqlite_fts5`:

- `go test -tags sqlite_fts5 ./API_tests -run 'AdminReturns|ApproveReturn'`
- `go test -tags sqlite_fts5 ./internal/integration -run 'ApproveReturn'`
- `go test -tags sqlite_fts5 ./internal/services -run 'UpdateDND|Send_'`
- `go test -tags sqlite_fts5 ./internal/repository -run 'AdminSeed|AutoRotation'`

## Final Recheck Summary

- Fixed: 5/5 issues
- Partially fixed: 0/5
- Open critical issues: 0
