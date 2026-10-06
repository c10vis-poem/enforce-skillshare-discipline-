package instructions

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"skillshare/internal/config"
	syncpkg "skillshare/internal/sync"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestImportLines_SkipsFencesAndProse(t *testing.T) {
	content := "# Title\n@rules/a.md\nsee @b.md here\n```\n@not-an-import\n```\n  @~/c.md\n"
	if got := ImportLines(content); !slices.Equal(got, []int{2, 7}) {
		t.Errorf("ImportLines = %v, want [2 7]", got)
	}
}

func TestReadOrder_ProjectFallbackReadOnlyWithoutMain(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "AGENTS.md"), "x")
	it := Resolve(config.InstructionsTarget{Path: "CLAUDE.md", Fallback: "AGENTS.md", Import: true}, root)

	order := ReadOrder(it, false)
	if order[1].Kind != KindFallback || !order[1].Read {
		t.Fatalf("fallback without CLAUDE.md = %+v, want read", order[1])
	}
	write(t, filepath.Join(root, "CLAUDE.md"), "y")
	if order = ReadOrder(it, false); order[1].Read {
		t.Errorf("fallback with CLAUDE.md = %+v, want not read", order[1])
	}
}

func TestReadOrder_GlobalImportTargetListsUnreadAgents(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "rules", "a.md"), "a")
	write(t, filepath.Join(dir, "rules", "b.md"), "b")
	order := ReadOrder(config.InstructionsTarget{Path: filepath.Join(dir, "CLAUDE.md"), Import: true, Rules: filepath.Join(dir, "rules")}, true)
	kinds := []string{}
	for _, e := range order {
		kinds = append(kinds, e.Kind)
	}
	if !slices.Equal(kinds, []string{KindMain, KindRules, KindUnread}) || order[1].Count != 2 {
		t.Errorf("ReadOrder = %+v", order)
	}
}

func TestPlanConvert_ImportKeepsToolLines(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "CLAUDE.md")
	write(t, src, "# Rules\n\nBe brief.\n\n@~/.claude/rules/a.md\n")

	changes, err := PlanConvert(src, ConvertOptions{Method: MethodImport, KeepToolLines: true})
	if err != nil {
		t.Fatal(err)
	}
	if changes[0].After != "# Rules\n\nBe brief.\n" {
		t.Errorf("AGENTS.md = %q", changes[0].After)
	}
	if changes[1].After != "@AGENTS.md\n\n@~/.claude/rules/a.md\n" {
		t.Errorf("CLAUDE.md = %q", changes[1].After)
	}
	if err := Apply(changes); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dir, "AGENTS.md")); got != changes[0].After {
		t.Errorf("written AGENTS.md = %q", got)
	}
}

func TestPlanConvert_AppendIntoExistingShared(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "CLAUDE.md")
	shared := filepath.Join(dir, "shared", "AGENTS.md")
	write(t, src, "Be brief.\n@~/.claude/rules/a.md\n")
	write(t, shared, "# Team\n\n")

	changes, err := PlanConvert(src, ConvertOptions{Method: MethodImport, KeepToolLines: true, Dest: shared, Managed: true, Append: true})
	if err != nil {
		t.Fatal(err)
	}
	want := Change{Path: shared, Status: ChangeModified, Before: "# Team\n\n", After: "# Team\n\nBe brief.\n"}
	if changes[0] != want {
		t.Errorf("shared change = %+v, want %+v", changes[0], want)
	}
}

func TestPlanConvert_RenameNeedsProject(t *testing.T) {
	src := filepath.Join(t.TempDir(), "CLAUDE.md")
	write(t, src, "x\n")
	if _, err := PlanConvert(src, ConvertOptions{Method: MethodRename}); err == nil {
		t.Fatal("rename in global mode should fail")
	}
	changes, err := PlanConvert(src, ConvertOptions{Method: MethodRename, Project: true})
	if err != nil || changes[1].Status != ChangeRemoved {
		t.Fatalf("project rename = %+v, %v", changes, err)
	}
}

func TestPlanConvert_RefusesExistingAgents(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "CLAUDE.md"), "x\n")
	write(t, filepath.Join(dir, "AGENTS.md"), "y\n")
	if _, err := PlanConvert(filepath.Join(dir, "CLAUDE.md"), ConvertOptions{Method: MethodCopy}); err == nil {
		t.Fatal("expected error when AGENTS.md exists")
	}
}

