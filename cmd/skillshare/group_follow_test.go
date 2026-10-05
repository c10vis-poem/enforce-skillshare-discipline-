package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/sourcewalk"
)

func TestGroupFollowedLogicalPaths(t *testing.T) {
	source, checkout := t.TempDir(), t.TempDir()
	if err := os.Symlink(checkout, filepath.Join(source, "_dev-skills")); err != nil {
		t.Fatal(err)
	}
	setupUpdatableSkill(t, source, "_dev-skills/foo")
	setupTrackedRepo(t, source, "_dev-skills/_repo")
	// The group root itself is deliberately omitted even if it is a skill/repo.
	setupTrackedRepo(t, source, "_dev-skills")
	walk := sourcewalk.Options{Follow: sourcewalk.NewFollow(source, nil)}
	updates, err := resolveGroupUpdatable("_dev-skills", source, walk)
	if err != nil {
		t.Fatal(err)
	}
	// uninstall --group on the link would empty the user's checkout, so it is
	// refused in favor of `uninstall _dev-skills` or naming the skills.
	if _, err := resolveGroupSkills("_dev-skills", source, walk); err == nil || !strings.Contains(err.Error(), "followed source link") {
		t.Fatalf("uninstall --group on a followed link: %v", err)
	}
	if len(updates) != 2 {
		t.Fatalf("updates=%+v", updates)
	}
	for _, target := range updates {
		if target.path != filepath.Join(source, target.name) {
			t.Errorf("nonlogical update: %+v", target)
		}
		if !target.isRepo && (target.meta == nil || target.name != filepath.Join("_dev-skills", "foo")) {
			t.Errorf("metadata identity lost: %+v", target)
		}
	}
	if _, err := resolveGroupUpdatable("_dev-skills", source); err == nil {
		t.Error("disabled update accepted external group")
	}
	if _, err := resolveGroupSkills("_dev-skills", source); err == nil {
		t.Error("disabled uninstall accepted external group")
	}
}

func TestGroupFollowRejectsNestedAndOverlappingLinks(t *testing.T) {
	for _, nested := range []bool{false, true} {
		source, checkout := t.TempDir(), t.TempDir()
		group := "group"
		targets := []string{checkout}
		if nested {
			if err := os.Mkdir(filepath.Join(source, "group"), 0755); err != nil {
				t.Fatal(err)
			}
			group = filepath.Join(group, "nested")
			targets = nil
		}
		if err := os.Symlink(checkout, filepath.Join(source, group)); err != nil {
			t.Fatal(err)
		}
		walk := sourcewalk.Options{Follow: sourcewalk.NewFollow(source, targets)}
		if _, err := resolveGroupUpdatable(group, source, walk); err == nil {
			t.Error("update accepted disallowed link")
		}
		if _, err := resolveGroupSkills(group, source, walk); err == nil {
			t.Error("uninstall accepted disallowed link")
		}
	}
}
