package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	syncpkg "skillshare/internal/sync"
)

func guidanceInput(t *testing.T, readers ...GuidanceReader) GuidanceInput {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	return GuidanceInput{Root: filepath.Join(t.TempDir(), "memory"), Readers: readers}
}

func guidanceReader(name string, files ...string) GuidanceReader {
	r := GuidanceReader{Name: name}
	for _, file := range files {
		r.Sources = append(r.Sources, GuidanceSource{Read: file, Live: true})
	}
	return r
}

func writeText(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func isStale(f GuidanceFailure) bool { return errors.Is(f.Err, ErrGuidanceStale) }

func TestGuidancePlanAndApply(t *testing.T) {
	dir := t.TempDir()
	own := writeText(t, filepath.Join(dir, "own.md"), "own\n")
	fresh := filepath.Join(dir, "new", "AGENTS.md")
	in := guidanceInput(t, guidanceReader("a", own), guidanceReader("b", fresh))
	names := []string{"b", "a"}

	plan, err := PlanGuidance(in, names, nil)
	if err != nil || len(plan.Changes) != 2 || plan.Changes[0].Created || !plan.Changes[1].Created || plan.Changes[0].Before != "own\n" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	var written []string
	res, err := ApplyGuidance(in, names, nil, plan.Token, func(c GuidanceChange) []GuidanceFailure {
		written = append(written, c.Path)
		return nil
	})
	if err != nil || len(res.Failures) != 0 || strings.Join(res.Applied, ",") != own+","+fresh || strings.Join(written, ",") != own+","+fresh {
		t.Fatalf("apply = %+v, %v, written %v", res, err, written)
	}
	if got := readText(t, own); got != plan.Changes[0].After || !strings.HasPrefix(got, "own\n\n<!-- skillshare:memory scope=global") {
		t.Errorf("own = %q", got)
	}
	if got := readText(t, fresh); got != plan.Changes[1].After {
		t.Errorf("new file = %q", got)
	}
	if backups, err := syncpkg.FileBackupVersions(own); err != nil || len(backups) == 0 {
		t.Errorf("backups of the changed file = %v, %v", backups, err)
	}
	for _, target := range GuidanceStatus(in) {
		if target.State != "configured" || target.Mode != ModePassive {
			t.Errorf("status = %+v", target)
		}
	}
	if again, err := PlanGuidance(in, names, nil); err != nil || len(again.Changes) != 0 || len(again.Skipped) != 2 || again.Skipped[0].Reason != "configured" {
		t.Errorf("second plan = %+v, %v", again, err)
	}
}

func TestApplyGuidanceRejectsPlanStaleSinceReview(t *testing.T) {
	own := writeText(t, filepath.Join(t.TempDir(), "own.md"), "own\n")
	in := guidanceInput(t, guidanceReader("a", own))
	names := []string{"a"}

	plan, _ := PlanGuidance(in, names, nil)
	writeText(t, own, "edited elsewhere\n")
	if _, err := ApplyGuidance(in, names, nil, plan.Token, nil); !errors.Is(err, ErrGuidanceStale) {
		t.Fatalf("file change: %v", err)
	}

	plan, _ = PlanGuidance(in, names, nil)
	in.Config = "changed"
	if _, err := ApplyGuidance(in, names, nil, plan.Token, nil); !errors.Is(err, ErrGuidanceStale) {
		t.Fatalf("config change: %v", err)
	}
	if _, err := ApplyGuidance(in, names, nil, "", nil); !errors.Is(err, ErrGuidanceStale) {
		t.Fatalf("empty token: %v", err)
	}
	if got := readText(t, own); got != "edited elsewhere\n" {
		t.Errorf("own = %q", got)
	}
}

func TestApplyGuidanceRechecksAfterBackup(t *testing.T) {
	own := writeText(t, filepath.Join(t.TempDir(), "own.md"), "own\n")
	in := guidanceInput(t, guidanceReader("a", own))
	plan, _ := PlanGuidance(in, []string{"a"}, nil)

	// An external editor changes the file while it is backed up.
	backupGuidanceFile = func(path, reason string) error {
		err := syncpkg.BackupFile(path, reason)
		writeText(t, path, "external edit after backup\n")
		return err
	}
	t.Cleanup(func() { backupGuidanceFile = syncpkg.BackupFile })

	res, err := ApplyGuidance(in, []string{"a"}, nil, plan.Token, nil)
	if err != nil || len(res.Applied) != 0 || len(res.Failures) != 1 || res.Failures[0].Path != own || !isStale(res.Failures[0]) {
		t.Fatalf("apply = %+v, %v", res, err)
	}
	if got := readText(t, own); got != "external edit after backup\n" {
		t.Errorf("external edit lost: %q", got)
	}
}

func TestApplyGuidanceRechecksEachReviewedFile(t *testing.T) {
	for _, change := range []string{"edited", "deleted", "created"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			first := writeText(t, filepath.Join(dir, "first.md"), "first\n")
			second := filepath.Join(dir, "second.md")
			if change != "created" {
				writeText(t, second, "second\n")
			}
			in := guidanceInput(t, guidanceReader("a", first), guidanceReader("b", second))
			names := []string{"a", "b"}
			plan, _ := PlanGuidance(in, names, nil)
			if len(plan.Changes) != 2 {
				t.Fatalf("plan = %+v", plan)
			}
			want := map[string]string{"edited": "external edit\n", "created": ""}[change]
			// An external editor changes the later file while the earlier file is applied.
			res, err := ApplyGuidance(in, names, nil, plan.Token, func(GuidanceChange) []GuidanceFailure {
				if change == "deleted" {
					if err := os.Remove(second); err != nil {
						t.Fatal(err)
					}
				} else {
					writeText(t, second, want)
				}
				return nil
			})
			if err != nil || len(res.Applied) != 1 || res.Applied[0] != first || len(res.Failures) != 1 || res.Failures[0].Path != second || !isStale(res.Failures[0]) {
				t.Fatalf("apply = %+v, %v", res, err)
			}
			if change == "deleted" {
				if _, err := os.Stat(second); !os.IsNotExist(err) {
					t.Fatal("deleted instructions recreated")
				}
			} else if got := readText(t, second); got != want {
				t.Fatalf("external edit lost: %q", got)
			}
		})
	}
}

