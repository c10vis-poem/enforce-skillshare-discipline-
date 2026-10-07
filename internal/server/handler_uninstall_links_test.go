package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/trash"
	"skillshare/internal/utils"
)

func uninstallLinkedSkill(t *testing.T, s *Server, name string, batch bool) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	rr := httptest.NewRecorder()
	if !batch {
		s.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/resources/"+name, nil))
		return rr, rr.Code == http.StatusOK
	}
	body, err := json.Marshal(batchUninstallRequest{Names: []string{name}})
	if err != nil {
		t.Fatal(err)
	}
	s.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/uninstall/batch", bytes.NewReader(body)))
	var resp struct {
		Results []batchUninstallItemResult `json:"results"`
		Summary batchUninstallSummary      `json:"summary"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode batch response: %v: %s", err, rr.Body.String())
	}
	return rr, rr.Code == http.StatusOK && resp.Summary.Succeeded == 1 && resp.Summary.Failed == 0 && len(resp.Results) == 1 && resp.Results[0].Success && resp.Results[0].MovedToTrash
}

func testUninstallFollowedLink(t *testing.T, batch bool) {
	for _, project := range []bool{false, true} {
		t.Run(map[bool]string{false: "global", true: "project"}[project], func(t *testing.T) {
			s, src := newTestServer(t)
			if project {
				src = t.TempDir()
				s = NewProject(s.cfg, &config.ProjectConfig{
					Sources: config.ProjectSources{Skills: src}, FollowSourceLinks: true,
				}, t.TempDir(), "127.0.0.1:0", "", "")
			} else {
				s.cfg.FollowSourceLinks = true
			}
			if err := s.saveConfig(); err != nil {
				t.Fatal(err)
			}
			target := t.TempDir()
			for _, name := range []string{"foo", "bar", "baz"} {
				addSkill(t, target, name)
			}
			for _, args := range [][]string{
				{"init", "-q"}, {"add", "."},
				{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-qm", "Seed skills"},
			} {
				cmd := exec.Command("git", args...)
				cmd.Dir = target
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
			}
			link := filepath.Join(src, "_dev-skills")
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			rr, success := uninstallLinkedSkill(t, s, "_dev-skills__foo", batch)
			if !success {
				t.Fatalf("uninstall failed: %d: %s", rr.Code, rr.Body.String())
			}
			if _, err := os.Stat(filepath.Join(target, "foo")); !os.IsNotExist(err) {
				t.Fatalf("selected skill remains: %v", err)
			}
			for _, name := range []string{"bar", "baz"} {
				if _, err := os.Stat(filepath.Join(link, name, "SKILL.md")); err != nil {
					t.Fatalf("other skill %s changed: %v", name, err)
				}
			}
			if got, err := os.Readlink(link); err != nil || got != target {
				t.Fatalf("source link changed: %q, %v", got, err)
			}
			cmd := exec.Command("git", "status", "--porcelain")
			cmd.Dir = target
			if out, err := cmd.CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "D foo/SKILL.md" {
				t.Fatalf("checkout changes: %s, %v", out, err)
			}
			items := trash.List(s.trashBase())
			if len(items) != 1 || items[0].Name != "_dev-skills/foo" {
				t.Fatalf("expected only selected skill with its logical restore path: %+v", items)
			}
			if _, err := os.Stat(filepath.Join(items[0].Path, "SKILL.md")); err != nil {
				t.Fatalf("selected skill not in trash: %v", err)
			}
			if err := trash.Restore(&items[0], src, s.skillsWalk().Follow); err != nil {
				t.Fatalf("restore to linked folder: %v", err)
			}
			if _, err := os.Stat(filepath.Join(target, "foo", "SKILL.md")); err != nil {
				t.Fatalf("skill not restored to checkout: %v", err)
			}
		})
	}
}

func TestHandleUninstallSkill_FollowedLink(t *testing.T) {
	testUninstallFollowedLink(t, false)
}

func TestHandleUninstallSkill_FollowedLinkRootRefused(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, project := range []bool{false, true} {
			for _, name := range []string{"_dev", "dev"} {
				t.Run(map[bool]string{false: "single", true: "batch"}[batch]+"/"+map[bool]string{false: "global", true: "project"}[project]+"/"+name, func(t *testing.T) {
					s, src := newTestServer(t)
					if project {
						src = t.TempDir()
						s = NewProject(s.cfg, &config.ProjectConfig{
							Sources: config.ProjectSources{Skills: src}, FollowSourceLinks: true,
						}, t.TempDir(), "127.0.0.1:0", "", "")
					} else {
						s.cfg.FollowSourceLinks = true
					}
					if err := s.saveConfig(); err != nil {
						t.Fatal(err)
					}
					target := t.TempDir()
					addSkill(t, target, ".")
					addSkill(t, target, "child")
					link := filepath.Join(src, name)
					if err := os.Symlink(target, link); err != nil {
						t.Fatal(err)
					}
					rr, success := uninstallLinkedSkill(t, s, name, batch)
					if success || (!batch && rr.Code != http.StatusBadRequest) || !strings.Contains(rr.Body.String(), name+" is the linked folder itself; use unlink to remove the link") {
						t.Fatalf("expected linked root refusal: %d: %s", rr.Code, rr.Body.String())
					}
					if !utils.IsSymlinkOrJunction(link) {
						t.Fatal("link was removed")
					}
					for _, rel := range []string{"SKILL.md", "child/SKILL.md"} {
						if _, err := os.Stat(filepath.Join(link, filepath.FromSlash(rel))); err != nil {
							t.Fatalf("skill disappeared through link: %s: %v", rel, err)
						}
					}
					if items := trash.List(s.trashBase()); len(items) != 0 {
						t.Fatalf("linked root moved to trash: %+v", items)
					}
				})
			}
		}
	}
}

func TestHandleUninstallSkill_ExactNameBeforeLinkedBasename(t *testing.T) {
	s, src := newTestServer(t)
	addSkill(t, src, "foo")
	target := t.TempDir()
	addSkill(t, target, "foo")
	if err := os.Symlink(target, filepath.Join(src, "_dev")); err != nil {
		t.Fatal(err)
	}
	s.cfg.FollowSourceLinks = true
	if err := s.saveConfig(); err != nil {
		t.Fatal(err)
	}
	if rr, ok := uninstallLinkedSkill(t, s, "foo", false); !ok {
		t.Fatalf("root uninstall failed: %d: %s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(src, "foo")); !os.IsNotExist(err) {
		t.Fatalf("root skill remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "foo", "SKILL.md")); err != nil {
		t.Fatalf("checkout skill modified: %v", err)
	}
	if rr, ok := uninstallLinkedSkill(t, s, "_dev__foo", false); !ok {
		t.Fatalf("linked uninstall failed: %d: %s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(target, "foo")); !os.IsNotExist(err) {
		t.Fatalf("linked skill remains: %v", err)
	}
	for _, name := range []string{"foo", "_dev/foo"} {
		if entry := trash.FindByName(s.trashBase(), name); entry == nil {
			t.Fatalf("missing trash entry: %s", name)
		}
	}
}

func TestHandleUninstallSkill_AmbiguousLinkedBasename(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "batch"}[batch], func(t *testing.T) {
			s, src := newTestServer(t)
			for _, name := range []string{"_a", "_b"} {
				target := t.TempDir()
				addSkill(t, target, "foo")
				if err := os.Symlink(target, filepath.Join(src, name)); err != nil {
					t.Fatal(err)
				}
			}
			s.cfg.FollowSourceLinks = true
			if err := s.saveConfig(); err != nil {
				t.Fatal(err)
			}
			rr, ok := uninstallLinkedSkill(t, s, "foo", batch)
			if ok || (!batch && rr.Code != http.StatusBadRequest) || !strings.Contains(rr.Body.String(), "ambiguous skill name") {
				t.Fatalf("expected ambiguity refusal: %d: %s", rr.Code, rr.Body.String())
			}
			for _, name := range []string{"_a", "_b"} {
				if !strings.Contains(rr.Body.String(), name+"__foo") {
					t.Fatalf("ambiguity missing flat name %s: %s", name, rr.Body.String())
				}
				if _, err := os.Stat(filepath.Join(src, name, "foo", "SKILL.md")); err != nil {
					t.Fatalf("ambiguous skill modified: %v", err)
				}
			}
			if items := trash.List(s.trashBase()); len(items) != 0 {
				t.Fatalf("ambiguous skill trashed: %+v", items)
			}
		})
	}
}

func TestHandleBatchUninstall_RepoInstalledInto(t *testing.T) {
	s, src := newTestServer(t)
	addTrackedRepo(t, src, "org/_team")
	addSkill(t, filepath.Join(src, "org", "_team"), "foo")
	_, res := postBatchUninstall(t, s, true, "org/_team")
	if _, err := os.Stat(filepath.Join(src, "org", "_team")); !os.IsNotExist(err) {
		t.Fatalf("expected org/_team to be uninstalled, stat err %v: %+v", err, res)
	}
}

func TestHandleBatchUninstall_GitDirInsideTrackedRepoIsKept(t *testing.T) {
	s, src := newTestServer(t)
	addTrackedRepo(t, src, "_team")
	addTrackedRepo(t, src, "_team/vendor/_lib") // e.g. a submodule checkout
	_, res := postBatchUninstall(t, s, true, "_team/vendor/_lib")
	if len(res) != 1 || res[0].Success {
		t.Fatalf("expected the request to be refused: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(src, "_team", "vendor", "_lib", ".git")); err != nil {
		t.Fatalf("folder inside a tracked repo was removed: %v", err)
	}
}

func TestHandleBatchUninstall_NestedRepoBehindUnfollowedLinkIsKept(t *testing.T) {
	s, src := newTestServer(t)
	outside := t.TempDir()
	addTrackedRepo(t, outside, "_evil")
	if err := os.Symlink(outside, filepath.Join(src, "org")); err != nil {
		t.Fatal(err)
	}
	_, res := postBatchUninstall(t, s, true, "org/_evil")
	if len(res) != 1 || res[0].Success {
		t.Fatalf("expected the request to be refused: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(outside, "_evil", ".git")); err != nil {
		t.Fatalf("repo outside the source was removed through an unfollowed link: %v", err)
	}
}

func TestHandleBatchUninstall_RepoBeforeLinkedBasename(t *testing.T) {
	s, src := newTestServer(t)
	addTrackedRepo(t, src, "_team")
	addSkill(t, filepath.Join(src, "_team"), "foo")
	target := t.TempDir()
	addSkill(t, target, "_team")
	if err := os.Symlink(target, filepath.Join(src, "_dev")); err != nil {
		t.Fatal(err)
	}
	s.cfg.FollowSourceLinks = true
	if err := s.saveConfig(); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(batchUninstallRequest{Names: []string{"_team"}, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	s.handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/uninstall/batch", bytes.NewReader(body)))
	var resp struct {
		Summary batchUninstallSummary `json:"summary"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusOK || resp.Summary.Succeeded != 1 || resp.Summary.Failed != 0 {
		t.Fatalf("repo uninstall failed: %d: %s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(src, "_team")); !os.IsNotExist(err) {
		t.Fatalf("managed repo remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "_team", "SKILL.md")); err != nil {
		t.Fatalf("checkout skill modified: %v", err)
	}
	entry := trash.FindByName(s.trashBase(), "_team")
	if entry == nil {
		t.Fatal("managed repo not in trash")
	}
	if _, err := os.Stat(filepath.Join(entry.Path, "foo", "SKILL.md")); err != nil {
		t.Fatalf("managed repo content missing from trash: %v", err)
	}
}

func TestHandleBatchUninstall_FollowedLink(t *testing.T) {
	testUninstallFollowedLink(t, true)
}

func TestHandleUninstallSkill_TrackedRepoMemberRefused(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "batch"}[batch], func(t *testing.T) {
			s, src := newTestServer(t)
			addTrackedRepo(t, src, "_repo")
			addSkill(t, filepath.Join(src, "_repo"), "foo")
			s.cfg.FollowSourceLinks = true
			if err := s.saveConfig(); err != nil {
				t.Fatal(err)
			}
			rr, success := uninstallLinkedSkill(t, s, "foo", batch)
			if success || !strings.Contains(rr.Body.String(), "tracked repo") {
				t.Fatalf("expected tracked repo refusal: %d: %s", rr.Code, rr.Body.String())
			}
			if _, err := os.Stat(filepath.Join(src, "_repo", "foo", "SKILL.md")); err != nil {
				t.Fatalf("tracked skill modified: %v", err)
			}
			if items := trash.List(s.trashBase()); len(items) != 0 {
				t.Fatalf("refused skill trashed: %+v", items)
			}
		})
	}
}

