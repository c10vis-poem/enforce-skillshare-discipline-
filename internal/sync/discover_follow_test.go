package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/sourcewalk"
)

// linkedRepoSource builds a source whose _dev-skills entry links to a git
// checkout named differently, holding foo/SKILL.md.
func linkedRepoSource(t *testing.T) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), "skills")
	checkout := filepath.Join(t.TempDir(), "checkout")
	for _, dir := range []string{source, filepath.Join(checkout, ".git"), filepath.Join(checkout, "foo")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(checkout, "foo", "SKILL.md"), []byte("---\nname: foo\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(checkout, filepath.Join(source, "_dev-skills")); err != nil {
		t.Fatal(err)
	}
	return source
}

func TestDiscoverFollowsFirstLevelLinkedRepo(t *testing.T) {
	source := linkedRepoSource(t)
	walk := sourcewalk.Options{Follow: sourcewalk.NewFollow(source, nil)}
	skills, repos, err := DiscoverSourceSkillsLite(source, walk)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(repos, []string{"_dev-skills"}) {
		t.Fatalf("tracked repos = %v", repos)
	}
	if len(skills) != 1 {
		t.Fatalf("skills = %+v", skills)
	}
	got := skills[0]
	want := DiscoveredSkill{
		SourcePath: filepath.Join(source, "_dev-skills", "foo"),
		RelPath:    "_dev-skills/foo",
		FlatName:   "_dev-skills__foo",
		IsInRepo:   true,
	}
	if got.SourcePath != want.SourcePath || got.RelPath != want.RelPath || got.FlatName != want.FlatName || got.IsInRepo != want.IsInRepo {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDiscoverIgnoresLinkedRepoWhenFollowOff(t *testing.T) {
	source := linkedRepoSource(t)
	skills, repos, err := DiscoverSourceSkillsLite(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 || len(repos) != 0 {
		t.Fatalf("follow off must keep the link invisible: %+v, %v", skills, repos)
	}
}

func TestSyncFollowedWalkFailureKeepsTargets(t *testing.T) {
	for _, mode := range []string{"merge", "copy"} {
		t.Run(mode, func(t *testing.T) {
			source := linkedRepoSource(t)
			writeSkillMD(t, filepath.Join(source, "healthy"), "---\nname: healthy\n---\n")
			target := config.TargetConfig{Path: t.TempDir()}
			st := SkillTarget{Name: "test", Target: target, Mode: mode}
			skills, err := DiscoverSourceSkills(source, sourcewalk.Options{Follow: sourcewalk.NewFollow(source, nil)})
			if err != nil {
				t.Fatal(err)
			}
			if r := SyncSkillTarget(st, skills, SkillRunOptions{Source: source}); r.Err != nil {
				t.Fatal(r.Err)
			}
			dir := filepath.Join(source, "_dev-skills", "foo")
			if err := os.Chmod(dir, 0000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(dir, 0755) })
			if _, err := os.ReadDir(dir); !os.IsPermission(err) {
				t.Skip("requires directory read permissions to be enforced")
			}
			walk := sourcewalk.Options{Follow: sourcewalk.NewFollow(source, nil)}
			skills, err = DiscoverSourceSkills(source, walk)
			if err != nil {
				t.Fatal(err)
			}
			if len(skills) != 1 || skills[0].RelPath != "healthy" {
				t.Fatalf("healthy inventory: %+v", skills)
			}
			r := SyncSkillTarget(st, skills, SkillRunOptions{Source: source, SourceIncomplete: walk.Follow.Incomplete()})
			if r.Err != nil || len(r.Pruned) != 0 {
				t.Fatalf("pruned after walk failure: %+v", r)
			}
			if _, err := os.Lstat(filepath.Join(target.Path, "_dev-skills__foo")); err != nil {
				t.Fatalf("followed skill removed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(target.Path, "healthy", "SKILL.md")); err != nil {
				t.Fatalf("healthy skill missing: %v", err)
			}
		})
	}
}
