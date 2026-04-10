package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// rawRequest fires a request WITHOUT the automatic CSRF injection that
// makeRequest provides.  Use this to verify that CSRF protection rejects
// requests that lack a valid token.
func rawRequest(app interface {
	Test(req *http.Request, msTimeout ...int) (*http.Response, error)
}, method, path, body, cookie string) *http.Response {
	var br io.Reader
	if body != "" {
		br = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, br)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	// Deliberately NO X-Csrf-Token header — we are testing CSRF rejection.
	resp, err := app.Test(req, -1)
	if err != nil {
		return &http.Response{StatusCode: http.StatusInternalServerError}
	}
	return resp
}

// TestCSRF_RejectsRequestsWithoutToken is a rejection matrix that verifies
// that POST, PUT, and DELETE requests to high-value routes are blocked with
// 403 Forbidden when no CSRF token is supplied.
//
// This guards against cross-site request forgery on state-changing operations
// such as order placement, payment confirmation, role changes, and admin
// actions.
func TestCSRF_RejectsRequestsWithoutToken(t *testing.T) {
	app, db, cleanup := newTestApp(t)
	defer cleanup()

	studentCookie := loginAs(t, app, db, "student")
	adminCookie := loginAs(t, app, db, "admin")

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		cookie string
	}{
		// Student-facing state-changing routes.
		{"POST /orders (place order)", http.MethodPost, "/orders", "material_id=1&qty=1", studentCookie},
		{"POST /orders/1/pay (confirm payment)", http.MethodPost, "/orders/1/pay", "", studentCookie},
		{"POST /orders/1/cancel (cancel order)", http.MethodPost, "/orders/1/cancel", "", studentCookie},
		{"POST /orders/1/returns (submit return)", http.MethodPost, "/orders/1/returns", "type=return&reason=x", studentCookie},
		{"POST /inbox/1/read (mark read)", http.MethodPost, "/inbox/1/read", "", studentCookie},
		{"POST /inbox/read-all (mark all read)", http.MethodPost, "/inbox/read-all", "", studentCookie},

		// Admin state-changing routes.
		{"POST /admin/users (create user)", http.MethodPost, "/admin/users", "username=x&email=x@x.com&password=P@ssw0rd!&role=student", adminCookie},
		{"POST /admin/users/1/role (update role)", http.MethodPost, "/admin/users/1/role", "role=admin", adminCookie},
		{"POST /admin/users/1/fields (set custom field)", http.MethodPost, "/admin/users/1/fields", "name=k&value=v", adminCookie},
		{"DELETE /admin/users/1/fields/k (delete field)", http.MethodDelete, "/admin/users/1/fields/k", "", adminCookie},
		{"POST /admin/materials (create material)", http.MethodPost, "/admin/materials", "title=x&total_qty=1", adminCookie},
		{"DELETE /admin/materials/1 (delete material)", http.MethodDelete, "/admin/materials/1", "", adminCookie},

		// Clerk routes.
		{"POST /distribution/issue", http.MethodPost, "/distribution/issue", "order_id=1&scan_id=X&material_id=1&qty=1", loginAs(t, app, db, "clerk")},
	}

	for _, tc := range cases {
		tc := tc // capture range var
		t.Run(tc.name, func(t *testing.T) {
			resp := rawRequest(app, tc.method, tc.path, tc.body, tc.cookie)
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s %s without CSRF token: expected 403 Forbidden, got %d",
					tc.method, tc.path, resp.StatusCode)
			}
		})
	}
}