func TestHandleUninstallSkill_SkippedLinkRefused(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, reason := range []string{"disabled", "unavailable", "sync-target"} {
			t.Run(map[bool]string{false: "single", true: "batch"}[batch]+"/"+reason, func(t *testing.T) {
				s, src := newTestServer(t)
				target := t.TempDir()
				addSkill(t, target, "foo")
				linkTarget := target
				if reason == "unavailable" {
					linkTarget = filepath.Join(target, "missing")
				}
				if err := os.Symlink(linkTarget, filepath.Join(src, "_dev-skills")); err != nil {
					t.Fatal(err)
				}
				s.cfg.FollowSourceLinks = reason != "disabled"
				if reason == "sync-target" {
					s.cfg.Targets["custom"] = config.TargetConfig{Path: target}
				}
				if err := s.saveConfig(); err != nil {
					t.Fatal(err)
				}
				rr, success := uninstallLinkedSkill(t, s, "_dev-skills__foo", batch)
				if success {
					t.Fatalf("skipped link uninstalled: %d: %s", rr.Code, rr.Body.String())
				}
				if _, err := os.Stat(filepath.Join(target, "foo", "SKILL.md")); err != nil {
					t.Fatalf("skipped target modified: %v", err)
				}
				if got, err := os.Readlink(filepath.Join(src, "_dev-skills")); err != nil || got != linkTarget {
					t.Fatalf("skipped link changed: %q, %v", got, err)
				}
				if items := trash.List(s.trashBase()); len(items) != 0 {
					t.Fatalf("skipped skill trashed: %+v", items)
				}
			})
		}
	}
}
