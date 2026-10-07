package server

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/testutil"
)

// addPendingTrackedRepo clones a tracked repo into src/rel and pushes one more
// commit to its remote, so an update moves HEAD. It returns the checkout path.
func addPendingTrackedRepo(t *testing.T, src, rel string) string {
	t.Helper()
	base := t.TempDir()
	remote := testutil.SetupBareRemoteRepo(t, base)
	testutil.SeedRemoteBranch(t, base, remote, "main", map[string]string{"child/SKILL.md": trackedCleanSkill})
	repo := filepath.Join(src, filepath.FromSlash(rel))
	testutil.RunGit(t, "", "clone", remote, repo)
	seed := filepath.Join(base, "seed-main")
	if err := os.WriteFile(filepath.Join(seed, "child", "SKILL.md"), []byte(trackedCleanSkill+"More help.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.RunGit(t, seed, "add", "-A")
	testutil.RunGit(t, seed, "commit", "-m", "update")
	testutil.RunGit(t, seed, "push", "origin", "HEAD:main")
	return repo
}

// `update org/team` for a repo installed with --into org (org/_team) must
// update that repo and leave a top-level _team alone, on both dashboard routes.
func TestHandleUpdate_ShorthandIntoRepo(t *testing.T) {
	for _, route := range []struct {
		name   string
		stream bool
	}{{"single", false}, {"stream", true}} {
		t.Run(route.name, func(t *testing.T) {
			s, src := newTestServer(t)
			nested := addPendingTrackedRepo(t, src, "org/_team")
			top := addPendingTrackedRepo(t, src, "_team")
			topBefore := testutil.RunGit(t, top, "rev-parse", "HEAD")
			nestedBefore := testutil.RunGit(t, nested, "rev-parse", "HEAD")

			results := runFollowUpdate(t, s, route.stream, "org/team")

			if len(results) != 1 || results[0].Name != "org/_team" || results[0].Action != "updated" {
				t.Fatalf("results = %+v, want org/_team updated", results)
			}
			if got := testutil.RunGit(t, nested, "rev-parse", "HEAD"); got == nestedBefore {
				t.Error("org/_team was not updated")
			}
			if got := testutil.RunGit(t, top, "rev-parse", "HEAD"); got != topBefore {
				t.Error("top-level _team was updated by the org/team shorthand")
			}
		})
	}
}

// A skill that exists at org/team is what the name refers to; the shorthand
// must not send the update to org/_team instead.
func TestHandleUpdateStream_ExistingSkillBeatsShorthand(t *testing.T) {
	s, src := newTestServer(t)
	addSkill(t, src, "org/team")
	addTrackedRepo(t, src, "org/_team")

	results := runFollowUpdate(t, s, true, "org/team")

	if len(results) != 1 || results[0].Name != "org/team" || results[0].IsRepo {
		t.Fatalf("results = %+v, want the org/team skill, not the org/_team repo", results)
	}
}

// `org/team/` names the same skill as `org/team`, so the exact-skill lookup
// must see it before the shorthand can pick org/_team.
func TestHandleUpdate_TrailingSlashKeepsExistingSkill(t *testing.T) {
	s, src := newTestServer(t)
	addSkill(t, src, "org/team")
	addSkillMeta(t, src, "org/team", "/nonexistent/skill-source")
	addTrackedRepo(t, src, "org/_team")
	s.reloadSkillsStore() // pick up the metadata written above

	results := runFollowUpdate(t, s, false, "org/team/")

	if len(results) != 1 || results[0].IsRepo {
		t.Fatalf("results = %+v, want the org/team skill, not the org/_team repo", results)
	}
}
