package api_tests

import (
	"net/http"
	"testing"
)

// account_test.go covers GET/POST /account/change-password.

// TestAccount_ChangePasswordPage_ReturnsOK checks the change-password form renders.
func TestAccount_ChangePasswordPage_ReturnsOK(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	resp := makeRequest(app, http.MethodGet, "/account/change-password", "", cookie, "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("expected 2xx for change-password page, got %d; body: %s", resp.StatusCode, readBody(resp))
	}
}

// TestAccount_ChangePasswordPage_Unauthenticated returns 401 or 302.
func TestAccount_ChangePasswordPage_Unauthenticated(t *testing.T) {
	app, _, cleanup := newTestApp(t)
	defer cleanup()

	resp := makeRequest(app, http.MethodGet, "/account/change-password", "", "", "")
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 401/302, got %d", resp.StatusCode)
	}
}

// TestAccount_ChangePassword_Valid submits matching passwords and expects redirect.
func TestAccount_ChangePassword_Valid(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	body := "new_password=NewSecure456%21&confirm_password=NewSecure456%21"
	resp := makeRequest(app, http.MethodPost, "/account/change-password", body, cookie,
		"application/x-www-form-urlencoded")
	if resp.StatusCode != http.StatusFound && resp.StatusCode/100 != 2 {
		t.Fatalf("expected 302/2xx on valid password change, got %d; body: %s",
			resp.StatusCode, readBody(resp))
	}
}

// TestAccount_ChangePassword_Mismatch rejects non-matching passwords.
func TestAccount_ChangePassword_Mismatch(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	body := "new_password=NewSecure456%21&confirm_password=DifferentPass789%21"
	resp := makeRequest(app, http.MethodPost, "/account/change-password", body, cookie,
		"application/x-www-form-urlencoded")
	if resp.StatusCode == http.StatusFound {
		t.Fatalf("expected non-redirect for mismatched passwords, got %d", resp.StatusCode)
	}
}

// TestAccount_ChangePassword_TooShort rejects passwords that fail the min-length check.
func TestAccount_ChangePassword_TooShort(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	cookie := loginAs(t, app, db, "student")
	body := "new_password=short&confirm_password=short"
	resp := makeRequest(app, http.MethodPost, "/account/change-password", body, cookie,
		"application/x-www-form-urlencoded")
	if resp.StatusCode == http.StatusFound {
		t.Fatalf("expected non-redirect for too-short password, got %d", resp.StatusCode)
	}
}
