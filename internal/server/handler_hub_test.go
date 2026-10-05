package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleHubIndex_Empty(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/hub/index", nil)
	rr := httptest.NewRecorder()
	s.handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Skills []any `json:"skills"`
	}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Skills) != 0 {
		t.Errorf("expected 0 skills in hub index, got %d", len(resp.Skills))
	}
}

func TestHandleHubIndex_WithSkills(t *testing.T) {
	s, src := newTestServer(t)
	addSkill(t, src, "hub-skill")

	req := httptest.NewRequest(http.MethodGet, "/api/hub/index", nil)
	rr := httptest.NewRecorder()
	s.handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHubHandlersFollowSourceLinks(t *testing.T) {
	for _, project := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("project=%t/follow=%t", project, enabled), func(t *testing.T) {
				s, source := newUpdateFollowServer(t, project, enabled)
				addSkill(t, source, "healthy")
				checkout := t.TempDir()
				addSkill(t, checkout, "linked-skill")
				if err := os.Symlink(checkout, filepath.Join(source, "linked")); err != nil {
					t.Fatal(err)
				}
				for _, surface := range []string{"index", "drafts", "search"} {
					t.Run(surface, func(t *testing.T) {
						rr := httptest.NewRecorder()
						req := httptest.NewRequest(http.MethodGet, "/api/search?q=linked-skill&hub=@builtin", nil)
						switch surface {
						case "index":
							s.handleHubIndex(rr, req)
						case "drafts":
							s.handleHubDraftCandidates(rr, req)
						case "search":
							s.handleSearch(rr, req)
						}
						if rr.Code != http.StatusOK {
							t.Fatalf("status=%d: %s", rr.Code, rr.Body.String())
						}
						var names []string
						switch surface {
						case "drafts":
							var entries []struct {
								ID string `json:"id"`
							}
							if err := json.Unmarshal(rr.Body.Bytes(), &entries); err != nil {
								t.Fatal(err)
							}
							for _, entry := range entries {
								names = append(names, filepath.Base(entry.ID))
							}
						default:
							var resp struct {
								Skills []struct {
									Name string `json:"name"`
								} `json:"skills"`
								Results []struct {
									Name string `json:"name"`
								} `json:"results"`
							}
							if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
								t.Fatal(err)
							}
							for _, entry := range resp.Skills {
								names = append(names, entry.Name)
							}
							for _, entry := range resp.Results {
								names = append(names, entry.Name)
							}
						}
						found, healthy := false, false
						for _, name := range names {
							found = found || name == "linked-skill"
							healthy = healthy || name == "healthy"
						}
						if found != enabled || (surface != "search" && !healthy) {
							t.Fatalf("follow=%t inventory=%v: %s", enabled, names, rr.Body.String())
						}
					})
				}
			})
		}
	}
}
