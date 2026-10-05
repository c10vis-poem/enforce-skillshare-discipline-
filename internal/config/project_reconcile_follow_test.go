package config

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/install"
)

func TestReconcileProjectSkills_KeepsEntriesOfUnavailableSourceLink(t *testing.T) {
	for _, follow := range []bool{true, false} {
		root := t.TempDir()
		skillsDir := filepath.Join(root, ".skillshare", "skills")
		if err := os.MkdirAll(skillsDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "unmounted"), filepath.Join(skillsDir, "_dev-skills")); err != nil {
			t.Fatal(err)
		}
		cfg := &ProjectConfig{
			Targets:           []ProjectTargetEntry{{Name: "claude"}},
			Skills:            []SkillEntry{{Name: "_dev-skills", Source: "github.com/user/dev-skills"}},
			FollowSourceLinks: follow,
		}
		if err := cfg.Save(root); err != nil {
			t.Fatal(err)
		}
		store := install.NewMetadataStore()
		store.Set("_dev-skills", &install.MetadataEntry{Source: "github.com/user/dev-skills", Tracked: true})

		if err := ReconcileProjectSkills(root, cfg, store, skillsDir); err != nil {
			t.Fatalf("ReconcileProjectSkills: %v", err)
		}
		// Off, the link is invisible and both entries go, as before; on, the
		// walk knows it missed the link and keeps them.
		if store.Has("_dev-skills") != follow {
			t.Errorf("follow=%v: store entry kept = %v", follow, store.Has("_dev-skills"))
		}
		if kept := len(cfg.Skills) == 1; kept != follow {
			t.Errorf("follow=%v: config skills = %v", follow, cfg.Skills)
		}
	}
}