func newResolver(sourceDir string) Resolver {
	return Resolver{
		SourceDir: func(e config.ExtraConfig) string { return filepath.Join(sourceDir, e.Name) },
		TargetDir: func(p string) string { return p },
	}
}

func TestAssign_ImportTargetTakesSeveralAndRestores(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	src, home := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "personal", "AGENTS.md"), "p\n")
	write(t, filepath.Join(src, "work", "AGENTS.md"), "w\n")
	claude := filepath.Join(home, "CLAUDE.md")
	write(t, claude, "mine\n")
	extras := []config.ExtraConfig{{Name: "personal", File: "AGENTS.md"}, {Name: "work", File: "AGENTS.md"}}
	target := Target{Name: "claude", File: claude, Import: true}
	r := newResolver(src)

	extras, err := Assign(extras, target, []string{"personal", "work"}, r)
	if err != nil {
		t.Fatal(err)
	}
	got := Assignments(extras, claude, r)
	if len(got) != 2 || got[0].Mode != "import" || got[0].Status != "synced" || extras[0].Targets[0].As != "CLAUDE.md" {
		t.Fatalf("assignments = %+v, targets = %+v", got, extras[0].Targets)
	}
	if extras, err = Assign(extras, target, nil, r); err != nil {
		t.Fatal(err)
	}
	if read(t, claude) != "mine\n" || len(extras[0].Targets)+len(extras[1].Targets) != 0 {
		t.Errorf("after unassign: %q, %+v", read(t, claude), extras)
	}
}

func TestAssign_SymlinkTargetTakesOne(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	src, home := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "personal", "AGENTS.md"), "p\n")
	write(t, filepath.Join(src, "work", "AGENTS.md"), "w\n")
	file := filepath.Join(home, "AGENTS.md")
	write(t, file, "mine\n")
	extras := []config.ExtraConfig{{Name: "personal", File: "AGENTS.md"}, {Name: "work", File: "AGENTS.md"}}
	target := Target{Name: "codex", File: file}
	r := newResolver(src)

	if _, err := Assign(extras, target, []string{"personal", "work"}, r); err == nil {
		t.Fatal("symlink target should take one file")
	}
	extras, err := Assign(extras, target, []string{"personal"}, r)
	if err != nil || read(t, file) != "p\n" {
		t.Fatalf("assign personal: %v, content %q", err, read(t, file))
	}
	if extras, err = Assign(extras, target, []string{"work"}, r); err != nil || read(t, file) != "w\n" {
		t.Fatalf("switch to work: %v, content %q", err, read(t, file))
	}
	if _, err = Assign(extras, target, nil, r); err != nil || read(t, file) != "mine\n" {
		t.Errorf("restore: %v, content %q", err, read(t, file))
	}
}

// A target without @import whose file already holds a shared file in a block
// takes the next one in a block of its own instead of refusing.
func TestAssign_BlockTargetTakesSeveral(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	src, home := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "personal", "AGENTS.md"), "p\n")
	write(t, filepath.Join(src, "work", "AGENTS.md"), "w\n")
	file := filepath.Join(home, "AGENTS.md")
	write(t, file, "mine\n")
	extras := []config.ExtraConfig{
		{Name: "personal", File: "AGENTS.md", Targets: []config.ExtraTargetConfig{{Path: home, Mode: "prepend"}}},
		{Name: "work", File: "AGENTS.md"},
	}
	target := Target{Name: "codex", File: file}
	r := newResolver(src)

	extras, err := Assign(extras, target, []string{"personal", "work"}, r)
	if err != nil {
		t.Fatal(err)
	}
	if got := Assignments(extras, file, r); len(got) != 2 || got[1].Mode != "prepend" || got[1].Status != "synced" {
		t.Fatalf("assignments = %+v", got)
	}
	// personal was configured but never synced, so only work's block is in the file.
	if got := read(t, file); !strings.HasSuffix(got, "\nmine\n") || strings.Count(got, "<!-- /skillshare:extra -->") != 1 {
		t.Fatalf("file =\n%s", got)
	}
}

