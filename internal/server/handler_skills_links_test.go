package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"skillshare/internal/config"
)

func TestHandleListSkills_SourceLinkIdentity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		follow   bool
		missing  bool
		refused  bool
		repo     bool
		linkName string
	}{
		{name: "followed", follow: true},
		{name: "followed_repo", follow: true, repo: true},
		{name: "nonrepo_underscore", follow: true, linkName: "_team"},
		{name: "following_off"},
		{name: "unavailable", follow: true, missing: true},
		{name: "refused_sync_target", follow: true, refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, src := newTestServer(t)
			addSkill(t, src, "plain/alpha")
			target := t.TempDir()
			if tc.repo {
				if out, err := exec.Command("git", "init", target).CombinedOutput(); err != nil {
					t.Fatalf("git init: %v: %s", err, out)
				}
			}
			if tc.missing {
				target = filepath.Join(target, "missing")
			} else {
				addSkill(t, target, "nested/beta")
				addSkill(t, target, "gamma")
			}
			// A relative link proves that the API reports the resolved policy target.
			rel, err := filepath.Rel(src, target)
			if err != nil {
				t.Fatal(err)
			}
			linkName := tc.linkName
			if linkName == "" {
				linkName = "team"
			}
			if err := os.Symlink(rel, filepath.Join(src, linkName)); err != nil {
				t.Fatal(err)
			}
			s.cfg.FollowSourceLinks = tc.follow
			if tc.refused {
				s.cfg.Targets["custom"] = config.TargetConfig{Path: target}
			}
			if err := s.saveConfig(); err != nil {
				t.Fatal(err)
			}
			rr := httptest.NewRecorder()
			s.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/resources?kind=skill", nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("got %d: %s", rr.Code, rr.Body.String())
			}
			var res struct {
				Resources []map[string]any `json:"resources"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
				t.Fatal(err)
			}
			linked := 0
			for _, skill := range res.Resources {
				if skill["relPath"] == "plain/alpha" {
					if _, ok := skill["linkName"]; ok {
						t.Fatal("plain sub-folder reported as a link")
					}
					if _, ok := skill["linkTarget"]; ok {
						t.Fatal("plain sub-folder has a link target")
					}
					if _, ok := skill["linkIsRepo"]; ok {
						t.Fatal("plain sub-folder has link repo metadata")
					}
					continue
				}
				if skill["linkName"] != linkName || skill["linkTarget"] != target {
					t.Fatalf("incorrect link identity: %+v", skill)
				}
				isRepo, _ := skill["linkIsRepo"].(bool)
				if isRepo != tc.repo {
					t.Fatalf("link repo flag %v, want %v", skill["linkIsRepo"], tc.repo)
				}
				linked++
			}
			want := 0
			if tc.follow && !tc.missing && !tc.refused {
				want = 2
			}
			if linked != want || len(res.Resources) != want+1 {
				t.Fatalf("got %d linked / %d resources, want %d linked plus plain folder", linked, len(res.Resources), want)
			}
		})
	}
}
