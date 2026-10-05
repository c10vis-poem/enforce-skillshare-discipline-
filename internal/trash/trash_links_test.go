package trash

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

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
				if relative {
					text = "../checkout"
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
				if got, err := os.Readlink(trashed); err != nil || got != target {
					t.Fatalf("trashed link = %q, %v", got, err)
				}
				items := List(filepath.Join(base, "trash"))
				if len(items) != 1 || items[0].Name != "_dev-skills" || items[0].LinkTarget != target {
					t.Fatalf("list = %+v", items)
				}
				if err := restore(&items[0], source, rename); err != nil {
					t.Fatal(err)
				}
				if got, err := os.Readlink(link); err != nil || got != target {
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
