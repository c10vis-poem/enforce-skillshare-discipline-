package sync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// seedFileBackup writes a backup folder for path holding the given history
// and drift files (name → content).
func seedFileBackup(t *testing.T, path string, history, drift map[string]string) string {
	t.Helper()
	dir := extraBackupDir(path)
	if err := os.MkdirAll(filepath.Join(dir, "drift"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "path"), []byte(filepath.Clean(path)), 0644); err != nil {
		t.Fatal(err)
	}
	for name, content := range history {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range drift {
		if err := os.WriteFile(filepath.Join(dir, "drift", name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func stamp(n int) string { return fmt.Sprintf("%019d", 1700000000000000000+n) }

func TestFileBackups_MixedOldAndReasonNamesKeepTimeOrder(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	dir := seedFileBackup(t, path, map[string]string{
		stamp(1) + ".bak":         "old\n",
		stamp(2) + ".convert.bak": "\n  second line first\n",
		stamp(3) + ".bak":         "third",
	}, map[string]string{
		stamp(4) + ".overwrite.bak": "my edit",
		stamp(0) + ".bak":           "symlink: /elsewhere/AGENTS.md\n",
	})

	if data, ok, _ := latestExtraBackup(path); !ok || string(data) != "third" {
		t.Fatalf("latest backup = %q, want third", data)
	}
	versions, err := FileBackupVersions(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range versions {
		got = append(got, v.ID+"|"+v.Kind+"|"+v.Reason+"|"+v.Preview+"|"+v.LinkTo)
	}
	want := []string{
		"drift:" + stamp(4) + ".overwrite|drift|overwrite|my edit|",
		stamp(3) + "|history||third|",
		stamp(2) + ".convert|history|convert|second line first|",
		stamp(1) + "|history||old|",
		"drift:" + stamp(0) + "|drift|||/elsewhere/AGENTS.md",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("versions:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !hasExtraDriftSince(dir, versions[0].Time) {
		t.Fatal("drift backup with a reason is not seen by restore preview")
	}

	for i := 5; i < 5+keepExtraBackups; i++ {
		if err := os.WriteFile(filepath.Join(dir, stamp(i)+".edit.bak"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	pruneExtraBackups(dir)
	names := extraBackupNames(dir)
	if len(names) != keepExtraBackups || names[0] != stamp(5)+".edit.bak" {
		t.Fatalf("prune kept %v", names)
	}
}

func TestBackupFile_RecordsReasonInName(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	if err := os.WriteFile(path, []byte("mine"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := BackupFile(path, BackupReasonEdit); err != nil {
		t.Fatal(err)
	}
	names := extraBackupNames(extraBackupDir(path))
	if len(names) != 1 || !strings.HasSuffix(names[0], ".edit.bak") {
		t.Fatalf("backup names = %v", names)
	}
}

func TestReapplyExtraFile_DriftBackupReasonIsOverwrite(t *testing.T) {
	src, tgt := setupExtraFileTest(t, "# shared")
	f := NewExtraFile(src, "AGENTS.md", tgt, "", "merge")
	if _, err := SyncExtraFile(f, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.Target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.Target, []byte("my edit"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ReapplyExtraFile(f, ""); err != nil {
		t.Fatal(err)
	}
	names := extraBackupNames(filepath.Join(extraBackupDir(f.Target), "drift"))
	if len(names) != 1 || !strings.HasSuffix(names[0], ".overwrite.bak") {
		t.Fatalf("drift names = %v", names)
	}
}

func TestRestoreFileBackup_SavesCurrentFirst(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	seedFileBackup(t, path, map[string]string{stamp(1) + ".edit.bak": "old"}, nil)
	if err := os.WriteFile(path, []byte("now"), 0600); err != nil {
		t.Fatal(err)
	}

	saved, err := RestoreFileBackup(path, stamp(1)+".edit", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "old" {
		t.Fatalf("content = %q, want old", got)
	}
	if !strings.HasSuffix(saved, ".restore") {
		t.Fatalf("saved id = %q", saved)
	}
	data, _, err := ReadFileBackupVersion(path, saved)
	if err != nil || string(data) != "now" {
		t.Fatalf("saved version = %q, %v", data, err)
	}
	// Windows has no permission bits to preserve.
	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %v, want the file's own 0600", info.Mode().Perm())
	}
}

func TestRestoreFileBackup_LinkNeedsUnlink(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	tmp := t.TempDir()
	path := filepath.Join(tmp, "CLAUDE.md")
	shared := filepath.Join(tmp, "AGENTS.md")
	os.WriteFile(shared, []byte("shared"), 0644)
	if err := os.Symlink(shared, path); err != nil {
		t.Fatal(err)
	}
	seedFileBackup(t, path, map[string]string{stamp(1) + ".bak": "old"}, nil)

	_, err := RestoreFileBackup(path, stamp(1), false)
	var linkErr *FileBackupLinkError
	if !errors.As(err, &linkErr) || linkErr.LinkTo != shared {
		t.Fatalf("err = %v, want link error to %s", err, shared)
	}
	if got := readFile(t, shared); got != "shared" {
		t.Fatalf("shared file changed to %q", got)
	}

	if _, err := RestoreFileBackup(path, stamp(1), true); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Lstat(path); info.Mode()&os.ModeSymlink != 0 || readFile(t, path) != "old" {
		t.Fatal("link was not replaced by the restored regular file")
	}
	if got := readFile(t, shared); got != "shared" {
		t.Fatalf("restore wrote through the link: %q", got)
	}
}

func TestRestoreFileBackup_OriginNoneRemovesFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	if err := os.WriteFile(path, []byte("written later"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := markExtraCreated(path); err != nil {
		t.Fatal(err)
	}
	versions, err := FileBackupVersions(path)
	if err != nil || len(versions) != 1 || versions[0].ID != "origin" || !versions[0].None {
		t.Fatalf("versions = %+v, %v", versions, err)
	}
	if _, err := RestoreFileBackup(path, "origin", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("origin with no file should remove the file")
	}
}

func TestFileBackups_RejectUnknownPathAndBadIDs(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	seedFileBackup(t, path, map[string]string{stamp(1) + ".bak": "old"}, nil)

	if _, err := FileBackupVersions(filepath.Join(t.TempDir(), "other.md")); !errors.Is(err, ErrFileBackupNotFound) {
		t.Fatalf("unknown path: err = %v", err)
	}
	for _, id := range []string{"../path", stamp(1) + "/../x", "drift:../../path", "123"} {
		if _, _, err := ReadFileBackupVersion(path, id); !errors.Is(err, ErrInvalidFileBackupID) {
			t.Fatalf("id %q: err = %v", id, err)
		}
	}
	if _, _, err := ReadFileBackupVersion(path, stamp(9)); !errors.Is(err, ErrFileBackupNotFound) {
		t.Fatalf("missing version: err = %v", err)
	}
}

func TestListFileBackups_NewestFirst(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	tmp := t.TempDir()
	a, b := filepath.Join(tmp, "a.md"), filepath.Join(tmp, "b.md")
	seedFileBackup(t, a, map[string]string{stamp(1) + ".bak": "a", stamp(5) + ".edit.bak": "a2"}, nil)
	seedFileBackup(t, b, map[string]string{stamp(3) + ".bak": "b"}, nil)

	files, err := ListFileBackups()
	if err != nil || len(files) != 2 {
		t.Fatalf("files = %+v, %v", files, err)
	}
	if files[0].Path != a || files[0].Versions != 2 || files[1].Path != b {
		t.Fatalf("files = %+v", files)
	}
}
