package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFoldHomePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("UserHomeDir unavailable")
	}
	sep := string(filepath.Separator)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"already-tilde", "~/foo", "~/foo"},
		{"exact-home", home, "~"},
		{"home-child", home + sep + "foo", "~" + sep + "foo"},
		{"home-deep", home + sep + ".config" + sep + "skillshare", "~" + sep + ".config" + sep + "skillshare"},
		{"sibling-not-home", home + "_other" + sep + "foo", home + "_other" + sep + "foo"},
		{"unrelated-abs", "/opt/unrelated/foo", "/opt/unrelated/foo"},
		{"relative", "foo/bar", "foo/bar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FoldHomePath(tt.in)
			if got != tt.want {
				t.Errorf("FoldHomePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFoldHomePath_WindowsCaseInsensitive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only case-fold behavior")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir unavailable")
	}
	// Pretend the path comes back with different casing (common on Windows).
	mixed := strings.ToLower(home) + string(filepath.Separator) + "Foo"
	got := FoldHomePath(mixed)
	if !strings.HasPrefix(got, "~") {
		t.Errorf("FoldHomePath(%q) = %q, expected fold under mixed case", mixed, got)
	}
}

func TestConfigWritePath(t *testing.T) {
	root := ResolveSymlink(t.TempDir())
	outside := filepath.Join(ResolveSymlink(t.TempDir()), "shared.yaml")
	inside := filepath.Join(root, "shared", "project.yaml")
	link := func(t *testing.T, target string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	projectLink := func(t *testing.T, target string) string {
		t.Helper()
		p := filepath.Join(root, ".skillshare", "config.yaml")
		os.MkdirAll(filepath.Dir(p), 0755)
		os.Remove(p)
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("global link writes its target", func(t *testing.T) {
		if got, err := ConfigWritePath(link(t, outside), false); err != nil || got != outside {
			t.Fatalf("got %q, %v; want %q", got, err, outside)
		}
	})
	t.Run("project link inside the project writes its target", func(t *testing.T) {
		if got, err := ConfigWritePath(projectLink(t, inside), true); err != nil || got != inside {
			t.Fatalf("got %q, %v; want %q", got, err, inside)
		}
	})
	t.Run("project link outside the project is refused", func(t *testing.T) {
		if _, err := ConfigWritePath(projectLink(t, outside), true); err == nil || !strings.Contains(err.Error(), "outside the project") {
			t.Fatalf("expected refusal, got %v", err)
		}
	})
	t.Run("dangling link chain leaving the project is refused", func(t *testing.T) {
		hop := filepath.Join(root, "hop.yaml")
		os.Remove(hop)
		if err := os.Symlink(filepath.Join(filepath.Dir(outside), "not-yet.yaml"), hop); err != nil {
			t.Fatal(err)
		}
		if _, err := ConfigWritePath(projectLink(t, hop), true); err == nil || !strings.Contains(err.Error(), "outside the project") {
			t.Fatalf("expected refusal, got %v", err)
		}
	})
	t.Run("missing target under an escaping directory link is refused", func(t *testing.T) {
		escape := filepath.Join(root, "escape")
		os.Remove(escape)
		if err := os.Symlink(filepath.Dir(outside), escape); err != nil {
			t.Fatal(err)
		}
		if _, err := ConfigWritePath(projectLink(t, "../escape/missing/config.yaml"), true); err == nil || !strings.Contains(err.Error(), "outside the project") {
			t.Fatalf("expected refusal, got %v", err)
		}
	})
	t.Run("project at the volume root is allowed", func(t *testing.T) {
		p := filepath.Join(string(filepath.Separator), ".skillshare-volume-root-test", "config.yaml")
		if _, err := ConfigWritePath(p, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("new project config is allowed", func(t *testing.T) {
		p := filepath.Join(root, "new", ".skillshare", "config.yaml")
		if _, err := ConfigWritePath(p, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
