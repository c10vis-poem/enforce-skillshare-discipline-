//go:build !online

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/testutil"
)

// linkSandbox holds a checkout with one skill outside the source.
func linkSandbox(t *testing.T) (*testutil.Sandbox, string) {
	t.Helper()
	sb := testutil.NewSandbox(t)
	checkout := filepath.Join(sb.Root, "code", "dev-skills")
	sb.WriteFile(filepath.Join(checkout, "foo", "SKILL.md"), "---\nname: foo\n---\n# foo")
	return sb, checkout
}

func TestLink_EnableThenListShowsLinkedSkills(t *testing.T) {
	sb, checkout := linkSandbox(t)
	defer sb.Cleanup()
	sb.WriteConfig("source: " + sb.SourcePath + "\ntargets: {}\n")

	result := sb.RunCLI("link", checkout, "--enable")
	result.AssertSuccess(t)
	result.AssertOutputContains(t, "Linked _dev-skills")
	if !sb.IsSymlink(filepath.Join(sb.SourcePath, "_dev-skills")) {
		t.Fatal("link not created")
	}
	if got := sb.ReadFile(sb.ConfigPath); !contains(got, "follow_source_links: true") {
		t.Fatalf("config not enabled:\n%s", got)
	}
	sb.RunCLI("list", "--json").AssertOutputContains(t, `"relPath": "_dev-skills/foo"`)
}

func TestLink_WithoutEnablePrintsDoctorHint(t *testing.T) {
	sb, checkout := linkSandbox(t)
	defer sb.Cleanup()
	sb.WriteConfig("source: " + sb.SourcePath + "\ntargets: {}\n")

	result := sb.RunCLI("link", checkout, "--name", "_mine")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "_mine: not followed by discovery; its contents are invisible to skillshare. Set follow_source_links: true to follow it")
	if contains(sb.ReadFile(sb.ConfigPath), "follow_source_links") {
		t.Fatal("config changed without --enable")
	}
}

func TestLink_RefusesTargetInsideSource(t *testing.T) {
	sb, _ := linkSandbox(t)
	defer sb.Cleanup()
	sb.WriteConfig("source: " + sb.SourcePath + "\ntargets: {}\n")
	sb.CreateSkill("local", map[string]string{"SKILL.md": "---\nname: local\n---\n# local"})

	result := sb.RunCLI("link", filepath.Join(sb.SourcePath, "local"))
	result.AssertFailure(t)
	result.AssertAnyOutputContains(t, "target is inside the source")
	result = sb.RunCLI("link", filepath.Dir(sb.SourcePath), "--name", "_up")
	result.AssertFailure(t)
	result.AssertAnyOutputContains(t, "target is the source or a parent of it")
	if sb.FileExists(filepath.Join(sb.SourcePath, "_local")) || sb.FileExists(filepath.Join(sb.SourcePath, "_up")) {
		t.Fatal("refused link was created")
	}
}

func TestLink_RefusesTargetOverlappingSyncTarget(t *testing.T) {
	sb, _ := linkSandbox(t)
	defer sb.Cleanup()
	target := sb.CreateTarget("claude")
	sb.WriteConfig("source: " + sb.SourcePath + "\ntargets:\n  claude:\n    path: " + target + "\n")

	result := sb.RunCLI("link", filepath.Dir(target), "--name", "_claude")
	result.AssertFailure(t)
	result.AssertAnyOutputContains(t, "target overlaps sync target")
	if sb.FileExists(filepath.Join(sb.SourcePath, "_claude")) {
		t.Fatal("refused link was created")
	}
}

func TestLink_WarnsWhenTargetIsNotCheckout(t *testing.T) {
	sb, checkout := linkSandbox(t)
	defer sb.Cleanup()
	sb.WriteConfig("source: " + sb.SourcePath + "\nfollow_source_links: true\ntargets: {}\n")

	result := sb.RunCLI("link", checkout)
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "target is not a git checkout")

	if err := os.Mkdir(filepath.Join(checkout, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	result = sb.RunCLI("link", checkout, "--name", "_again")
	result.AssertSuccess(t)
	result.AssertOutputNotContains(t, "not a git checkout")
}

func TestUnlink_RemovesOnlyTheLink(t *testing.T) {
	sb, checkout := linkSandbox(t)
	defer sb.Cleanup()
	sb.WriteConfig("source: " + sb.SourcePath + "\nfollow_source_links: true\ntargets: {}\n")
	sb.RunCLI("link", checkout).AssertSuccess(t)

	sb.RunCLI("unlink", "_dev-skills").AssertSuccess(t)
	if _, err := os.Lstat(filepath.Join(sb.SourcePath, "_dev-skills")); !os.IsNotExist(err) {
		t.Fatalf("link still present: %v", err)
	}
	if !sb.FileExists(filepath.Join(checkout, "foo", "SKILL.md")) {
		t.Fatal("unlink touched the target")
	}
}

func TestUnlink_RefusesPlainDirectory(t *testing.T) {
	sb, _ := linkSandbox(t)
	defer sb.Cleanup()
	sb.WriteConfig("source: " + sb.SourcePath + "\ntargets: {}\n")
	sb.CreateSkill("local", map[string]string{"SKILL.md": "---\nname: local\n---\n# local"})

	result := sb.RunCLI("unlink", "local")
	result.AssertFailure(t)
	result.AssertAnyOutputContains(t, "is not a link")
	if !sb.FileExists(filepath.Join(sb.SourcePath, "local", "SKILL.md")) {
		t.Fatal("unlink removed a plain directory")
	}
}

func TestLink_ProjectUsesProjectSourceAndTargets(t *testing.T) {
	sb, checkout := linkSandbox(t)
	defer sb.Cleanup()
	projectRoot := sb.SetupProjectDir("claude")
	source := filepath.Join(projectRoot, ".skillshare", "skills")

	result := sb.RunCLIInDir(projectRoot, "link", filepath.Join(projectRoot, ".claude"), "--name", "_claude", "-p")
	result.AssertFailure(t)
	result.AssertAnyOutputContains(t, "target overlaps sync target")

	sb.RunCLIInDir(projectRoot, "link", checkout, "--enable", "-p").AssertSuccess(t)
	if !sb.IsSymlink(filepath.Join(source, "_dev-skills")) {
		t.Fatal("link not created in the project source")
	}
	if got := sb.ReadFile(filepath.Join(projectRoot, ".skillshare", "config.yaml")); !contains(got, "follow_source_links: true") {
		t.Fatalf("project config not enabled:\n%s", got)
	}
	sb.RunCLIInDir(projectRoot, "unlink", "_dev-skills", "-p").AssertSuccess(t)
	if sb.FileExists(filepath.Join(source, "_dev-skills")) || !sb.FileExists(filepath.Join(checkout, "foo", "SKILL.md")) {
		t.Fatal("project unlink did not remove only the link")
	}
}