// Two new files for a target whose only block holder is being removed: the
// plan must see the file without that holder and refuse, rather than let
// Assign pick a link for each after the removal and have the second replace the first.
func TestAssign_RemovingTheBlockHolderRefusesTwoNewFiles(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	src, home := t.TempDir(), t.TempDir()
	for _, n := range []string{"a", "b", "old"} {
		write(t, filepath.Join(src, n, "AGENTS.md"), n+"\n")
	}
	file := filepath.Join(home, "AGENTS.md")
	write(t, file, "mine\n")
	extras := []config.ExtraConfig{
		{Name: "a", File: "AGENTS.md"},
		{Name: "b", File: "AGENTS.md"},
		{Name: "old", File: "AGENTS.md", Targets: []config.ExtraTargetConfig{{Path: home, Mode: "prepend"}}},
	}
	target := Target{Name: "codex", File: file}
	r := newResolver(src)
	if _, err := syncpkg.SyncExtraFile(ExtraFile(extras[2], 0, r), false, ""); err != nil {
		t.Fatal(err)
	}
	before := read(t, file)

	var conflict *config.ExtraTargetConflict
	if _, err := Assign(extras, target, []string{"a", "b"}, r); !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want a target conflict", err)
	}
	if got := read(t, file); got != before {
		t.Fatalf("a refused assignment must leave the file alone:\n%s", got)
	}
	if len(extras[2].Targets) != 1 {
		t.Fatal("old must stay attached")
	}
}

func TestAssign_RestoreModifiedTargetPutsBackPreAttachFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	src, home := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "personal", "AGENTS.md"), "p\n")
	file := filepath.Join(home, "AGENTS.md")
	write(t, file, "mine\n")
	extras := []config.ExtraConfig{{Name: "personal", File: "AGENTS.md"}}
	target := Target{Name: "codex", File: file}
	r := newResolver(src)

	extras, err := Assign(extras, target, []string{"personal"}, r)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(file)
	write(t, file, "edited\n")

	if extras, err = Assign(extras, target, nil, r); err != nil {
		t.Fatal(err)
	}
	if got := read(t, file); got != "mine\n" || len(extras[0].Targets) != 0 {
		t.Errorf("after restore: content %q, targets %+v", got, extras[0].Targets)
	}
}

func TestAssign_LeavesOtherSingleFileExtrasAlone(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	src, home := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "personal", "AGENTS.md"), "p\n")
	write(t, filepath.Join(src, "prompt", "system.md"), "s\n")
	claude := filepath.Join(home, "CLAUDE.md")
	write(t, claude, "mine\n")
	prompt := config.ExtraConfig{Name: "prompt", File: "system.md", Targets: []config.ExtraTargetConfig{{Path: home, As: "CLAUDE.md", Mode: "import"}}}
	extras := []config.ExtraConfig{{Name: "personal", File: "AGENTS.md"}, prompt}
	target := Target{Name: "claude", File: claude, Import: true}
	r := newResolver(src)
	if _, err := syncpkg.SyncExtraFile(ExtraFile(prompt, 0, r), false, ""); err != nil {
		t.Fatal(err)
	}

	if _, err := PlanAssign(extras, target, []string{"prompt"}, r); err == nil {
		t.Error("PlanAssign accepted a single-file extra that is not an AGENTS.md")
	}
	extras, err := Assign(extras, target, []string{"personal"}, r)
	if err != nil {
		t.Fatal(err)
	}
	if extras, err = Assign(extras, target, nil, r); err != nil {
		t.Fatal(err)
	}
	if len(extras[1].Targets) != 1 || !strings.Contains(read(t, claude), "system.md") {
		t.Errorf("prompt extra detached: targets %+v, CLAUDE.md %q", extras[1].Targets, read(t, claude))
	}
}

func TestProjectReach(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "AGENTS.md"), "a\n")
	claude := config.InstructionsTarget{Path: "CLAUDE.md", Fallback: "AGENTS.md", Import: true}
	gemini := config.InstructionsTarget{Path: "GEMINI.md"}

	if r := ProjectReach(root, "claude", claude); r.How != ReachFallback || !r.Reads {
		t.Errorf("claude without CLAUDE.md = %+v", r)
	}
	write(t, filepath.Join(root, "CLAUDE.md"), "own\n")
	if r := ProjectReach(root, "claude", claude); r.How != ReachShadowed || r.Shim != ShimImport {
		t.Errorf("claude with CLAUDE.md = %+v", r)
	}
	if r := ProjectReach(root, "gemini", gemini); r.How != ReachMissing || r.Shim != ShimLink {
		t.Errorf("gemini without GEMINI.md = %+v", r)
	}
	if r := ProjectReach(root, "codex", config.InstructionsTarget{Path: "AGENTS.md"}); r.How != ReachDirect {
		t.Errorf("codex = %+v", r)
	}
}

