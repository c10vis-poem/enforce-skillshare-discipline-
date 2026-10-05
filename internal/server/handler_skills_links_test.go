package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/config"
)

func TestHandleListSkills_SourceLinkIdentity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		follow  bool
		missing bool
		refused bool
	}{
		{name: "followed", follow: true},
		{name: "following_off"},
		{name: "unavailable", follow: true, missing: true},
		{name: "refused_sync_target", follow: true, refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, src := newTestServer(t)
			addSkill(t, src, "plain/alpha")
			target := t.TempDir()
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
			if err := os.Symlink(rel, filepath.Join(src, "team")); err != nil {
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
					continue
				}
				if skill["linkName"] != "team" || skill["linkTarget"] != target {
					t.Fatalf("incorrect link identity: %+v", skill)
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
