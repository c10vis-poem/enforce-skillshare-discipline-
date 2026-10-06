// Package checktest seeds a skills source whose skills cover every outcome of
// update-status resolution, backed by local bare remotes instead of a network.
package checktest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"skillshare/internal/install"
	"skillshare/internal/testutil"
)

// InstalledAt is the install date recorded for the "current" and "plain" skills.
var InstalledAt = time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)

// Seed installs one skill per resolution outcome into sourceDir and returns
// each skill's recorded version. Every skill's source is "src/<name>".
//
//	current  pinned branch, version equals the remote ref     up_to_date
//	same     HEAD moved, subdir tree unchanged                up_to_date
//	slash    like same, subdir recorded with a leading "/"    up_to_date
//	changed  HEAD moved, subdir tree changed                  update_available
//	notree   HEAD moved, no recorded tree hash                update_available
//	doomed   HEAD moved, subdir removed upstream              stale
//	broken   remote cannot be reached                         error
//	plain    no remote                                        local
func Seed(t *testing.T, root, sourceDir string) map[string]string {
	t.Helper()

	movedBase := filepath.Join(root, "moved")
	moved := testutil.SetupBareRemoteRepo(t, movedBase)
	testutil.SeedRemoteBranch(t, movedBase, moved, "main", map[string]string{
		"skills/same/SKILL.md":    "# same\n",
		"skills/changed/SKILL.md": "# v1\n",
		"skills/doomed/SKILL.md":  "# doomed\n",
	})
	seed := filepath.Join(movedBase, "seed-main")
	old := testutil.RunGit(t, seed, "rev-parse", "--short=7", "HEAD")
	tree := func(subdir string) string {
		return testutil.RunGit(t, seed, "rev-parse", "HEAD:"+subdir)
	}
	sameTree, changedTree, doomedTree := tree("skills/same"), tree("skills/changed"), tree("skills/doomed")
	if err := os.WriteFile(filepath.Join(seed, "skills", "changed", "SKILL.md"), []byte("# v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(seed, "skills", "doomed")); err != nil {
		t.Fatal(err)
	}
	testutil.RunGit(t, seed, "add", "-A")
	testutil.RunGit(t, seed, "commit", "-m", "move HEAD")
	testutil.RunGit(t, seed, "push", "origin", "HEAD:main")

	pinnedBase := filepath.Join(root, "pinned")
	pinned := testutil.SetupBareRemoteRepo(t, pinnedBase)
	testutil.SeedRemoteBranch(t, pinnedBase, pinned, "main", map[string]string{"SKILL.md": "# current\n"})
	head := testutil.RunGit(t, filepath.Join(pinnedBase, "seed-main"), "rev-parse", "--short=7", "HEAD")

	movedURL := "file://" + moved
	entries := map[string]*install.MetadataEntry{
		"current": {RepoURL: "file://" + pinned, Branch: "main", Version: head, InstalledAt: InstalledAt},
		"same":    {RepoURL: movedURL, Version: old, TreeHash: sameTree, Subdir: "skills/same"},
		"slash":   {RepoURL: movedURL, Version: old, TreeHash: sameTree, Subdir: "/skills/same"},
		"changed": {RepoURL: movedURL, Version: old, TreeHash: changedTree, Subdir: "skills/changed"},
		"notree":  {RepoURL: movedURL, Version: old, Subdir: "skills/same"},
		"doomed":  {RepoURL: movedURL, Version: old, TreeHash: doomedTree, Subdir: "skills/doomed"},
		"broken":  {RepoURL: "file://" + filepath.Join(root, "missing.git"), Version: "0000000"},
		"plain":   {Type: "local", Version: "v1", InstalledAt: InstalledAt},
	}

	store := install.LoadMetadataOrNew(sourceDir)
	versions := make(map[string]string, len(entries))
	for name, entry := range entries {
		entry.Source = "src/" + name
		store.Set(name, entry)
		versions[name] = entry.Version
		dir := filepath.Join(sourceDir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Save(sourceDir); err != nil {
		t.Fatal(err)
	}
	return versions
}