func TestApplyGuidanceCompetingCreates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	in := guidanceInput(t, guidanceReader("a", path))
	plan, _ := PlanGuidance(in, []string{"a"}, nil)
	if len(plan.Changes) != 1 || !plan.Changes[0].Created {
		t.Fatalf("plan = %+v", plan)
	}
	type result struct {
		res GuidanceResult
		err error
	}
	const writers = 8
	results := make(chan result, writers)
	start := make(chan struct{})
	for range writers {
		go func() {
			<-start
			res, err := ApplyGuidance(in, []string{"a"}, nil, plan.Token, nil)
			results <- result{res, err}
		}()
	}
	close(start)
	wins, stale := 0, 0
	for range writers {
		r := <-results
		switch {
		case r.err == nil && len(r.res.Applied) == 1 && len(r.res.Failures) == 0:
			wins++
		case errors.Is(r.err, ErrGuidanceStale), r.err == nil && len(r.res.Applied) == 0 && len(r.res.Failures) == 1 && isStale(r.res.Failures[0]):
			stale++
		default:
			t.Fatalf("apply = %+v, %v", r.res, r.err)
		}
	}
	if wins != 1 || stale != writers-1 {
		t.Fatalf("wins = %d, stale = %d", wins, stale)
	}
	if got := readText(t, path); got != plan.Changes[0].After {
		t.Errorf("winner overwritten: %q", got)
	}
}

// The concurrent test cannot choose which check a loser fails; this one
// creates the file after the final review check, where only O_EXCL protects it.
func TestGuidanceCreateIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	change := GuidanceChange{Path: path, Created: true, After: "reviewed guidance"}
	if err := checkGuidanceChange(change); err != nil {
		t.Fatal(err)
	}
	writeText(t, path, "external instructions")
	if err := commitGuidanceChange(change); !errors.Is(err, ErrGuidanceStale) {
		t.Fatalf("expected stale plan, got %v", err)
	}
	if got := readText(t, path); got != "external instructions" {
		t.Fatalf("external instructions lost: %q", got)
	}
}

func TestCreateExclusiveRemovesUnfinishedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	failed := errors.New("write failed")
	if err := createExclusive(path, func(*os.File) error { return failed }); err != failed {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("unfinished file kept: %v", err)
	}
}

func TestGuidanceFlagsBlocksOfDifferentModes(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	in := guidanceInput(t, guidanceReader("claude", a, b))
	writeText(t, a, in.instructions(ModePassive))
	writeText(t, b, in.instructions(ModeActive))

	if got := GuidanceStatus(in)[0]; got.State != "broken" || got.Detail != "mixed_modes" || got.Mode != "" {
		t.Fatalf("status = %+v", got)
	}
	plan, err := PlanGuidance(in, []string{"claude"}, map[string]string{"claude": ModeActive})
	if err != nil || len(plan.Changes) != 0 || len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "mixed_modes" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
}

func TestGuidanceRefusesModeSwitchAcrossSeveralBlocks(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	in := guidanceInput(t, guidanceReader("claude", a, b))
	writeText(t, a, in.instructions(ModePassive))
	writeText(t, b, in.instructions(ModePassive))

	if got := GuidanceStatus(in)[0]; got.State != "configured" || got.Mode != ModePassive {
		t.Fatalf("status = %+v", got)
	}
	plan, err := PlanGuidance(in, []string{"claude"}, map[string]string{"claude": ModeActive})
	if err != nil || len(plan.Changes) != 0 || len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "multiple_blocks" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
}

func TestGuidanceLeavesUnsafeFilesUntouched(t *testing.T) {
	block := Instructions("/notes", "", ModePassive)
	cases := map[string]struct{ content, detail string }{
		"non-UTF-8": {"caf\xe9 rules\n", "unsupported"},
		"modified":  {"own\n" + strings.Replace(block, "Open only", "Always open", 1), "modified"},
		"malformed": {"own\n" + strings.TrimSuffix(block, blockEnd+"\n"), "malformed"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			own := writeText(t, filepath.Join(t.TempDir(), "own.md"), tc.content)
			in := guidanceInput(t, guidanceReader("a", own))
			in.Root = "/notes"
			if got := GuidanceStatus(in)[0]; got.State != "broken" || got.Detail != tc.detail || got.File != own {
				t.Fatalf("status = %+v", got)
			}
			plan, err := PlanGuidance(in, []string{"a"}, nil)
			if err != nil || len(plan.Changes) != 0 || len(plan.Skipped) != 1 || plan.Skipped[0].Reason != tc.detail {
				t.Fatalf("plan = %+v, %v", plan, err)
			}
			if res, err := ApplyGuidance(in, []string{"a"}, nil, plan.Token, nil); err != nil || len(res.Applied) != 0 || len(res.Failures) != 0 {
				t.Fatalf("apply = %+v, %v", res, err)
			}
			if got := readText(t, own); got != tc.content {
				t.Errorf("bytes changed: %q", got)
			}
		})
	}
}
