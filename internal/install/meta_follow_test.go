package install

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/sourcewalk"
)

func TestComputeFileHashesFollowedRoot(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	os.MkdirAll(filepath.Join(target, "nested"), 0755)
	os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("# Skill"), 0644)
	os.WriteFile(filepath.Join(target, "nested", "file.txt"), []byte("content"), 0644)
	link := filepath.Join(source, "_dev-skills")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	off, err := ComputeFileHashes(link)
	if err != nil || len(off) != 0 {
		t.Fatalf("default hashes = %v, %v", off, err)
	}
	follow := sourcewalk.NewFollow(source, nil)
	hashes, err := ComputeFileHashes(link, follow)
	if err != nil || len(hashes) != 2 || hashes["SKILL.md"] == "" || hashes["nested/file.txt"] == "" {
		t.Fatalf("followed hashes = %v, %v", hashes, err)
	}
	store := NewMetadataStore()
	store.Set("_dev-skills", &MetadataEntry{Source: "local", Tracked: true})
	changed, err := store.RefreshTrackedRootSkillHashes("_dev-skills", link, follow)
	if err != nil || !changed {
		t.Fatalf("refresh = %t, %v", changed, err)
	}
	if entry := store.GetByPath("_dev-skills"); entry == nil || len(entry.FileHashes) != 2 {
		t.Fatalf("logical metadata = %+v", entry)
	}
}
