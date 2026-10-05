//go:build !online

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/testutil"
)

// Target links to skills behind an unavailable source link are kept by sync,
// so doctor must not call them broken or suggest pruning them.
func TestDoctor_FollowSourceLinks_UnavailableLinkIsNotBroken(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	checkout := filepath.Join(sb.Root, "code", "dev-skills")
	sb.WriteFile(filepath.Join(checkout, "foo", "SKILL.md"), "---\nname: foo\n---\n# foo")
	if err := os.MkdirAll(filepath.Join(checkout, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	sb.CreateSymlink(checkout, filepath.Join(sb.SourcePath, "_dev-skills"))
	target := sb.CreateTarget("claude")
	sb.WriteConfig(`source: ` + sb.SourcePath + `
follow_source_links: true
targets:
  claude:
    path: ` + target + `
`)
	sb.RunCLI("sync").AssertSuccess(t)
	if err := os.Rename(checkout, checkout+".moved"); err != nil {
		t.Fatal(err)
	}

	result := sb.RunCLI("doctor")
	result.AssertSuccess(t)
	result.AssertOutputContains(t, "behind an unavailable source link, kept until it is back: _dev-skills__foo")
	result.AssertOutputNotContains(t, "broken symlink")
	result.AssertOutputNotContains(t, "prune the broken links")

	jsonResult := sb.RunCLI("doctor", "--json")
	jsonResult.AssertSuccess(t)
	found := false
	for _, check := range parseDoctorJSON(t, jsonResult.Stdout).Checks {
		if check.Name != "broken_symlinks" {
			continue
		}
		found = true
		if check.Status != "warning" {
			t.Errorf("want a warning, got %+v", check)
		}
	}
	if !found {
		t.Error("broken_symlinks check missing")
	}
}
