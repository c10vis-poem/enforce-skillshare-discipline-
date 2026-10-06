package config

import (
	"os"
	"path/filepath"
	"testing"
)

// setHome points the user home at dir. Windows reads USERPROFILE, not HOME.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// absPath gives a slash-form fixture path the volume it needs to be absolute
// on Windows.
func absPath(p string) string {
	return filepath.VolumeName(os.TempDir()) + filepath.FromSlash(p)
}
