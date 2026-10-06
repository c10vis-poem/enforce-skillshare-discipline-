package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/install"
)

func TestHandleCheck_EmptySource(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/check", nil)
	rr := httptest.NewRecorder()
	s.handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		TrackedRepos []any `json:"tracked_repos"`
		Skills       []any `json:"skills"`
	}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.TrackedRepos) != 0 {
		t.Errorf("expected 0 tracked repos, got %d", len(resp.TrackedRepos))
	}
	if len(resp.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(resp.Skills))
	}
}

// addTrackedRepoWithBrokenIndex clones a tracked repo from a local origin and
// corrupts its index, so `git status` fails while `git fetch` still succeeds.
func addTrackedRepoWithBrokenIndex(t *testing.T, sourceDir, name string) string {
	t.Helper()
	seed := t.TempDir()
	initGitRepo(t, seed)
	origin := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, seed, "clone", "--bare", seed, origin)
	runGit(t, sourceDir, "clone", origin, name)
	repoDir := filepath.Join(sourceDir, name)
	// fetch reads the index for submodule recursion unless it is disabled.
	runGit(t, repoDir, "config", "fetch.recurseSubmodules", "false")
	if err := os.WriteFile(filepath.Join(repoDir, ".git", "index"), []byte("corrupt"), 0644); err != nil {
		t.Fatalf("corrupt index: %v", err)
	}
	return repoDir
}

func TestHandleCheck_DirtyCheckErrorReported(t *testing.T) {
	s, src := newTestServer(t)
	addTrackedRepoWithBrokenIndex(t, src, "_team")

	req := httptest.NewRequest(http.MethodGet, "/api/check", nil)
	rr := httptest.NewRecorder()
	s.handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		TrackedRepos []repoCheckResult `json:"tracked_repos"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.TrackedRepos) != 1 || resp.TrackedRepos[0].Status != "error" {
		t.Fatalf("expected one repo with status error, got %+v", resp.TrackedRepos)
	}
}

func TestHandleCheck_ProjectRelativeLocalSourceIsCompared(t *testing.T) {
	s, _ := newTestServer(t)
	projectRoot := t.TempDir()
	skillsDir := filepath.Join(projectRoot, ".skillshare", "skills")
	os.MkdirAll(skillsDir, 0755)
	s.projectRoot = projectRoot
	s.projectCfg = &config.ProjectConfig{}
	if err := s.projectCfg.Save(projectRoot); err != nil {
		t.Fatal(err)
	}

	vendor := filepath.Join(projectRoot, "vendor", "team-skill")
	os.MkdirAll(vendor, 0755)
	os.WriteFile(filepath.Join(vendor, "SKILL.md"), []byte("---\nname: team-skill\n---\n# v1"), 0644)
	// Project installs record the source as typed, relative to the project root.
	source := &install.Source{Type: install.SourceTypeLocalPath, Raw: "./vendor/team-skill", Path: vendor, Name: "team-skill"}
	if _, err := install.Install(source, filepath.Join(skillsDir, "team-skill"), install.InstallOptions{SourceDir: skillsDir, SkipAudit: true}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(vendor, "SKILL.md"), []byte("---\nname: team-skill\n---\n# v2"), 0644)

	rr := httptest.NewRecorder()
	s.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/check", nil))

	if !strings.Contains(rr.Body.String(), `"status":"update_available"`) {
		t.Errorf("expected update_available for the relative source, got %s", rr.Body.String())
	}
}
