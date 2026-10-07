//go:build !online

package integration

import (
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/testutil"
)

// addPendingTrackedRepo clones a tracked repo into sourceDir/rel and pushes one
// more commit to its remote, so an update changes my-skill/SKILL.md.
func addPendingTrackedRepo(t *testing.T, sb *testutil.Sandbox, sourceDir, rel string) string {
	t.Helper()
	repoPath := filepath.Join(sourceDir, filepath.FromSlash(rel))
	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	run(t, "", "git", "init", "--bare", remoteDir)
	run(t, sb.Root, "git", "clone", remoteDir, repoPath)
	sb.WriteFile(filepath.Join(repoPath, "my-skill", "SKILL.md"), "---\nname: my-skill\n---\n# Clean")
	run(t, repoPath, "git", "add", "-A")
	run(t, repoPath, "git", "commit", "-m", "init")
	run(t, repoPath, "git", "push", "origin", "HEAD")

	workDir := filepath.Join(t.TempDir(), "work")
	run(t, sb.Root, "git", "clone", remoteDir, workDir)
	sb.WriteFile(filepath.Join(workDir, "my-skill", "SKILL.md"), "---\nname: my-skill\n---\n# Updated clean")
	run(t, workDir, "git", "add", "-A")
	run(t, workDir, "git", "commit", "-m", "update")
	run(t, workDir, "git", "push", "origin", "HEAD")
	return repoPath
}

// `update org/team` for a repo installed with --into org (org/_team) must
// update that repo, never a top-level _team that sits next to it.
func assertShorthandUpdatesNestedRepo(t *testing.T, sb *testutil.Sandbox, sourceDir string, update func(name string) *testutil.Result) {
	t.Helper()
	nested := addPendingTrackedRepo(t, sb, sourceDir, "org/_team")
	top := addPendingTrackedRepo(t, sb, sourceDir, "_team")

	update("org/team").AssertSuccess(t)

	if got := sb.ReadFile(filepath.Join(nested, "my-skill", "SKILL.md")); !strings.Contains(got, "Updated clean") {
		t.Errorf("org/_team was not updated, got %q", got)
	}
	if got := sb.ReadFile(filepath.Join(top, "my-skill", "SKILL.md")); strings.Contains(got, "Updated clean") {
		t.Errorf("top-level _team was updated by the org/team shorthand")
	}
}

func TestUpdate_ShorthandIntoRepo(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()
	setupGlobalConfig(sb)

	assertShorthandUpdatesNestedRepo(t, sb, sb.SourcePath, func(name string) *testutil.Result {
		return sb.RunCLI("update", name)
	})
}

func TestUpdateProject_ShorthandIntoRepo(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()
	projectRoot := sb.SetupProjectDir("claude")

	assertShorthandUpdatesNestedRepo(t, sb, filepath.Join(projectRoot, ".skillshare", "skills"), func(name string) *testutil.Result {
		return sb.RunCLIInDir(projectRoot, "update", name, "-p")
	})
}
