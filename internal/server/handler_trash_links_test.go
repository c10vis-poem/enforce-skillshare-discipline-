package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/trash"
)

func TestHandleRestoreTrash_SourceLinkFollowPolicy(t *testing.T) {
	for _, project := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			for _, linkName := range []string{"_dev-skills", "dev-skills"} {
				name := map[bool]string{false: "global", true: "project"}[project] + "/" + map[bool]string{false: "disabled", true: "enabled"}[enabled] + "/" + linkName
				t.Run(name, func(t *testing.T) {
					s, src := newTestServer(t)
					if project {
						s.cfg.FollowSourceLinks = !enabled
						src = t.TempDir()
						s = NewProject(s.cfg, &config.ProjectConfig{
							Sources: config.ProjectSources{Skills: src}, FollowSourceLinks: enabled,
						}, t.TempDir(), "127.0.0.1:0", "", "")
					} else {
						s.cfg.FollowSourceLinks = enabled
					}
					if err := s.saveConfig(); err != nil {
						t.Fatal(err)
					}
					target := t.TempDir()
					addSkill(t, target, "foo")
					addSkill(t, target, "bar")
					link := filepath.Join(src, linkName)
					if err := os.Symlink(target, link); err != nil {
						t.Fatal(err)
					}
					logicalName := linkName + "/foo"
					if _, err := trash.MoveToTrash(filepath.Join(target, "foo"), logicalName, s.trashBase()); err != nil {
						t.Fatal(err)
					}
					rr := httptest.NewRecorder()
					s.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/trash/"+url.PathEscape(logicalName)+"/restore", nil))
					if enabled {
						if rr.Code != http.StatusOK {
							t.Fatalf("restore failed: %d: %s", rr.Code, rr.Body.String())
						}
						if _, err := os.Stat(filepath.Join(target, "foo", "SKILL.md")); err != nil {
							t.Fatalf("skill not restored to link target: %v", err)
						}
						if entry := trash.FindByName(s.trashBase(), logicalName); entry != nil {
							t.Fatalf("restored skill still in trash: %+v", entry)
						}
					} else {
						var resp struct{ Error string }
						if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
							t.Fatal(err)
						}
						want := "failed to restore: restore path unsafe: " + link + " is a link; edit its target directly"
						if rr.Code != http.StatusInternalServerError || resp.Error != want {
							t.Fatalf("expected link refusal %q, got %d: %s", want, rr.Code, rr.Body.String())
						}
						if _, err := os.Stat(filepath.Join(target, "foo")); !os.IsNotExist(err) {
							t.Fatalf("refused restore modified target: %v", err)
						}
						entry := trash.FindByName(s.trashBase(), logicalName)
						if entry == nil {
							t.Fatal("refused restore lost trash entry")
						}
						if _, err := os.Stat(filepath.Join(entry.Path, "SKILL.md")); err != nil {
							t.Fatalf("refused restore lost trashed skill: %v", err)
						}
					}
					if got, err := os.Readlink(link); err != nil || got != target {
						t.Fatalf("source link changed: %q, %v", got, err)
					}
					if _, err := os.Stat(filepath.Join(target, "bar", "SKILL.md")); err != nil {
						t.Fatalf("sibling skill changed: %v", err)
					}
				})
			}
		}
	}
}
