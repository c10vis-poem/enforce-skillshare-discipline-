package install

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/sourcefs"
	"skillshare/internal/sourcewalk"
)

func TestSwapStagedIntoSource(t *testing.T) {
	source := t.TempDir()
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "SKILL.md"), []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(source, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	stage := func() string {
		dir := filepath.Join(t.TempDir(), "skill")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("staged"), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	for _, dest := range []string{
		filepath.Join(source, "linked"),          // the skill is a link
		filepath.Join(source, "linked", "child"), // the skill is below one
	} {
		if err := swapStagedIntoSource(source, stage(), dest, nil); !errors.Is(err, sourcefs.ErrLink) {
			t.Errorf("swap into %s: got %v, want ErrLink", dest, err)
		}
	}
	if data, _ := os.ReadFile(filepath.Join(external, "SKILL.md")); string(data) != "external" {
		t.Fatalf("external skill changed: %q", data)
	}
	if entries, _ := os.ReadDir(external); len(entries) != 1 {
		t.Fatalf("external tree changed: %v", entries)
	}

	if err := swapStagedIntoSource(source, stage(), filepath.Join(source, "plain"), nil); err != nil {
		t.Fatalf("swap into a real directory: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(source, "plain", "SKILL.md")); string(data) != "staged" {
		t.Fatalf("plain skill = %q, want staged content", data)
	}
}

func TestSwapStagedIntoSource_FollowedLinkReplacesInCheckout(t *testing.T) {
	source := t.TempDir()
	checkout := t.TempDir()
	if err := os.MkdirAll(filepath.Join(checkout, "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "foo", "old.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(checkout, filepath.Join(source, "_dev-skills")); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(t.TempDir(), "foo")
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "SKILL.md"), []byte("staged"), 0o644); err != nil {
		t.Fatal(err)
	}

	follow := sourcewalk.NewFollow(source, nil)
	if err := swapStagedIntoSource(source, staged, filepath.Join(source, "_dev-skills", "foo"), follow); err != nil {
		t.Fatalf("swap below a followed link: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(checkout, "foo", "SKILL.md")); string(data) != "staged" {
		t.Fatalf("checkout skill = %q, want staged content", data)
	}
	if _, err := os.Stat(filepath.Join(checkout, "foo", "old.md")); !os.IsNotExist(err) {
		t.Fatalf("old content kept: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(source, "_dev-skills")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link was replaced: %v", err)
	}
}

func TestSwapStagedIntoSource_FollowedCopyFailureKeepsOldSkill(t *testing.T) {
	source, checkout, staged := t.TempDir(), t.TempDir(), t.TempDir()
	dest := filepath.Join(checkout, "foo")
	if err := os.Mkdir(dest, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("old skill"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(checkout, filepath.Join(source, "linked")); err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "SKILL.md"), []byte("new skill"), 0644); err != nil {
		t.Fatal(err)
	}
	// A cyclic file link fails even as root, after an earlier file was copied.
	if err := os.Symlink("z-loop", filepath.Join(staged, "z-loop")); err != nil {
		t.Skip(err)
	}
	if err := swapStagedIntoSource(source, staged, filepath.Join(source, "linked", "foo"), sourcewalk.NewFollow(source, nil)); err == nil {
		t.Fatal("expected copy failure")
	}
	if got, err := os.ReadFile(filepath.Join(dest, "SKILL.md")); err != nil || string(got) != "old skill" {
		t.Fatalf("old skill lost: %q, %v", got, err)
	}
	if entries, err := os.ReadDir(checkout); err != nil || len(entries) != 1 || entries[0].Name() != "foo" {
		t.Fatalf("checkout leftovers: %v, %v", entries, err)
	}
	if got, err := os.ReadFile(filepath.Join(staged, "SKILL.md")); err != nil || string(got) != "new skill" {
		t.Fatalf("staged tree lost: %q, %v", got, err)
	}
}
