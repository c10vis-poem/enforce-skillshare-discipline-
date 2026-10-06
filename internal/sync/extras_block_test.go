package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncExtraFile_PrependKeepsContentAndIsIdempotent(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "# team\nrule one\n")
	target := filepath.Join(tgt, ".cursorrules")
	os.WriteFile(target, []byte("# Mine\nkeep me\n"), 0644)
	f := NewExtraFile(src, "AGENTS.md", tgt, ".cursorrules", "prepend")

	for i := 0; i < 2; i++ {
		if _, err := SyncExtraFile(f, false, ""); err != nil {
			t.Fatal(err)
		}
	}
	got := readFile(t, target)
	body := "# team\nrule one"
	want := contentBlockBegin(filepath.Join(src, "AGENTS.md"), body) + "\n" + body + "\n" + contentBlockEnd + "\n\n# Mine\nkeep me\n"
	if got != want {
		t.Fatalf("content =\n%s\nwant\n%s", got, want)
	}
	if got := ExtraFileStatus(f); got != "synced" {
		t.Errorf("status = %q, want synced", got)
	}
}

func TestSyncExtraFile_AppendReplacesBlockWhenSourceChanges(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "v1")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	f := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "append")
	if _, err := SyncExtraFile(f, false, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(readFile(t, target), "# Mine\n\n"+contentBlockBegin(filepath.Join(src, "AGENTS.md"), "v1")) {
		t.Fatalf("append should put the block after the content:\n%s", readFile(t, target))
	}

	os.WriteFile(filepath.Join(src, "AGENTS.md"), []byte("v2\n"), 0644)
	if got := ExtraFileStatus(f); got != "drift" {
		t.Errorf("status after source change = %q, want drift", got)
	}
	if _, err := SyncExtraFile(f, false, ""); err != nil {
		t.Fatal(err)
	}
	want := "# Mine\n\n" + contentBlockBegin(filepath.Join(src, "AGENTS.md"), "v2") + "\nv2\n" + contentBlockEnd + "\n"
	if got := readFile(t, target); got != want {
		t.Fatalf("content =\n%s\nwant\n%s", got, want)
	}
}

func TestSyncExtraFile_EditedBlockIsRefused(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	f := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend")
	if _, err := SyncExtraFile(f, false, ""); err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(readFile(t, target), "\nrule\n", "\nrule, edited by hand\n", 1)
	os.WriteFile(target, []byte(edited), 0644)

	if got := ExtraFileStatus(f); got != "modified" {
		t.Errorf("status = %q, want modified", got)
	}
	if _, err := SyncExtraFile(f, false, ""); err == nil || !strings.Contains(err.Error(), "edited") {
		t.Fatalf("err = %v, want a refusal naming the hand edit", err)
	}
	if got := readFile(t, target); got != edited {
		t.Fatal("a refused sync must leave the file alone")
	}
}

// Collect keeps the hand edit: the block body becomes the source and the block
// is rewritten with a matching hash; the rest of the file is untouched.
func TestCollectBackExtraFile_BlockWritesTheEditToTheSource(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	f := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "append")
	if _, err := SyncExtraFile(f, false, ""); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(target, []byte(strings.Replace(readFile(t, target), "\nrule\n", "\nrule, edited\n", 1)), 0644)

	if err := CollectBackExtraFile(f, ""); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(src, "AGENTS.md")); got != "rule, edited\n" {
		t.Fatalf("source = %q, want the edit", got)
	}
	if got := ExtraFileStatus(f); got != "synced" {
		t.Errorf("status = %q, want synced", got)
	}
	if got := readFile(t, target); !strings.HasPrefix(got, "# Mine\n\n") {
		t.Fatalf("own content must stay:\n%s", got)
	}
}

// Reapply keeps the source: the block is rewritten and the rest of the file stays.
func TestReapplyExtraFile_BlockRewritesOnlyTheBlock(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	f := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend")
	if _, err := SyncExtraFile(f, false, ""); err != nil {
		t.Fatal(err)
	}
	synced := readFile(t, target)
	os.WriteFile(target, []byte(strings.Replace(synced, "\nrule\n", "\nrule, edited\n", 1)+"# Added below\n"), 0644)

	if err := ReapplyExtraFile(f, ""); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != synced+"# Added below\n" {
		t.Fatalf("content =\n%s\nwant the synced block with the lines added outside it kept", got)
	}
}

func TestRestoreExtraTarget_BlocksShareAFileAndLeaveWhenRemoved(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "team")
	os.WriteFile(filepath.Join(src, "TEAM.md"), []byte("more"), 0644)
	target := filepath.Join(tgt, "CLAUDE.md")
	a := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend")
	b := NewExtraFile(src, "TEAM.md", tgt, "CLAUDE.md", "append")
	if _, err := SyncExtraFile(a, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncExtraFile(b, false, ""); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); strings.Count(got, contentBlockEnd) != 2 {
		t.Fatalf("both blocks should be in the file:\n%s", got)
	}

	if _, err := RestoreExtraTarget(a); err != nil {
		t.Fatal(err)
	}
	want := contentBlockBegin(filepath.Join(src, "TEAM.md"), "more") + "\nmore\n" + contentBlockEnd + "\n"
	if got := readFile(t, target); got != want {
		t.Fatalf("after removing a =\n%s\nwant\n%s", got, want)
	}
	if _, err := RestoreExtraTarget(b); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("a file skillshare created should be deleted once its last block is gone")
	}
}
