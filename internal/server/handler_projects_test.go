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
)

func projectRequest(t *testing.T, s *Server, method, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	s.handler.ServeHTTP(rr, httptest.NewRequest(method, url, strings.NewReader(body)))
	return rr
}

func addProject(t *testing.T, s *Server, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	body := `{"create":true,"root":"` + root + `","targets":["claude","codex"],"skills":{"mode":"copy","include":["team-*"],"exclude":[]}}`
	if rr := projectRequest(t, s, http.MethodPut, "/api/projects", body); rr.Code != http.StatusOK {
		t.Fatalf("add project: %d %s", rr.Code, rr.Body)
	}
}

func TestListProjects_DetectsVisibleProjectConfig(t *testing.T) {
	s, src := newTestServer(t)
	root := filepath.Join(filepath.Dir(src), "app")
	addProject(t, s, root)
	if err := os.MkdirAll(filepath.Join(root, "skillshare"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skillshare", "config.yaml"), []byte("targets:\n  - claude\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var listed struct {
		Projects []projectItem `json:"projects"`
	}
	json.Unmarshal(projectRequest(t, s, http.MethodGet, "/api/projects", "").Body.Bytes(), &listed)
	if len(listed.Projects) != 1 || !listed.Projects[0].HasOwnConfig {
		t.Fatalf("projects %+v, want hasOwnConfig for the visible project dir", listed.Projects)
	}
}

func TestSaveProject_AddsTargetsThatOnlyTheProjectScopeLists(t *testing.T) {
	s, src := newTestServer(t)
	root := filepath.Join(filepath.Dir(src), "app")
	addProject(t, s, root)

	var listed struct {
		Targets []targetItem `json:"targets"`
	}
	json.Unmarshal(projectRequest(t, s, http.MethodGet, "/api/targets?scope=projects", "").Body.Bytes(), &listed)
	if len(listed.Targets) != 2 || listed.Targets[0].Project != root {
		t.Fatalf("project targets %+v", listed.Targets)
	}
	json.Unmarshal(projectRequest(t, s, http.MethodGet, "/api/targets", "").Body.Bytes(), &listed)
	if len(listed.Targets) != 0 {
		t.Fatalf("own targets %+v", listed.Targets)
	}
}

func TestSaveProject_Rejected(t *testing.T) {
	s, src := newTestServer(t)
	root := filepath.Join(filepath.Dir(src), "app")
	addProject(t, s, root)
	for name, body := range map[string]string{
		"already declared": `{"create":true,"root":"` + root + `","targets":["claude"]}`,
		"folder not found": `{"create":true,"root":"` + root + `-gone","targets":["claude"]}`,
		"unknown target":   `{"root":"` + root + `","targets":["nope"]}`,
		"invalid mode":     `{"root":"` + root + `","targets":["claude"],"skills":{"mode":"mirror"}}`,
	} {
		if rr := projectRequest(t, s, http.MethodPut, "/api/projects", body); rr.Code == http.StatusOK {
			t.Errorf("%s: saved", name)
		}
	}
}

func TestProjectTargets_CannotBeEditedAsTargets(t *testing.T) {
	s, src := newTestServer(t)
	addProject(t, s, filepath.Join(filepath.Dir(src), "app"))

	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		if rr := projectRequest(t, s, method, "/api/targets/app@claude", `{"mode":"merge"}`); rr.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", method, rr.Code, rr.Body)
		}
	}
}

func TestRemoveProject_DropsItsTargets(t *testing.T) {
	s, src := newTestServer(t)
	root := filepath.Join(filepath.Dir(src), "app")
	addProject(t, s, root)

	if rr := projectRequest(t, s, http.MethodDelete, "/api/projects?root="+root, ""); rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body)
	}
	if len(s.cfg.Projects) != 0 || len(s.cfg.Targets) != 0 {
		t.Fatalf("projects %v, targets %v", s.cfg.Projects, s.cfg.Targets)
	}
}

func TestConvertProject_MovesTargetsUnderProjectsWithTheSameFolders(t *testing.T) {
	root := filepath.Join(t.TempDir(), "legacy")
	skills := filepath.Join(root, ".claude", "skills")
	s, _ := newTestServerWithTargets(t, map[string]string{"old-claude": skills})

	if rr := projectRequest(t, s, http.MethodPost, "/api/projects/convert", `{"root":"`+root+`"}`); rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body)
	}
	if _, kept := s.cfg.Targets["old-claude"]; kept {
		t.Error("old-claude is still a target")
	}
	converted := s.cfg.Targets[config.ProjectTargetName("legacy", "claude")]
	if got := converted.SkillsConfig().Path; got != skills {
		t.Errorf("skills path %q, want %q", got, skills)
	}
}
