package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/sync"
	"skillshare/internal/trash"
)

func postSourceLink(t *testing.T, s *Server, body sourceLinkRequest) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	s.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/source-links", bytes.NewReader(b)))
	return rr
}

func TestHandleCreateSourceLink_Success(t *testing.T) {
	s, src := newTestServer(t)
	target := t.TempDir()
	rr := postSourceLink(t, s, sourceLinkRequest{Path: target})
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
	}
	var res struct{ Path, Target, Kind, Warning string }
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Path != filepath.Join(src, "_"+filepath.Base(target)) || res.Target != target || res.Kind != "symlink" || res.Warning != "target is not a git checkout" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if got, err := os.Readlink(res.Path); err != nil || got != target {
		t.Fatalf("link = %q, %v", got, err)
	}
	if s.cfg.FollowSourceLinks || s.skillsWalk().Follow != nil {
		t.Fatal("link must not silently enable following")
	}
}

func TestHandleCreateSourceLink_GuardRefusal(t *testing.T) {
	s, src := newTestServer(t)
	for _, tc := range []struct{ path, reason string }{
		{src, "target is the source or a parent of it"},
		{filepath.Join(src, "child"), "target is inside the source"},
		{filepath.Join(t.TempDir(), "missing"), "target is missing"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			rr := postSourceLink(t, s, sourceLinkRequest{Path: tc.path, Name: "_refused", Enable: true})
			if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), tc.reason) {
				t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
			}
			if s.cfg.FollowSourceLinks {
				t.Fatal("refused creation must not enable following")
			}
		})
	}
}

func TestHandleCreateSourceLink_Enable(t *testing.T) {
	for _, project := range []bool{false, true} {
		t.Run(map[bool]string{false: "global", true: "project"}[project], func(t *testing.T) {
			s, src := newTestServer(t)
			if project {
				root := t.TempDir()
				src = t.TempDir()
				pcfg := &config.ProjectConfig{Sources: config.ProjectSources{Skills: src}}
				if err := pcfg.Save(root); err != nil {
					t.Fatal(err)
				}
				s = NewProject(s.cfg, pcfg, root, "127.0.0.1:0", "", "")
			}
			target := t.TempDir()
			addSkill(t, target, "linked-skill")
			rr := postSourceLink(t, s, sourceLinkRequest{Path: target, Name: "_team", Enable: true})
			if rr.Code != http.StatusOK {
				t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
			}
			if s.skillsWalk().Follow == nil {
				t.Fatal("follow snapshot not refreshed")
			}
			skills, err := sync.DiscoverSourceSkills(src, s.skillsWalk())
			if err != nil || len(skills) != 1 {
				t.Fatalf("skills not followed: %v, %v", skills, err)
			}
			if project {
				cfg, err := config.LoadProject(s.projectRoot)
				if err != nil || !cfg.FollowSourceLinks || s.cfg.FollowSourceLinks {
					t.Fatalf("project config not enabled independently: %+v, %v", cfg, err)
				}
			} else {
				cfg, err := config.Load()
				if err != nil || !cfg.FollowSourceLinks {
					t.Fatalf("global config not enabled: %+v, %v", cfg, err)
				}
			}
		})
	}
}

func TestHandleCreateSourceLink_SyncTargetRefusal(t *testing.T) {
	target := t.TempDir()
	s, _ := newTestServerWithTargets(t, map[string]string{"custom": target})
	rr := postSourceLink(t, s, sourceLinkRequest{Path: target})
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "target overlaps sync target "+target) {
		t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleRemoveSourceLink(t *testing.T) {
	s, src := newTestServer(t)
	target := t.TempDir()
	addSkill(t, target, "kept")
	if rr := postSourceLink(t, s, sourceLinkRequest{Path: target, Name: "_team"}); rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}
	rr := httptest.NewRecorder()
	s.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/source-links/_team", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
	}
	if _, err := os.Lstat(filepath.Join(src, "_team")); !os.IsNotExist(err) {
		t.Fatalf("link still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "kept", "SKILL.md")); err != nil {
		t.Fatalf("target modified: %v", err)
	}
	items := trash.List(s.trashBase())
	if len(items) != 1 {
		t.Fatalf("link not trashed: %v", items)
	}
}

func TestHandleRemoveSourceLink_NonLink(t *testing.T) {
	s, src := newTestServer(t)
	addSkill(t, src, "plain")
	rr := httptest.NewRecorder()
	s.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/source-links/plain", nil))
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "plain is not a link") {
		t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(src, "plain", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
