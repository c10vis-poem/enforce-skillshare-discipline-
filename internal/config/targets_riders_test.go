package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func riderNames(riders []InstructionsRider) string {
	names := make([]string, len(riders))
	for i, r := range riders {
		names[i] = r.Name
	}
	return strings.Join(names, ",")
}

func TestInstructionRiders_CodexDetected(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	os.MkdirAll(filepath.Join(home, ".codex"), 0755)

	riders := InstructionRiders("universal", map[string]TargetConfig{"universal": {Path: "~/.agents/skills"}})
	if riderNames(riders) != "codex" || riders[0].Path != filepath.Join(home, ".codex", "AGENTS.md") || riders[0].Via != "universal" {
		t.Errorf("riders = %+v", riders)
	}
}

func TestInstructionRiders_CodexNotInstalled(t *testing.T) {
	setHome(t, t.TempDir())
	if riders := InstructionRiders("universal", map[string]TargetConfig{"universal": {Path: "~/.agents/skills"}}); len(riders) != 0 {
		t.Errorf("riders = %+v, want none", riders)
	}
}

func TestInstructionRiders_CodexConfigured(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	os.MkdirAll(filepath.Join(home, ".codex"), 0755)

	targets := map[string]TargetConfig{"universal": {Path: "~/.agents/skills"}, "codex": {Path: "~/.agents/skills"}}
	if riders := InstructionRiders("universal", targets); len(riders) != 0 {
		t.Errorf("riders = %+v, want none", riders)
	}
}

func TestInstructionRiders_SortedByName(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	os.MkdirAll(filepath.Join(home, ".codex"), 0755)
	os.MkdirAll(filepath.Join(home, ".config", "goose"), 0755)

	if got := riderNames(InstructionRiders("universal", map[string]TargetConfig{"universal": {Path: "~/.agents/skills"}})); got != "codex,goose" {
		t.Errorf("riders = %s, want codex,goose", got)
	}
}

// A tool that also scans the shared folder (Gemini, Pi) reads its skills too,
// and counts as installed when its own folder exists.
func TestInstructionRiders_AlsoScansTools(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	os.MkdirAll(filepath.Join(home, ".gemini"), 0755)
	os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0755)

	riders := InstructionRiders("universal", map[string]TargetConfig{"universal": {Path: "~/.agents/skills"}})
	if got := riderNames(riders); got != "gemini,pi" {
		t.Errorf("riders = %s, want gemini,pi", got)
	}
}

// Zed has no detect entry and its skills folder is the shared one, so the
// folder of its instruction file tells whether it is installed.
func TestInstructionRiders_DetectedByInstructionFolder(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0755)
	os.MkdirAll(filepath.Join(home, ".config", "zed"), 0755)

	if got := riderNames(InstructionRiders("universal", map[string]TargetConfig{"universal": {Path: "~/.agents/skills"}})); got != "zed" {
		t.Errorf("riders = %s, want zed", got)
	}
}

// Antigravity reads Gemini's GEMINI.md (same_as gemini); one file is listed
// once, under the tool that owns it.
func TestInstructionRiders_OneEntryPerFile(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	os.MkdirAll(filepath.Join(home, ".gemini", "config"), 0755)

	if got := riderNames(InstructionRiders("universal", map[string]TargetConfig{"universal": {Path: "~/.agents/skills"}})); got != "gemini" {
		t.Errorf("riders = %s, want gemini", got)
	}
}

// With gemini configured, GEMINI.md is managed on gemini's page, so
// Antigravity, which reads that same file, is not listed under universal.
func TestInstructionRiders_FileOfConfiguredTarget(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	os.MkdirAll(filepath.Join(home, ".gemini", "config"), 0755)

	targets := map[string]TargetConfig{"universal": {Path: "~/.agents/skills"}, "gemini": {Path: "~/.gemini/skills"}}
	if riders := InstructionRiders("universal", targets); len(riders) != 0 {
		t.Errorf("riders = %s, want none", riderNames(riders))
	}
}

// codex writes to ~/.agents/skills too, but riders belong on universal's page:
// with universal not configured, codex's page lists none.
func TestInstructionRiders_OnlyUniversal(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0755)
	os.MkdirAll(filepath.Join(home, ".config", "zed"), 0755)
	targets := map[string]TargetConfig{"codex": {Path: "~/.agents/skills"}}
	if riders := InstructionRiders("codex", targets); len(riders) != 0 {
		t.Errorf("riders = %s, want none", riderNames(riders))
	}
}

func TestInstructionReaders_Universal(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := strings.Join(InstructionReaders("universal", filepath.Join(home, ".agents", "AGENTS.md")), ","); got != "cline,jazz,warp" {
		t.Errorf("readers = %s, want cline,jazz,warp", got)
	}
}
