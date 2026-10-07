package install

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeTrackedRepo(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel), ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestResolveTrackedRepo(t *testing.T) {
	root := t.TempDir()
	fakeTrackedRepo(t, root, "_team")
	fakeTrackedRepo(t, root, "org/_team")
	fakeTrackedRepo(t, root, "org/_solo")
	fakeTrackedRepo(t, root, "a/_dup")
	fakeTrackedRepo(t, root, "b/_dup")
	outside := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-outside", "_x")
	if err := os.MkdirAll(filepath.Join(outside, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct{ input, want string }{
		{"team", "_team"},
		{"_team", "_team"},
		{"org/team", "org/_team"},
		{"org/_team", "org/_team"},
		{"solo", "org/_solo"},
		{"team/", "_team"},
		{"org/team/", "org/_team"},
		{"./org/_team", "org/_team"},
		{"../" + filepath.Base(root) + "-outside/x", ""},
		{"ghost", ""},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got, path, err := ResolveTrackedRepo(root, c.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("name = %q, want %q", got, c.want)
			}
			if wantPath := filepath.Join(root, filepath.FromSlash(c.want)); c.want != "" && path != wantPath {
				t.Errorf("path = %q, want %q", path, wantPath)
			}
		})
	}

	if _, _, err := ResolveTrackedRepo(root, "dup"); err == nil {
		t.Error("expected an ambiguity error for dup")
	}
}
