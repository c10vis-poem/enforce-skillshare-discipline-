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

// Switching a block target to symlink records the restore point from the file
// as it is then, which must not include the block this extra had put there.
func TestSyncExtraFile_SwitchingBlockToLinkKeepsBlockOutOfRestorePoint(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "team")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend"), false, ""); err != nil {
		t.Fatal(err)
	}
	linked := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "symlink")
	if _, err := SyncExtraFile(linked, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreExtraTarget(linked); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != "# Mine\n" {
		t.Fatalf("restored =\n%s\nwant the file without the block", got)
	}
}

func TestRestoreExtraTarget_EditedBlockIsKeptAsDrift(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	f := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend")
	if _, err := SyncExtraFile(f, false, ""); err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(readFile(t, target), "\nrule\n", "\nrule, edited\n", 1)
	os.WriteFile(target, []byte(edited), 0644)

	if _, err := RestoreExtraTarget(f); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != "# Mine\n" {
		t.Fatalf("restored =\n%s\nwant only the user's lines", got)
	}
	if got := driftBackups(t, target); len(got) != 1 || got[0] != edited {
		t.Fatalf("drift backups = %q, want the edited file", got)
	}
}

// A source that quotes a marker inside a code fence must round-trip: the
// fence is tracked inside the block too, so the quoted marker is text.
func TestSyncExtraFile_BlockWithFencedMarkerExampleStaysSynced(t *testing.T) {
	body := "Blocks look like:\n\n```md\n<!-- skillshare:extra src=\"x\" sha256=0123456789abcdef -->\nbody\n" + contentBlockEnd + "\n```\n"
	src, tgt := setupExtraFileTest(t, body)
	target := filepath.Join(tgt, "CLAUDE.md")
	f := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "append")
	for i := 0; i < 2; i++ {
		if _, err := SyncExtraFile(f, false, ""); err != nil {
			t.Fatal(err)
		}
	}
	if got := ExtraFileStatus(f); got != "synced" {
		t.Errorf("status = %q, want synced", got)
	}
	if got := readFile(t, target); strings.Count(got, contentBlockEnd) != 2 {
		t.Fatalf("the quoted end marker must stay inside the one block:\n%s", got)
	}
}

// Switching copy → prepend: the copy skillshare wrote is not the user's
// content, so the file is rebuilt from the attach-time base plus the block.
func TestSyncExtraFile_SwitchingCopyToPrependRebuildsFromBase(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "copy"), false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend"), false, ""); err != nil {
		t.Fatal(err)
	}
	want := contentBlockBegin(filepath.Join(src, "AGENTS.md"), "rule") + "\nrule\n" + contentBlockEnd + "\n\n# Mine\n"
	if got := readFile(t, target); got != want {
		t.Fatalf("content =\n%s\nwant the base with one block, not the copy as well\n%s", got, want)
	}
}

func TestSyncExtraFile_SwitchingImportToAppendDropsTheImportLine(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	imported := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "import")
	if _, err := SyncExtraFile(imported, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "append"), false, ""); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, target)
	if imported.hasImport(got) {
		t.Fatalf("the @ line must go when the block takes its place:\n%s", got)
	}
	if !strings.HasPrefix(got, "# Mine\n") || strings.Count(got, contentBlockEnd) != 1 {
		t.Fatalf("content =\n%s\nwant the user's lines then one block", got)
	}
}

func TestSyncExtraFile_SwitchingPrependToImportDropsTheBlockAndKeepsAnEdit(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend"), false, ""); err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(readFile(t, target), "\nrule\n", "\nrule, edited\n", 1)
	os.WriteFile(target, []byte(edited), 0644)

	imported := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "import")
	if _, err := SyncExtraFile(imported, false, ""); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, target)
	if strings.Contains(got, contentBlockEnd) || !imported.hasImport(got) || !strings.Contains(got, "# Mine\n") {
		t.Fatalf("content =\n%s\nwant the @ line and the user's lines, no block", got)
	}
	if backups := driftBackups(t, target); len(backups) != 1 || backups[0] != edited {
		t.Fatalf("drift backups = %q, want the edited file", backups)
	}
}

// A block whose end marker was deleted is damaged: switching to import and
// detaching must both refuse rather than leave it behind as user content.
func TestSyncExtraFile_DamagedBlockRefusesImportAndDetach(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	f := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend")
	if _, err := SyncExtraFile(f, false, ""); err != nil {
		t.Fatal(err)
	}
	damaged := strings.Replace(readFile(t, target), contentBlockEnd+"\n", "", 1)
	os.WriteFile(target, []byte(damaged), 0644)

	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "import"), false, ""); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("switch to import err = %v, want a damaged-block refusal", err)
	}
	if _, err := RestoreExtraTarget(f); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("detach err = %v, want a damaged-block refusal", err)
	}
	if got := readFile(t, target); got != damaged {
		t.Fatalf("a refused change must leave the file alone:\n%s", got)
	}
}

// A block that created its file, then had the user's own lines added around
// it, leaves those lines on detach; the file is the user's from then on, so a
// later link attach and restore must put it back rather than delete it.
func TestRestoreExtraTarget_LastBlockLeavingHandsTheFileToTheUser(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	block := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "append")
	if _, err := SyncExtraFile(block, false, ""); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(target, []byte("# Mine\n\n"+readFile(t, target)), 0644)
	if _, err := RestoreExtraTarget(block); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != "# Mine\n" {
		t.Fatalf("after detach =\n%s\nwant the user's lines", got)
	}

	linked := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "symlink")
	if _, err := SyncExtraFile(linked, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreExtraTarget(linked); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != "# Mine\n" {
		t.Fatalf("after link restore =\n%s\nwant the user's file back", got)
	}
}

