package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
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

func TestHandleGetSkill_FollowedLinkRootFiles(t *testing.T) {
	for _, project := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("project=%t/follow=%t", project, enabled), func(t *testing.T) {
				s, source := newUpdateFollowServer(t, project, enabled)
				checkout := t.TempDir()
				if err := os.MkdirAll(filepath.Join(checkout, "assets"), 0755); err != nil {
					t.Fatal(err)
				}
				for name, content := range map[string]string{"SKILL.md": "# Linked root skill\n", "assets/notes.txt": "attachment\n"} {
					if err := os.WriteFile(filepath.Join(checkout, name), []byte(content), 0644); err != nil {
						t.Fatal(err)
					}
				}
				link := filepath.Join(source, "_dev-skills")
				if err := os.Symlink(checkout, link); err != nil {
					t.Fatal(err)
				}
				rr := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/api/resources/_dev-skills", nil)
				req.SetPathValue("name", "_dev-skills")
				s.handleGetSkill(rr, req)
				if !enabled {
					if rr.Code != http.StatusNotFound {
						t.Fatalf("disabled link discovered: %d %s", rr.Code, rr.Body.String())
					}
					return
				}
				var resp struct {
					Resource skillItem `json:"resource"`
					Files    []string  `json:"files"`
					Content  string    `json:"skillMdContent"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatal(err)
				}
				if rr.Code != http.StatusOK || !slices.Equal(resp.Files, []string{"SKILL.md", "assets/notes.txt"}) {
					t.Fatalf("root-of-link files missing: %d %s", rr.Code, rr.Body.String())
				}
				if resp.Resource.SourcePath != link || resp.Resource.RelPath != "_dev-skills" || resp.Content != "# Linked root skill\n" {
					t.Fatalf("logical skill response changed: %+v", resp)
				}
			})
		}
	}
}
