package sync

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// What restoring a single-file extra target leaves at its path.
const (
	RestoreKindContent = "content" // a file with ExtraRestorePreview.Content
	RestoreKindDelete  = "delete"  // no file: none existed before attaching
	RestoreKindLink    = "link"    // the user's own link to ExtraRestorePreview.LinkTo
)

// ExtraRestorePreview describes what RestoreExtraTarget would put back.
type ExtraRestorePreview struct {
	Kind    string
	Content string
	LinkTo  string
	// RecordedAt is when the target was attached; zero without a record.
	RecordedAt time.Time
	// Drift: edits made after attaching are, or will be, kept as a drift
	// backup rather than restored.
	Drift bool
}

// PreviewRestoreExtraTarget reports what RestoreExtraTarget(f) would put back,
// read from the attach-time record, without changing anything.
func PreviewRestoreExtraTarget(f ExtraFile) ExtraRestorePreview {
	dir := extraBackupDir(f.Target)
	record, at := extraAttachRecord(f.Target)
	p := ExtraRestorePreview{RecordedAt: at, Drift: hasExtraDriftSince(dir, at)}

	if f.Mode == "import" || isContentBlockMode(f.Mode) {
		// Only the import line or block goes, unless nothing else is left in the file.
		data, _ := os.ReadFile(f.Target)
		stripped, _ := f.removeImport(string(data))
		if isContentBlockMode(f.Mode) {
			stripped, _ = f.removeContentBlock(string(data))
		}
		if strings.TrimSpace(stripped) != "" || record == "" {
			p.Kind, p.Content = RestoreKindContent, stripped
			return p
		}
	} else if info, err := os.Lstat(f.Target); err == nil {
		owned, drift := extraRestoreOwnership(f, info)
		if !owned && !drift {
			// Restore leaves a target it does not own unchanged.
			if info.Mode()&os.ModeSymlink != 0 {
				p.Kind = RestoreKindLink
				p.LinkTo, _ = os.Readlink(f.Target)
			} else {
				data, _ := os.ReadFile(f.Target)
				p.Kind, p.Content = RestoreKindContent, string(data)
			}
			return p
		}
		p.Drift = p.Drift || drift
	}

	switch record {
	case attachCreated:
		p.Kind = RestoreKindDelete
	case attachRestoreLink, attachRestoreJunction:
		dest, _ := os.ReadFile(filepath.Join(dir, record))
		p.Kind, p.LinkTo = RestoreKindLink, string(dest)
	case attachRestore:
		data, _ := os.ReadFile(filepath.Join(dir, record))
		p.Kind, p.Content = RestoreKindContent, string(data)
	default:
		// Without a record, restore falls back to the latest backup.
		if data, ok, err := latestExtraBackup(f.Target); ok && err == nil {
			p.Kind, p.Content = RestoreKindContent, string(data)
		} else {
			p.Kind = RestoreKindDelete
		}
	}
	return p
}

// extraAttachRecord returns the attach-time record of path and when it was
// written, or "" when there is none.
func extraAttachRecord(path string) (string, time.Time) {
	dir := extraBackupDir(path)
	for _, name := range []string{attachCreated, attachRestore, attachRestoreLink, attachRestoreJunction} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return name, info.ModTime()
		}
	}
	return "", time.Time{}
}

// hasExtraDriftSince reports whether a drift backup in dir was made at or
// after since (any, when since is zero).
func hasExtraDriftSince(dir string, since time.Time) bool {
	for _, name := range extraBackupNames(filepath.Join(dir, "drift")) {
		n, err := strconv.ParseInt(extraBackupStamp(name), 10, 64)
		if err == nil && (since.IsZero() || n >= since.UnixNano()) {
			return true
		}
	}
	return false
}
