//go:build !online

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"skillshare/internal/testutil"
	"skillshare/internal/utils"
)

func TestUninstall_FollowedLinkRootSkillRefused(t *testing.T) {
	for _, args := range [][]string{{"dev"}, {"--all"}, {"dev", "--json"}, {"dev", "--dry-run"}, {"dev", "--dry-run", "--json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			sb := testutil.NewSandbox(t)
			defer sb.Cleanup()
			checkout := filepath.Join(sb.Root, "code", "dev-skills")
			sb.WriteFile(filepath.Join(checkout, "SKILL.md"), "---\nname: dev\n---\n# dev")
			sb.WriteFile(filepath.Join(checkout, "child", "SKILL.md"), "---\nname: child\n---\n# child")
			link := filepath.Join(sb.SourcePath, "dev")
			sb.CreateSymlink(checkout, link)
			sb.WriteConfig("source: " + sb.SourcePath + "\nfollow_source_links: true\ntargets: {}\n")
			result := sb.RunCLI(append([]string{"uninstall", "--force"}, args...)...)
			result.AssertFailure(t)
			result.AssertAnyOutputContains(t, "dev is the linked folder itself; use unlink to remove the link")
			if slices.Contains(args, "--dry-run") && slices.Contains(args, "--json") {
				var output struct {
					Error   string   `json:"error"`
					Removed []string `json:"removed"`
				}
				if err := json.Unmarshal([]byte(result.Stdout), &output); err != nil {
					t.Fatalf("decode dry-run refusal: %v: %s", err, result.Stdout)
				}
				if output.Error != "dev is the linked folder itself; use unlink to remove the link" || len(output.Removed) != 0 {
					t.Fatalf("dry run claimed root skill removal: %+v", output)
				}
			}
			if !utils.IsSymlinkOrJunction(link) {
				t.Fatal("link was removed")
			}
			for _, rel := range []string{"SKILL.md", "child/SKILL.md"} {
				if !sb.FileExists(filepath.Join(link, filepath.FromSlash(rel))) {
					t.Fatalf("skill disappeared through link: %s", rel)
				}
			}
			sb.RunCLI("unlink", "dev").AssertSuccess(t)
			if utils.IsSymlinkOrJunction(link) || !sb.FileExists(filepath.Join(checkout, "SKILL.md")) {
				t.Fatal("unlink did not remove only the link")
			}
		})
	}
}

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

// --group on the link would move every skill out of the user's checkout,
// while uninstalling the link by name removes only the link entry.
func TestUninstall_GroupOnFollowedSourceLinkIsRefused(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()
	checkout := filepath.Join(sb.Root, "code", "dev-skills")
	sb.WriteFile(filepath.Join(checkout, "foo", "SKILL.md"), "---\nname: foo\n---\n# foo")
	link := filepath.Join(sb.SourcePath, "_dev-skills")
	sb.CreateSymlink(checkout, link)
	sb.WriteConfig("source: " + sb.SourcePath + "\nfollow_source_links: true\ntargets: {}\n")

	result := sb.RunCLI("uninstall", "--group", "_dev-skills", "--force")
	result.AssertFailure(t)
	result.AssertAnyOutputContains(t, "followed source link")
	if _, err := os.Stat(filepath.Join(checkout, "foo", "SKILL.md")); err != nil {
		t.Errorf("checkout skill touched: %v", err)
	}
	if !sb.IsSymlink(link) {
		t.Error("the link itself was removed")
	}
}
