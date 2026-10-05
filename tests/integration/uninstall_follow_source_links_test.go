//go:build !online

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/testutil"
)

func TestUninstall_NestedSkillBelowFollowedSourceLink(t *testing.T) {
	for _, follow := range []bool{true, false} {
		sb := testutil.NewSandbox(t)
		checkout := filepath.Join(sb.Root, "code", "dev-skills")
		sb.WriteFile(filepath.Join(checkout, "foo", "SKILL.md"), "---\nname: foo\n---\n# foo")
		sb.WriteFile(filepath.Join(checkout, "bar", "SKILL.md"), "---\nname: bar\n---\n# bar")
		link := filepath.Join(sb.SourcePath, "_dev-skills")
		sb.CreateSymlink(checkout, link)
		cfg := "source: " + sb.SourcePath + "\ntargets: {}\n"
		if follow {
			cfg += "follow_source_links: true\n"
		}
		sb.WriteConfig(cfg)

		result := sb.RunCLI("uninstall", "_dev-skills/foo", "--force")
		if follow {
			result.AssertSuccess(t)
			if _, err := os.Stat(filepath.Join(checkout, "foo")); !os.IsNotExist(err) {
				t.Errorf("follow on: checkout foo still present: %v", err)
			}
		} else {
			result.AssertFailure(t)
			if _, err := os.Stat(filepath.Join(checkout, "foo", "SKILL.md")); err != nil {
				t.Errorf("follow off: checkout foo touched: %v", err)
			}
		}
		if !sb.IsSymlink(link) {
			t.Errorf("follow=%v: the link itself was removed", follow)
		}
		if _, err := os.Stat(filepath.Join(checkout, "bar", "SKILL.md")); err != nil {
			t.Errorf("follow=%v: sibling skill touched: %v", follow, err)
		}
		sb.Cleanup()
	}
}

func TestInstall_ReplacesSkillBelowFollowedSourceLink(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()
	checkout := filepath.Join(sb.Root, "code", "dev-skills")
	sb.WriteFile(filepath.Join(checkout, "foo", "SKILL.md"), "---\nname: foo\n---\n# old")
	sb.WriteFile(filepath.Join(checkout, "foo", "old.md"), "old")
	link := filepath.Join(sb.SourcePath, "dev")
	sb.CreateSymlink(checkout, link)
	sb.WriteConfig("source: " + sb.SourcePath + "\nfollow_source_links: true\ntargets: {}\n")
	upstream := filepath.Join(sb.Root, "upstream", "foo")
	sb.WriteFile(filepath.Join(upstream, "SKILL.md"), "---\nname: foo\n---\n# new")

	sb.RunCLI("install", upstream, "--into", "dev", "--force").AssertSuccess(t)

	if got := sb.ReadFile(filepath.Join(checkout, "foo", "SKILL.md")); got != "---\nname: foo\n---\n# new" {
		t.Errorf("checkout skill = %q, want the new content", got)
	}
	if sb.FileExists(filepath.Join(checkout, "foo", "old.md")) {
		t.Error("old content survived the replacement")
	}
	if !sb.IsSymlink(link) {
		t.Error("the link itself was replaced")
	}
}