// A source whose fence never closes still gets a block that parses on the next sync.
func TestSyncExtraFile_SourceWithUnclosedFenceStaysSynced(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "Example:\n\n```sh\necho hi\n")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	f := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend")
	for i := 0; i < 2; i++ {
		if _, err := SyncExtraFile(f, false, ""); err != nil {
			t.Fatal(err)
		}
	}
	if got := ExtraFileStatus(f); got != "synced" {
		t.Errorf("status = %q, want synced", got)
	}
	if _, err := RestoreExtraTarget(f); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); got != "# Mine\n" {
		t.Fatalf("after detach =\n%s\nwant only the user's lines", got)
	}
}

// Switching to symlink with a damaged block would record the block as the
// user's restore point; the switch is refused instead.
func TestSyncExtraFile_DamagedBlockRefusesSwitchToLink(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend"), false, ""); err != nil {
		t.Fatal(err)
	}
	damaged := strings.Replace(readFile(t, target), contentBlockEnd+"\n", "", 1)
	os.WriteFile(target, []byte(damaged), 0644)

	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "symlink"), false, ""); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("err = %v, want a damaged-block refusal", err)
	}
	if got := readFile(t, target); got != damaged {
		t.Fatalf("a refused switch must leave the file alone:\n%s", got)
	}
}

func TestSyncExtraFile_AppendAfterOpenFenceIsRefused(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	mine := "# Mine\n\n```sh\necho unfinished\n"
	os.WriteFile(target, []byte(mine), 0644)
	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "append"), false, ""); err == nil || !strings.Contains(err.Error(), "fence") {
		t.Fatalf("err = %v, want an open-fence refusal", err)
	}
	if got := readFile(t, target); got != mine {
		t.Fatalf("a refused append must leave the file alone:\n%s", got)
	}
}

// A prepend body with an unclosed fence, followed by the user's own fenced
// code, still ends where it was written: the hash marks the real end marker.
func TestSyncExtraFile_UnclosedFenceBodyBeforeUserCodeStaysSynced(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "```sh\necho from source\n")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("```go\nfmt.Println()\n```\n"), 0644)
	f := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend")
	for i := 0; i < 2; i++ {
		if _, err := SyncExtraFile(f, false, ""); err != nil {
			t.Fatal(err)
		}
	}
	if got := ExtraFileStatus(f); got != "synced" {
		t.Errorf("status = %q, want synced", got)
	}
}

// Two prepend blocks keep their order: the second is not misplaced just
// because the first sits above it.
func TestSyncExtraFile_SiblingPrependBlocksKeepTheirOrder(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "a")
	os.WriteFile(filepath.Join(src, "TEAM.md"), []byte("b"), 0644)
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	a := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend")
	b := NewExtraFile(src, "TEAM.md", tgt, "CLAUDE.md", "prepend")
	for _, f := range []ExtraFile{a, b} {
		if _, err := SyncExtraFile(f, false, ""); err != nil {
			t.Fatal(err)
		}
	}
	settled := readFile(t, target)
	for _, f := range []ExtraFile{a, b, a} {
		if _, err := SyncExtraFile(f, false, ""); err != nil {
			t.Fatal(err)
		}
		if got := readFile(t, target); got != settled {
			t.Fatalf("syncing %s again reordered the blocks:\n%s\nwant\n%s", f.Source, got, settled)
		}
	}
}

func TestSyncExtraFile_SourceWithMarkerLineIsRefused(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "before\n"+contentBlockEnd+"\nafter\n")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend"), false, ""); err == nil || !strings.Contains(err.Error(), "marker") {
		t.Fatalf("err = %v, want a marker-line refusal", err)
	}
	if got := readFile(t, target); got != "# Mine\n" {
		t.Fatalf("a refused sync must leave the file alone:\n%s", got)
	}
}

// A file a block created, with the user's lines added outside it, switched
// to symlink: restoring the link later puts those lines back.
func TestSyncExtraFile_CreatedBlockFileWithUserLinesSurvivesLinkRestore(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend"), false, ""); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(target, []byte(readFile(t, target)+"\n# Mine\n"), 0644)

	linked := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "symlink")
	if _, err := SyncExtraFile(linked, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreExtraTarget(linked); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, target); strings.TrimSpace(got) != "# Mine" {
		t.Fatalf("restored =\n%s\nwant the user's lines", got)
	}
}

func TestSyncExtraFile_ChangingPrependToAppendMovesTheBlock(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "rule")
	target := filepath.Join(tgt, "CLAUDE.md")
	os.WriteFile(target, []byte("# Mine\n"), 0644)
	if _, err := SyncExtraFile(NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "prepend"), false, ""); err != nil {
		t.Fatal(err)
	}
	appended := NewExtraFile(src, "AGENTS.md", tgt, "CLAUDE.md", "append")
	if _, err := SyncExtraFile(appended, false, ""); err != nil {
		t.Fatal(err)
	}
	want := "# Mine\n\n" + contentBlockBegin(filepath.Join(src, "AGENTS.md"), "rule") + "\nrule\n" + contentBlockEnd + "\n"
	if got := readFile(t, target); got != want {
		t.Fatalf("content =\n%s\nwant the block moved below the user's lines\n%s", got, want)
	}
	if got := ExtraFileStatus(appended); got != "synced" {
		t.Errorf("status = %q, want synced", got)
	}
}
