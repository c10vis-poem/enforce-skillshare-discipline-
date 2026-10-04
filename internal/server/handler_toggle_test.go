package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleDisableSkill_OverriddenByLocalNegation_Conflict(t *testing.T) {
	s, src := newTestServer(t)
	seedLocalNegation(t, src)

	req := httptest.NewRequest(http.MethodPost, "/api/resources/feature-radar__feature-radar/disable", nil)
	rr := httptest.NewRecorder()
	s.handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), ".skillignore.local") {
		t.Fatalf("expected 409 naming .skillignore.local, got %d: %s", rr.Code, rr.Body.String())
	}
}
