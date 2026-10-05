package sourcefs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/sourcewalk"
)

// followFixture reuses fixture's src/_f -> ext and opens src again with a
// policy that follows _f. ext/child holds a nested link escaping ext.
func followFixture(t *testing.T) (followed, plain *Root, ext string) {
	t.Helper()
	plain, ext = fixture(t)
	outside := filepath.Join(filepath.Dir(ext), "outside")
	mustWrite(t, filepath.Join(outside, "SKILL.md"), "outside")
	if err := os.Symlink(outside, filepath.Join(ext, "child", "escape")); err != nil {
		t.Fatal(err)
	}
	followed, err := Open(plain.Dir(), sourcewalk.NewFollow(plain.Dir(), nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { followed.Close() })
	return followed, plain, ext
}

func TestFollowWritesLandInTarget(t *testing.T) {
	r, _, ext := followFixture(t)
	name := filepath.Join("_f", "child", "SKILL.md")
	if err := r.WriteFileAtomic(name, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(ext, "child", "SKILL.md")); string(data) != "edited" {
		t.Fatalf("checkout file = %q", data)
	}
	if err := r.MkdirAll("_f", 0o755); err != nil {
		t.Fatalf("MkdirAll of the followed link: %v", err)
	}
	if err := r.MkdirAll(filepath.Join("_f", "new", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.Rename(filepath.Join("_f", "new"), filepath.Join("_f", "renamed")); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveAll(filepath.Join("_f", "renamed")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ext, "renamed")); !os.IsNotExist(err) {
		t.Fatalf("renamed dir still exists: %v", err)
	}
	if err := CheckMoveOut(r.Dir(), filepath.Join(r.Dir(), "_f", "child"), sourcewalk.NewFollow(r.Dir(), nil)); err != nil {
		t.Fatalf("nested move out: %v", err)
	}
}

func TestFollowStillRefusesEscapes(t *testing.T) {
	r, _, _ := followFixture(t)
	for _, name := range []string{
		"_f" + string(filepath.Separator) + ".." + string(filepath.Separator) + "x",
		filepath.Join("_f", "child", "escape", "SKILL.md"),
	} {
		if err := r.WriteFile(name, []byte("x"), 0o644); err == nil {
			t.Errorf("%s: write escaped", name)
		}
	}
	if err := r.Rename(filepath.Join("_f", "child"), "moved"); err == nil {
		t.Error("rename across the link boundary was allowed")
	}
}

func TestFollowKeepsLinkEntrySemantics(t *testing.T) {
	r, _, ext := followFixture(t)
	// The link itself is never written through or removed through.
	if err := r.RemoveAll("_f"); !errors.Is(err, ErrLink) {
		t.Fatalf("RemoveAll(_f) = %v, want ErrLink", err)
	}
	if _, err := os.Stat(filepath.Join(ext, "SKILL.md")); err != nil {
		t.Fatalf("checkout touched: %v", err)
	}
	if err := CheckMoveOut(r.Dir(), filepath.Join(r.Dir(), "_f"), sourcewalk.NewFollow(r.Dir(), nil)); err != nil {
		t.Fatalf("link entry must stay movable to trash: %v", err)
	}
}

func TestNoPolicyKeepsRefusals(t *testing.T) {
	_, plain, _ := followFixture(t)
	name := filepath.Join("_f", "child", "SKILL.md")
	if err := plain.WriteFile(name, []byte("x"), 0o644); !errors.Is(err, ErrLink) {
		t.Fatalf("WriteFile without policy = %v, want ErrLink", err)
	}
	if err := CheckMoveOut(plain.Dir(), filepath.Join(plain.Dir(), "_f", "child")); !errors.Is(err, ErrLink) {
		t.Fatalf("CheckMoveOut without policy = %v, want ErrLink", err)
	}
}
