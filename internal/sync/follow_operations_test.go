package sync

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/sourcewalk"
	"skillshare/internal/utils"
)

func TestFollowedProjectLinkOwnership(t *testing.T) {
	source := linkedRepoSource(t)
	project := filepath.Dir(source)
	targetPath := filepath.Join(project, "target")
	target := config.TargetConfig{Path: targetPath}
	skills, err := DiscoverSourceSkills(source, sourcewalk.Options{Follow: sourcewalk.NewFollow(source, nil)})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		result, err := SyncTargetMergeWithSkills("test", target, skills, source, false, false, project)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Updated) != 0 || len(result.Linked) != 1 {
			t.Fatalf("sync %d: %+v", i, result)
		}
	}
	link := filepath.Join(targetPath, skills[0].FlatName)
	dest, err := utils.ResolveLinkTarget(link)
	if err != nil {
		t.Fatal(err)
	}
	if dest != skills[0].SourcePath {
		t.Fatalf("link points to %s, want logical %s", dest, skills[0].SourcePath)
	}
	raw, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if canCreateRelativeLink() && filepath.IsAbs(raw) {
		t.Fatalf("expected relative project link: %s", raw)
	}
	_, linked, local := CheckStatusMerge(targetPath, source)
	if linked != 1 || local != 0 {
		t.Fatalf("linked=%d local=%d", linked, local)
	}
	result, err := PruneOrphanLinksWithSkills(PruneOptions{TargetPath: targetPath, SourcePath: source, Skills: skills, TargetName: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 0 {
		t.Fatalf("pruned valid link: %+v", result)
	}
	detached, err := DetachSkills(map[string]config.TargetConfig{"test": target}, "test", source, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(detached.Removed) != 1 {
		t.Fatalf("unlink did not recognize ownership: %+v", detached)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("target link still exists: %v", err)
	}
	if _, err := os.Stat(skills[0].SourcePath); err != nil {
		t.Fatalf("unlink touched source: %v", err)
	}
}

func TestFollowedSkillRootCopy(t *testing.T) {
	source := linkedRepoSource(t)
	root := filepath.Join(source, "_dev-skills")
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("# root"), 0644); err != nil {
		t.Fatal(err)
	}
	skills, err := DiscoverSourceSkills(source, sourcewalk.Options{Follow: sourcewalk.NewFollow(source, nil)})
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("skills=%+v", skills)
	}
	target := config.TargetConfig{Path: filepath.Join(t.TempDir(), "target")}
	if _, err := SyncTargetMergeWithSkills("test", target, skills, source, false, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncTargetCopyWithSkills("test", target, skills, source, false, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target.Path, "_dev-skills", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestPullForcePreservesFollowedLink(t *testing.T) {
	source := linkedRepoSource(t)
	link := filepath.Join(source, "_dev-skills")
	original, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	local := t.TempDir()
	if err := os.WriteFile(filepath.Join(local, "SKILL.md"), []byte("replacement"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := PullSkills([]LocalSkillInfo{{Name: "_dev-skills", Path: local}}, source, PullOptions{Force: true, Walk: sourcewalk.Options{Follow: sourcewalk.NewFollow(source, nil)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 1 || len(result.Pulled) != 0 || result.Warnings["_dev-skills"] == "" {
		t.Fatalf("result=%+v", result)
	}
	got, err := os.Readlink(link)
	if err != nil || got != original {
		t.Fatalf("link changed: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(link, "foo", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestFollowedLinkReformatKeepsIdentity(t *testing.T) {
	source := linkedRepoSource(t)
	targetPath := filepath.Join(filepath.Dir(source), "target")
	target := config.TargetConfig{Path: targetPath}
	skills, err := DiscoverSourceSkills(source, sourcewalk.Options{Follow: sourcewalk.NewFollow(source, nil)})
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range []string{"", filepath.Dir(source), ""} {
		if _, err := SyncTargetMergeWithSkills("test", target, skills, source, false, false, project); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(targetPath, skills[0].FlatName)
		raw, err := os.Readlink(link)
		if err != nil {
			t.Fatal(err)
		}
		if project == "" && raw != skills[0].SourcePath {
			t.Errorf("global link changed: %q", raw)
		}
		if project != "" && canCreateRelativeLink() && filepath.IsAbs(raw) {
			t.Errorf("project link not relative: %q", raw)
		}
		resolved, err := utils.ResolveLinkTarget(link)
		if err != nil || resolved != skills[0].SourcePath {
			t.Fatalf("identity changed: %q %v", resolved, err)
		}
	}
}

func TestFollowedLinkCanonicalRootAndParent(t *testing.T) {
	source := linkedRepoSource(t)
	aliases := t.TempDir()
	sourceAlias := filepath.Join(aliases, "source")
	target := t.TempDir()
	targetAlias := filepath.Join(aliases, "target")
	if err := os.Symlink(source, sourceAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, targetAlias); err != nil {
		t.Fatal(err)
	}
	logical := filepath.Join(sourceAlias, "_dev-skills", "foo")
	link := filepath.Join(targetAlias, "foo")
	if err := createLink(link, logical, true, sourceAlias); err != nil {
		t.Fatal(err)
	}
	raw, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(source, "_dev-skills", "foo")
	if got := resolveReadlink(raw, filepath.Join(target, "foo")); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
