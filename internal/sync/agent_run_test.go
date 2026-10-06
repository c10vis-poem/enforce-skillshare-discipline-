package sync

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/resource"
)

func noExtension(string) (*ExtensionSpec, error) { return nil, nil }

func TestRunAgentSync_LinksAndPrunesOrphans(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	os.WriteFile(filepath.Join(sourceDir, "tutor.md"), []byte("# Tutor"), 0644)
	if err := os.Symlink(filepath.Join(sourceDir, "old.md"), filepath.Join(targetDir, "old.md")); err != nil {
		t.Fatal(err)
	}
	agents := []resource.DiscoveredResource{{FlatName: "tutor.md", RelPath: "tutor.md", AbsPath: filepath.Join(sourceDir, "tutor.md")}}

	results := RunAgentSync([]AgentTarget{{Name: "claude", Path: targetDir}}, agents,
		AgentRunOptions{Source: sourceDir, ResolveExtension: noExtension})

	r := results[0]
	if !r.Synced || r.Mode != "merge" || !slices.Equal(r.Linked, []string{"tutor.md"}) || !slices.Equal(r.Pruned, []string{"old.md"}) {
		t.Fatalf("unexpected result: %+v", r)
	}
}

func TestRunAgentSync_TargetWithoutPathIsNotSynced(t *testing.T) {
	results := RunAgentSync([]AgentTarget{{Name: "custom"}}, nil,
		AgentRunOptions{Source: t.TempDir(), ResolveExtension: noExtension})

	if r := results[0]; r.Synced || r.Err != nil || r.SyncErr != nil {
		t.Fatalf("expected untouched result, got %+v", r)
	}
}

func TestRunAgentSync_InvalidFilterFailsOnlyThatTarget(t *testing.T) {
	sourceDir := t.TempDir()
	os.WriteFile(filepath.Join(sourceDir, "tutor.md"), []byte("# Tutor"), 0644)
	agents := []resource.DiscoveredResource{{FlatName: "tutor.md", RelPath: "tutor.md", AbsPath: filepath.Join(sourceDir, "tutor.md")}}
	targets := []AgentTarget{
		{Name: "bad", Path: t.TempDir(), Config: config.ResourceTargetConfig{Include: []string{"["}}},
		{Name: "good", Path: t.TempDir()},
	}

	results := RunAgentSync(targets, agents, AgentRunOptions{Source: sourceDir, ResolveExtension: noExtension})

	if r := results[0]; r.Synced || r.Err == nil || !strings.HasPrefix(r.Err.Error(), "invalid agent filter: ") {
		t.Fatalf("expected filter error, got %+v", r)
	}
	if r := results[1]; !r.Synced || len(r.Linked) != 1 {
		t.Fatalf("expected good target synced, got %+v", r)
	}
}

func TestRunAgentSync_PruneErrorBecomesWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports reading a file as a directory as not-exist, which prune ignores")
	}
	// A dry run skips creating the target, so prune reads a path that is a file.
	targetPath := filepath.Join(t.TempDir(), "agents")
	os.WriteFile(targetPath, []byte("not a directory"), 0644)

	results := RunAgentSync([]AgentTarget{{Name: "claude", Path: targetPath}}, nil,
		AgentRunOptions{Source: t.TempDir(), DryRun: true, ResolveExtension: noExtension})

	r := results[0]
	if r.PruneErr == nil || !slices.ContainsFunc(r.Warnings, func(w string) bool {
		return strings.HasPrefix(w, "claude: agents prune failed: ")
	}) {
		t.Fatalf("expected prune failure warning, got %+v", r)
	}
}
