package uninstall

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/install"
	"skillshare/internal/sourcefs"
	"skillshare/internal/trash"
)

type fixture struct {
	source string
	opts   Options
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	source := filepath.Join(t.TempDir(), "skills")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	return &fixture{source: source, opts: Options{
		SourceDir:    source,
		TrashDir:     filepath.Join(t.TempDir(), "trash"),
		Store:        install.NewMetadataStore(),
		GitignoreDir: source,
	}}
}

func (f *fixture) skill(t *testing.T, rel string) Item {
	t.Helper()
	dir := filepath.Join(f.source, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+rel), 0o644); err != nil {
		t.Fatal(err)
	}
	return Item{Name: rel, Path: dir, TrashName: rel}
}

// repo creates a committed tracked repo holding one skill.
func (f *fixture) repo(t *testing.T, name string) Item {
	t.Helper()
	item := f.skill(t, name)
	runGit(t, item.Path, "init", "-q")
	runGit(t, item.Path, "add", ".")
	runGit(t, item.Path, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-qm", "seed")
	item.Repo, item.Gitignore = true, name
	return item
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestRun_RemovesSkillAndWhatInstallRecorded(t *testing.T) {
	f := newFixture(t)
	gone := f.skill(t, "frontend/foo")
	gone.Gitignore = "frontend/foo"
	f.skill(t, "foo")
	f.opts.Store.Set("frontend/foo", &install.MetadataEntry{Group: "frontend", Source: "github.com/acme/foo"})
	f.opts.Store.Set("foo", &install.MetadataEntry{Source: "github.com/acme/other"})
	f.opts.Store.SetTargetOverride("frontend/foo", []string{"claude"})
	if err := install.UpdateGitIgnoreBatch(f.source, []string{"frontend/foo", "foo"}); err != nil {
		t.Fatal(err)
	}

	out := Run([]Item{gone}, f.opts)

	if len(out.Results) != 1 || out.Results[0].Err != nil || out.GitignoreErr != nil || out.SaveErr != nil {
		t.Fatalf("unexpected outcome: %+v", out)
	}
	if exists(gone.Path) {
		t.Error("skill should have left the source")
	}
	if items := trash.List(f.opts.TrashDir); len(items) != 1 || items[0].Name != "frontend/foo" {
		t.Errorf("expected one trash entry named frontend/foo, got %+v", items)
	}
	saved, err := install.LoadMetadata(f.source)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Has("frontend/foo") || !saved.Has("foo") {
		t.Errorf("saved metadata should hold only foo, got %v", saved.List())
	}
	if len(saved.TargetOverrides) != 0 {
		t.Errorf("target override should be gone, got %v", saved.TargetOverrides)
	}
	ignore, _ := os.ReadFile(filepath.Join(f.source, ".gitignore"))
	if strings.Contains(string(ignore), "frontend/foo/") || !strings.Contains(string(ignore), "foo/") {
		t.Errorf(".gitignore should drop only frontend/foo:\n%s", ignore)
	}
}

func TestRun_DirtyRepoNeedsForce(t *testing.T) {
	f := newFixture(t)
	repo := f.repo(t, "_team")
	clean := f.skill(t, "plain")
	f.opts.Store.Set("_team", &install.MetadataEntry{Tracked: true})
	if err := os.WriteFile(filepath.Join(repo.Path, "wip.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := Run([]Item{repo, clean}, f.opts)

	if !errors.Is(out.Results[0].Err, ErrDirty) {
		t.Fatalf("dirty repo should be refused with ErrDirty, got %v", out.Results[0].Err)
	}
	if out.Results[1].Err != nil {
		t.Fatalf("the clean skill next to it should still go: %v", out.Results[1].Err)
	}
	if !exists(repo.Path) || !f.opts.Store.Has("_team") {
		t.Fatal("refused repo must keep its directory and metadata")
	}

	f.opts.Force = true
	if out := Run([]Item{repo}, f.opts); out.Results[0].Err != nil {
		t.Fatalf("forced uninstall failed: %v", out.Results[0].Err)
	}
	if exists(repo.Path) || f.opts.Store.Has("_team") {
		t.Error("forced uninstall should remove the repo and its metadata")
	}
}

func TestRun_UnreadableGitStatusNeedsForce(t *testing.T) {
	f := newFixture(t)
	repo := f.repo(t, "_team")
	if err := os.WriteFile(filepath.Join(repo.Path, ".git", "index"), []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := Run([]Item{repo}, f.opts)

	var statusErr *StatusError
	if !errors.As(out.Results[0].Err, &statusErr) || !exists(repo.Path) {
		t.Fatalf("expected a StatusError and the repo in place, got %v", out.Results[0].Err)
	}
}

func TestRun_MissingItemIsATrashErrorAndPrunesNothing(t *testing.T) {
	f := newFixture(t)
	f.opts.Store.Set("ghost", &install.MetadataEntry{})

	out := Run([]Item{{Name: "ghost", Path: filepath.Join(f.source, "ghost"), TrashName: "ghost"}}, f.opts)

	var trashErr *TrashError
	if !errors.As(out.Results[0].Err, &trashErr) {
		t.Fatalf("expected a TrashError, got %v", out.Results[0].Err)
	}
	if !f.opts.Store.Has("ghost") || len(out.Removed()) != 0 {
		t.Error("a failed item must keep its metadata")
	}
}

// A followed source link whose folder is itself a skill is the user's
// checkout: uninstall refuses it whether or not it is a git repo, and before
// looking at its git state.
func TestRun_RefusesLinkedSkillRoot(t *testing.T) {
	f := newFixture(t)
	checkout := t.TempDir()
	if err := os.WriteFile(filepath.Join(checkout, "SKILL.md"), []byte("# root"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, checkout, "init", "-q") // untracked SKILL.md: also dirty
	link := filepath.Join(f.source, "_dev")
	if err := os.Symlink(checkout, link); err != nil {
		t.Fatal(err)
	}
	f.opts.Follow = config.SkillsWalk(true, f.source, nil).Follow
	item := Item{Name: "_dev", Path: link, TrashName: "_dev", Repo: true}

	for _, err := range []error{Preflight([]Item{item}, f.opts)[0], Run([]Item{item}, f.opts).Results[0].Err} {
		if !errors.Is(err, sourcefs.ErrLinkedSkillRoot) {
			t.Fatalf("expected ErrLinkedSkillRoot, got %v", err)
		}
	}
	if !exists(link) {
		t.Error("the link must stay")
	}
}

func TestPreflight_ReportsDirtyRepoEvenWhenForced(t *testing.T) {
	f := newFixture(t)
	dirty := f.repo(t, "_dirty")
	clean := f.repo(t, "_clean")
	plain := f.skill(t, "plain")
	if err := os.WriteFile(filepath.Join(dirty.Path, "wip.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.opts.Force = true

	errs := Preflight([]Item{dirty, clean, plain}, f.opts)

	if !errors.Is(errs[0], ErrDirty) || errs[1] != nil || errs[2] != nil {
		t.Fatalf("expected [ErrDirty nil nil], got %v", errs)
	}
	if !exists(dirty.Path) || !exists(clean.Path) || !exists(plain.Path) {
		t.Error("Preflight must not move anything")
	}
}

func TestAgents_MovesFilesAndPrunesMetadata(t *testing.T) {
	source := t.TempDir()
	trashDir := filepath.Join(t.TempDir(), "trash")
	file := filepath.Join(source, "team", "reviewer.md")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("# reviewer"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := install.NewMetadataStore()
	store.Set("team/reviewer", &install.MetadataEntry{Kind: install.MetadataKindAgent})
	store.Set("ghost", &install.MetadataEntry{Kind: install.MetadataKindAgent})

	errs, saveErr := Agents([]Agent{
		{Name: "team/reviewer", File: file},
		{Name: "ghost", File: filepath.Join(source, "ghost.md")},
	}, source, trashDir, store)

	if errs[0] != nil || errs[1] == nil || saveErr != nil {
		t.Fatalf("expected [nil, error] and a clean save, got %v, %v", errs, saveErr)
	}
	if exists(file) {
		t.Error("agent file should have left the source")
	}
	saved, err := install.LoadMetadata(source)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Has("team/reviewer") || !saved.Has("ghost") {
		t.Errorf("saved metadata should keep only the agent that failed, got %v", saved.List())
	}
}
