//go:build !online

package integration

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"skillshare/internal/testutil"
)

func TestSyncProject_FollowSourceLinks_UnavailableLinkKeepsTargets(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectRoot := sb.SetupProjectDir("claude")
	sb.WriteProjectConfig(projectRoot, "targets:\n  - claude\nfollow_source_links: true\n")
	checkout := filepath.Join(sb.Root, "code", "dev-skills")
	for _, name := range []string{"foo", "bar"} {
		sb.WriteFile(filepath.Join(checkout, name, "SKILL.md"), "---\nname: "+name+"\n---\n# "+name)
	}
	sb.CreateSymlink(checkout, filepath.Join(projectRoot, ".skillshare", "skills", "_dev-skills"))
	target := filepath.Join(projectRoot, ".claude", "skills")

	sb.RunCLIInDir(projectRoot, "sync", "-p").AssertSuccess(t)
	want := sb.ListDir(target)
	if !sb.IsSymlink(filepath.Join(target, "_dev-skills__foo")) {
		t.Fatalf("followed skills not synced: %v", want)
	}

	// The checkout goes away (unmounted drive, re-clone in progress).
	if err := os.Rename(checkout, checkout+".moved"); err != nil {
		t.Fatal(err)
	}
	result := sb.RunCLIInDir(projectRoot, "sync", "-p")
	result.AssertSuccess(t)
	warning := "source link _dev-skills not followed: target is missing; kept existing target entries, nothing pruned this run"
	if got := strings.Count(result.Output(), warning); got != 1 {
		t.Fatalf("want the warning once, got %d:\n%s", got, result.Output())
	}
	if got := sb.ListDir(target); !reflect.DeepEqual(got, want) {
		t.Errorf("target changed: %v, want %v", got, want)
	}
}