func TestApplyShim(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	write(t, filepath.Join(root, "AGENTS.md"), "a\n")
	write(t, filepath.Join(root, "CLAUDE.md"), "own\n")
	claude := config.InstructionsTarget{Path: "CLAUDE.md", Fallback: "AGENTS.md", Import: true}
	gemini := config.InstructionsTarget{Path: "GEMINI.md"}

	if err := ApplyShim(root, claude, ShimImport); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(root, "CLAUDE.md")); !strings.HasPrefix(got, "@AGENTS.md\n") {
		t.Errorf("CLAUDE.md = %q", got)
	}
	if r := ProjectReach(root, "claude", claude); r.How != ReachImport {
		t.Errorf("claude after shim = %+v", r)
	}
	if err := ApplyShim(root, gemini, ShimLink); err != nil {
		t.Fatal(err)
	}
	if r := ProjectReach(root, "gemini", gemini); r.How != ReachLink {
		t.Errorf("gemini after shim = %+v", r)
	}
}

func TestHasMovableContent_FalseWhenOnlyManagedBlockAndImports(t *testing.T) {
	converted := "<!-- skillshare:instructions:begin -->\n@/x/extras/personal/AGENTS.md\n<!-- skillshare:instructions:end -->\n\n@~/.claude/rules/go-style.md\n"
	if HasMovableContent(converted) {
		t.Fatal("a file with only the managed block and @imports has nothing to move")
	}
	if !HasMovableContent(converted + "\n- Be concise.\n") {
		t.Fatal("a file with its own instructions has content to move")
	}
}

func TestProjectReach_NestedFileImportsResolveFromItsDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	write(t, filepath.Join(root, "AGENTS.md"), "a\n")
	own := filepath.Join(root, ".claude", "CLAUDE.md")
	write(t, own, "@AGENTS.md\nown\n") // .claude/AGENTS.md, not the project's
	it := config.InstructionsTarget{Path: ".claude/CLAUDE.md", Import: true}

	if r := ProjectReach(root, "claude", it); r.How != ReachShadowed {
		t.Errorf("@AGENTS.md in .claude/CLAUDE.md = %+v, want shadowed", r)
	}
	write(t, own, "own\n")
	for range 2 {
		if err := ApplyShim(root, it, ShimImport); err != nil {
			t.Fatal(err)
		}
	}
	if got := read(t, own); got != "@../AGENTS.md\n\nown\n" {
		t.Errorf(".claude/CLAUDE.md after two shims = %q", got)
	}
	if r := ProjectReach(root, "claude", it); r.How != ReachImport || !r.Reads {
		t.Errorf("after shim = %+v, want import", r)
	}
}

func TestPlanConvert_RenameRefusesManagedBlock(t *testing.T) {
	src := filepath.Join(t.TempDir(), "CLAUDE.md")
	write(t, src, "<!-- skillshare:instructions:begin -->\n@/x/extras/team/AGENTS.md\n<!-- skillshare:instructions:end -->\n\nBe brief.\n")
	if _, err := PlanConvert(src, ConvertOptions{Method: MethodRename, Project: true}); err == nil {
		t.Fatal("rename of a file with a managed import block should be refused")
	}
}

func TestAssignments_DirectoryInTheWay(t *testing.T) {
	home := t.TempDir()
	src := filepath.Join(home, "extras")
	write(t, filepath.Join(src, "team", "AGENTS.md"), "shared\n")
	target := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	extras := []config.ExtraConfig{{Name: "team", File: "AGENTS.md", Targets: []config.ExtraTargetConfig{{Path: filepath.Dir(target), Mode: "symlink"}}}}
	r := Resolver{SourceDir: func(e config.ExtraConfig) string { return filepath.Join(src, e.Name) }, TargetDir: func(p string) string { return p }}

	got := Assignments(extras, target, r)
	if len(got) != 1 || got[0].Reason != ReasonDirectory {
		t.Fatalf("Assignments = %+v, want reason %q", got, ReasonDirectory)
	}
}
