package trash

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/sourcefs"
	"skillshare/internal/sourcewalk"
)

func TestTrashLinkPreservesTraversalThroughIntermediateLink(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	external := filepath.Join(base, "external")
	for _, dir := range []string{source, filepath.Join(external, "subdir"), filepath.Join(external, "checkout")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(external, "subdir"), filepath.Join(source, "alias")); err != nil {
		t.Skip(err)
	}
	link := filepath.Join(source, "_dev-skills")
	if err := os.Symlink("alias/../checkout", link); err != nil {
		t.Skip(err)
	}
	trashed, err := MoveToTrash(link, "_dev-skills", filepath.Join(base, "trash"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{trashed, link} {
		if path == link {
			if err := Restore(&TrashEntry{Name: "_dev-skills", Path: trashed}, source); err != nil {
				t.Fatal(err)
			}
		}
		got, err := filepath.EvalSymlinks(path)
		if err != nil || got != filepath.Join(external, "checkout") {
			t.Fatalf("relocated target = %q, %v", got, err)
		}
	}
}

func TestRestoreFollowedChild(t *testing.T) {
	source, checkout, entry := setupFollowedRestore(t)
	if err := Restore(entry, source, sourcewalk.NewFollow(source, nil)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(checkout, "group", "foo", "SKILL.md")); err != nil || string(got) != "restore me" {
		t.Fatalf("restored content = %q, %v", got, err)
	}
	if _, err := os.Lstat(entry.Path); !os.IsNotExist(err) {
		t.Fatalf("trash entry still exists: %v", err)
	}
}

func TestRestoreRefusesNestedLink(t *testing.T) {
	source, checkout, entry := setupFollowedRestore(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(checkout, "group")); err != nil {
		t.Skip(err)
	}
	if err := Restore(entry, source, sourcewalk.NewFollow(source, nil)); !errors.Is(err, sourcefs.ErrLink) {
		t.Fatalf("expected nested link refusal, got %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(entry.Path, "SKILL.md")); err != nil || string(got) != "restore me" {
		t.Fatalf("trash copy changed = %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "foo")); !os.IsNotExist(err) {
		t.Fatalf("restore wrote outside checkout: %v", err)
	}
}

func TestRestoreRefusesFirstLevelLinkWithoutPolicy(t *testing.T) {
	source, checkout, entry := setupFollowedRestore(t)
	if err := Restore(entry, source); !errors.Is(err, sourcefs.ErrLink) {
		t.Fatalf("expected disabled-policy link refusal, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(entry.Path, "SKILL.md")); err != nil {
		t.Fatalf("trash copy missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(checkout, "group")); !os.IsNotExist(err) {
		t.Fatalf("restore created parents in checkout: %v", err)
	}
}

func setupFollowedRestore(t *testing.T) (string, string, *TrashEntry) {
	t.Helper()
	base := t.TempDir()
	source := filepath.Join(base, "source")
	checkout := filepath.Join(base, "checkout")
	trashed := filepath.Join(base, "trash", "foo")
	for _, dir := range []string{source, checkout, trashed} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(checkout, filepath.Join(source, "_dev-skills")); err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(filepath.Join(trashed, "SKILL.md"), []byte("restore me"), 0644); err != nil {
		t.Fatal(err)
	}
	return source, checkout, &TrashEntry{Name: "_dev-skills/group/foo", Path: trashed}
}

func TestTrashLink(t *testing.T) {
	for _, relative := range []bool{false, true} {
		for _, fallback := range []bool{false, true} {
			t.Run(fmt.Sprintf("relative=%t/fallback=%t", relative, fallback), func(t *testing.T) {
				base := t.TempDir()
				target := filepath.Join(base, "checkout")
				source := filepath.Join(base, "source")
				os.MkdirAll(target, 0755)
				os.MkdirAll(source, 0755)
				content := filepath.Join(target, "SKILL.md")
				os.WriteFile(content, []byte("unchanged"), 0644)
				link := filepath.Join(source, "_dev-skills")
				text := target
				wantText := target
				if relative {
					text = "../checkout"
					wantText = source + string(filepath.Separator) + text
				}
				if err := os.Symlink(text, link); err != nil {
					t.Skip(err)
				}
				rename := os.Rename
				if fallback {
					rename = func(string, string) error { return fmt.Errorf("cross-device rename") }
				}
				trashed, err := moveToTrash(link, "_dev-skills", filepath.Join(base, "trash"), rename)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Lstat(link); !os.IsNotExist(err) {
					t.Fatalf("source link still exists: %v", err)
				}
				if got, err := os.Readlink(trashed); err != nil || got != wantText {
					t.Fatalf("trashed link = %q, %v", got, err)
				}
				items := List(filepath.Join(base, "trash"))
				if len(items) != 1 || items[0].Name != "_dev-skills" || items[0].LinkTarget != target {
					t.Fatalf("list = %+v", items)
				}
				if err := restore(&items[0], source, rename); err != nil {
					t.Fatal(err)
				}
				if got, err := os.Readlink(link); err != nil || got != wantText {
					t.Fatalf("restored link = %q, %v", got, err)
				}
				if got, err := os.ReadFile(content); err != nil || string(got) != "unchanged" {
					t.Fatalf("target changed: %q, %v", got, err)
				}
				if _, err := os.Lstat(trashed); !os.IsNotExist(err) {
					t.Fatalf("trash link still exists: %v", err)
				}
			})
		}
	}
}

func TestTrashLinkListKeepsSiblings(t *testing.T) {
	base := t.TempDir()
	trashBase := filepath.Join(base, "trash")
	os.MkdirAll(trashBase, 0755)
	target := filepath.Join(base, "checkout")
	os.MkdirAll(filepath.Join(target, "not-an-entry_2026-01-01_10-00-00"), 0755)
	for _, name := range []string{"a", "b"} {
		if err := os.Symlink(target, filepath.Join(trashBase, name+"_2026-01-01_10-00-00")); err != nil {
			t.Skip(err)
		}
	}
	if items := List(trashBase); len(items) != 2 {
		t.Fatalf("links must neither hide siblings nor inventory target: %+v", items)
	}
}

func TestTrashBrokenLink(t *testing.T) {
	base := t.TempDir()
	link := filepath.Join(base, "broken")
	target := filepath.Join(base, "missing")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	if _, err := MoveToTrash(link, "broken", filepath.Join(base, "trash")); err != nil {
		t.Fatal(err)
	}
	items := List(filepath.Join(base, "trash"))
	if len(items) != 1 {
		t.Fatalf("list = %+v", items)
	}
	if err := Restore(&items[0], base); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("restored link = %q, %v", got, err)
	}
}
