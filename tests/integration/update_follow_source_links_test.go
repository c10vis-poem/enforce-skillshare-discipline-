//go:build !online

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/testutil"
)

// A followed link is the user's own checkout, so update --all must leave it
// alone even with --force, which would otherwise reset their local changes.
func TestUpdateAll_FollowSourceLinks_SkipsFollowedCheckout(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	checkout := filepath.Join(sb.Root, "code", "dev-skills")
	sb.WriteFile(filepath.Join(checkout, "foo", "SKILL.md"), "---\nname: foo\n---\n# foo")
	testutil.RunGit(t, checkout, "init", "-q")
	testutil.ConfigureGitUser(t, checkout)
	testutil.RunGit(t, checkout, "add", "-A")
	testutil.RunGit(t, checkout, "commit", "-qm", "init")
	edited := filepath.Join(checkout, "foo", "SKILL.md")
	sb.WriteFile(edited, "---\nname: foo\n---\n# foo, local edit")
	sb.CreateSymlink(checkout, filepath.Join(sb.SourcePath, "_dev-skills"))
	sb.WriteConfig("source: " + sb.SourcePath + "\nfollow_source_links: true\ntargets: {}\n")

	result := sb.RunCLI("update", "--all", "--force", "--skip-audit")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "_dev-skills: followed source link, not updated by --all")

	data, err := os.ReadFile(edited)
	if err != nil || string(data) != "---\nname: foo\n---\n# foo, local edit" {
		t.Fatalf("local edit was lost: %q, %v", data, err)
	}
}
