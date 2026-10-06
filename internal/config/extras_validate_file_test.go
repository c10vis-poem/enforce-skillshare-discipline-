package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateExtraConfig_SingleFile(t *testing.T) {
	tests := []struct {
		name    string
		extra   ExtraConfig
		wantErr string
	}{
		{"directory extra", ExtraConfig{Name: "rules", Targets: []ExtraTargetConfig{{Path: "/t", Mode: "copy"}}}, ""},
		{"file symlink", ExtraConfig{Name: "i", File: "AGENTS.md", Targets: []ExtraTargetConfig{{Path: "/t", Mode: "symlink", As: "CLAUDE.md"}}}, ""},
		{"file import", ExtraConfig{Name: "i", File: "AGENTS.md", Targets: []ExtraTargetConfig{{Path: "/t", Mode: "import"}}}, ""},
		{"file with separator", ExtraConfig{Name: "i", File: "a/AGENTS.md", Targets: []ExtraTargetConfig{{Path: "/t"}}}, "plain filename"},
		{"file dotdot", ExtraConfig{Name: "i", File: "..", Targets: []ExtraTargetConfig{{Path: "/t"}}}, "plain filename"},
		{"as with separator", ExtraConfig{Name: "i", File: "AGENTS.md", Targets: []ExtraTargetConfig{{Path: "/t", As: "../CLAUDE.md"}}}, "plain filename"},
		{"as without file", ExtraConfig{Name: "i", Targets: []ExtraTargetConfig{{Path: "/t", As: "CLAUDE.md"}}}, "requires file"},
		{"import without file", ExtraConfig{Name: "i", Targets: []ExtraTargetConfig{{Path: "/t", Mode: "import"}}}, "requires file"},
		{"file prepend", ExtraConfig{Name: "i", File: "AGENTS.md", Targets: []ExtraTargetConfig{{Path: "/t", Mode: "prepend", As: ".cursorrules"}}}, ""},
		{"append without file", ExtraConfig{Name: "i", Targets: []ExtraTargetConfig{{Path: "/t", Mode: "append"}}}, "append mode requires file"},
		{"flatten with file", ExtraConfig{Name: "i", File: "AGENTS.md", Targets: []ExtraTargetConfig{{Path: "/t", Flatten: true}}}, "flatten"},
		{"extension with import", ExtraConfig{Name: "i", File: "AGENTS.md", Targets: []ExtraTargetConfig{{Path: "/t", Mode: "import", Extension: "x"}}}, "extension"},
		{"extension with single-file copy", ExtraConfig{Name: "i", File: "AGENTS.md", Targets: []ExtraTargetConfig{{Path: "/t", Mode: "copy", Extension: "x"}}}, "extension cannot be used with a single-file extra"},
		{"extension on directory extra", ExtraConfig{Name: "rules", Targets: []ExtraTargetConfig{{Path: "/t", Mode: "copy", Extension: "x"}}}, ""},
		{"unknown mode", ExtraConfig{Name: "i", Targets: []ExtraTargetConfig{{Path: "/t", Mode: "bogus"}}}, "invalid mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExtraConfig(tt.extra)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// Managed modes (import, prepend, append) may share one target file; a mode
// that owns the whole file may not share it with anything.
func TestValidateExtraConnections_ManagedModesShareAFile(t *testing.T) {
	sourceDir := func(e ExtraConfig) string { return "/src/" + e.Name }
	targetDir := func(p string) string { return p }
	share := func(modeA, modeB string) error {
		return ValidateExtraConnections([]ExtraConfig{
			{Name: "a", File: "AGENTS.md", Targets: []ExtraTargetConfig{{Path: "/t", As: "CLAUDE.md", Mode: modeA}}},
			{Name: "b", File: "TEAM.md", Targets: []ExtraTargetConfig{{Path: "/t", As: "CLAUDE.md", Mode: modeB}}},
		}, sourceDir, targetDir)
	}
	if err := share("prepend", "import"); err != nil {
		t.Errorf("prepend + import: %v", err)
	}
	if err := share("prepend", "append"); err != nil {
		t.Errorf("prepend + append: %v", err)
	}
	if err := share("prepend", "copy"); err == nil {
		t.Error("prepend + copy should conflict: copy owns the whole file")
	}
}

// A block target that is a link to another extra's source would have the block
// written into that source; it is refused as an import target already is.
func TestValidateExtraConnections_BlockTargetLinkedToAnotherSourceConflicts(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src", "other"), 0755)
	os.WriteFile(filepath.Join(root, "src", "other", "AGENTS.md"), []byte("x"), 0644)
	tgt := filepath.Join(root, "tgt")
	os.MkdirAll(tgt, 0755)
	if err := os.Symlink(filepath.Join(root, "src", "other", "AGENTS.md"), filepath.Join(tgt, "CLAUDE.md")); err != nil {
		t.Skip(err)
	}
	sourceDir := func(e ExtraConfig) string { return filepath.Join(root, "src", e.Name) }
	for _, mode := range []string{"prepend", "append"} {
		err := ValidateExtraConnections([]ExtraConfig{
			{Name: "team", File: "AGENTS.md", Targets: []ExtraTargetConfig{{Path: tgt, As: "CLAUDE.md", Mode: mode}}},
			{Name: "other", File: "AGENTS.md"},
		}, sourceDir, func(p string) string { return p })
		var conflict *ExtraTargetConflict
		if !errors.As(err, &conflict) || conflict.Name != "other" {
			t.Errorf("%s: err = %v, want a conflict naming other", mode, err)
		}
	}
}

func TestValidSyncModes_SkillsRejectImport(t *testing.T) {
	if IsValidSyncMode("import") {
		t.Error("skills sync modes must not accept import")
	}
	if err := ValidateExtraMode("import"); err != nil {
		t.Errorf("extras mode import rejected: %v", err)
	}
}
