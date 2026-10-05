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

func TestSync_FollowSourceLinks_UnavailableLinkKeepsTargets(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	checkout := filepath.Join(sb.Root, "code", "dev-skills")
	for _, name := range []string{"foo", "bar"} {
		sb.WriteFile(filepath.Join(checkout, name, "SKILL.md"), "---\nname: "+name+"\n---\n# "+name)
	}
	if err := os.MkdirAll(filepath.Join(checkout, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	sb.CreateSymlink(checkout, filepath.Join(sb.SourcePath, "_dev-skills"))
	mergeTarget := sb.CreateTarget("claude")
	copyTarget := sb.CreateTarget("cursor")
	sb.WriteConfig(`source: ` + sb.SourcePath + `
follow_source_links: true
targets:
  claude:
    path: ` + mergeTarget + `
  cursor:
    path: ` + copyTarget + `
    mode: copy
`)

	sb.RunCLI("sync").AssertSuccess(t)
	wantMerge, wantCopy := sb.ListDir(mergeTarget), sb.ListDir(copyTarget)
	if !sb.IsSymlink(filepath.Join(mergeTarget, "_dev-skills__foo")) || !sb.FileExists(filepath.Join(copyTarget, "_dev-skills__foo", "SKILL.md")) {
		t.Fatalf("followed skills not synced: merge %v, copy %v", wantMerge, wantCopy)
	}

	// The checkout goes away (unmounted drive, re-clone in progress).
	if err := os.Rename(checkout, checkout+".moved"); err != nil {
		t.Fatal(err)
	}
	result := sb.RunCLI("sync")
	result.AssertSuccess(t)
	warning := "source link _dev-skills not followed: target is missing; kept existing target entries, nothing pruned this run"
	if got := strings.Count(result.Output(), warning); got != 1 {
		t.Fatalf("want the warning once, got %d:\n%s", got, result.Output())
	}
	if got := sb.ListDir(mergeTarget); !reflect.DeepEqual(got, wantMerge) {
		t.Errorf("merge target changed: %v, want %v", got, wantMerge)
	}
	if got := sb.ListDir(copyTarget); !reflect.DeepEqual(got, wantCopy) {
		t.Errorf("copy target changed: %v, want %v", got, wantCopy)
	}
	if !sb.FileExists(filepath.Join(copyTarget, "_dev-skills__foo", "SKILL.md")) {
		t.Error("copy of an unavailable skill was deleted")
	}
}
