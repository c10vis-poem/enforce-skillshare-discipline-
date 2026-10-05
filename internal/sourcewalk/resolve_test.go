package sourcewalk

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFollowResolve(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on windows")
	}
	source := filepath.Join(t.TempDir(), "skills")
	checkout := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(checkout, filepath.Join(source, "_dev")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "gone"), filepath.Join(source, "_missing")); err != nil {
		t.Fatal(err)
	}
	// An alias of the source root: callers may spell paths through it.
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}

	f := NewFollow(source, nil)
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{filepath.Join(source, "_dev", "foo"), filepath.Join(checkout, "foo"), true},
		{filepath.Join(source, "_dev"), checkout, true},
		{filepath.Join(alias, "_dev", "foo", "SKILL.md"), filepath.Join(checkout, "foo", "SKILL.md"), true},
		{filepath.Join(source, "plain", "foo"), "", false},
		{filepath.Join(source, "_missing", "foo"), "", false},
		{source, "", false},
		{filepath.Dir(source), "", false},
	}
	for _, c := range cases {
		got, ok := f.Resolve(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("Resolve(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
	if !f.Incomplete() {
		t.Error("missing link target should mark the policy incomplete")
	}

	var nilFollow *Follow
	if _, ok := nilFollow.Resolve(filepath.Join(source, "_dev")); ok {
		t.Error("nil policy must resolve nothing")
	}
}

// A relative global source path is discovered and resolved like an absolute
// one: the policy compares canonical absolute forms, not the walk's spelling.
func TestFollowRelativeSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on windows")
	}
	root, checkout := followFixture(t)
	t.Chdir(filepath.Dir(root))
	f := NewFollow("skills", nil)
	walk, _ := walkEntries(t, "skills", Options{Follow: f}, "")
	if len(walk) != 8 || len(f.Skipped()) != 0 {
		t.Fatalf("walk %v, skipped %v", walk, f.Skipped())
	}
	got, ok := f.Resolve(filepath.Join("skills", "_dev", "foo"))
	if !ok || got != filepath.Join(checkout, "foo") {
		t.Fatalf("Resolve = %q, %v", got, ok)
	}
}
