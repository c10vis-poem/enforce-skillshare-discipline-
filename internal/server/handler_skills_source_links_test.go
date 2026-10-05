package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleListSkills_SourceLinkWarnings(t *testing.T) {
	for _, project := range []bool{false, true} {
		t.Run(fmt.Sprintf("project=%t", project), func(t *testing.T) {
			s, source := newUpdateFollowServer(t, project, true)
			addSkill(t, source, "healthy")
			if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), filepath.Join(source, "gone")); err != nil {
				t.Fatal(err)
			}
			rr := httptest.NewRecorder()
			s.handleListSkills(rr, httptest.NewRequest(http.MethodGet, "/api/resources", nil))
			var resp struct {
				Resources []skillItem `json:"resources"`
				Warnings  []string    `json:"sourceLinkWarnings"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if len(resp.Resources) != 1 || resp.Resources[0].Name != "healthy" {
				t.Fatalf("healthy inventory lost: %s", rr.Body.String())
			}
			if len(resp.Warnings) != 1 || !strings.Contains(resp.Warnings[0], "source link gone not followed: target is missing") {
				t.Fatalf("missing warning: %s", rr.Body.String())
			}
		})
	}
}
