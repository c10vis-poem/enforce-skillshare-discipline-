package sync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"skillshare/internal/config"
	"skillshare/internal/utils"
)

// Kinds of a file's restorable versions.
const (
	FileVersionHistory = "history" // saved before skillshare rewrote the file
	FileVersionDrift   = "drift"   // an edit skillshare replaced
	FileVersionOrigin  = "origin"  // the state recorded when a target was first attached
)

var (
	// ErrFileBackupNotFound is returned for a path without backups or an
	// unknown version.
	ErrFileBackupNotFound = errors.New("no backup found")
	// ErrInvalidFileBackupID is returned for a malformed version ID.
	ErrInvalidFileBackupID = errors.New("invalid backup version ID")

	fileBackupStamp = regexp.MustCompile(`^\d{19}(\.[a-z]+)?$`)
)

// FileBackupLinkError is returned when restoring over a symlink or junction
// without permission to replace it by a regular file.
type FileBackupLinkError struct {
	Path   string
	LinkTo string
}

func (e *FileBackupLinkError) Error() string {
	return fmt.Sprintf("%s is a link to %s; restoring replaces it with a regular file", e.Path, e.LinkTo)
}

// FileBackup summarizes the versions kept for one file.
type FileBackup struct {
	Path     string
	Versions int
	Latest   time.Time
}

// FileBackupVersion is one restorable version of a file.
type FileBackupVersion struct {
	ID      string // "<19-digit time>[.<reason>]", "drift:" + that, or "origin"
	Kind    string // FileVersionHistory, FileVersionDrift, or FileVersionOrigin
	Reason  string
	Time    time.Time
	Size    int64
	Preview string // first non-empty line, at most 120 characters
	LinkTo  string // the version is a link to this destination
	None    bool   // origin only: no file existed

	junction bool
}

// FileState describes what is at a backed-up path now.
type FileState struct {
	Exists bool
	LinkTo string
}

// CurrentFileState reports whether path exists and where it links to.
func CurrentFileState(path string) FileState {
	if _, err := os.Lstat(path); err != nil {
		return FileState{}
	}
	st := FileState{Exists: true}
	if utils.IsSymlinkOrJunction(path) {
		st.LinkTo, _ = os.Readlink(path)
	}
	return st
}

func fileBackupsRoot() string {
	return filepath.Join(config.StateDir(), "extras", "backups")
}

// ListFileBackups returns every file with at least one restorable version,
// most recently backed up first.
func ListFileBackups() ([]FileBackup, error) {
	entries, err := os.ReadDir(fileBackupsRoot())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []FileBackup
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(fileBackupsRoot(), e.Name(), "path"))
		if err != nil {
			continue
		}
		path := string(data)
		dir := extraBackupDir(path)
		if filepath.Base(dir) != e.Name() {
			continue // a path file that does not match its folder is not trusted
		}
		versions := fileBackupVersions(path, dir, false)
		if len(versions) == 0 {
			continue
		}
		fb := FileBackup{Path: path, Versions: len(versions)}
		for _, v := range versions {
			if v.Time.After(fb.Latest) {
				fb.Latest = v.Time
			}
		}
		out = append(out, fb)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Latest.After(out[j].Latest) })
	return out, nil
}

// FileBackupVersions returns the versions kept for path, newest first, with
// the attach-time origin last.
func FileBackupVersions(path string) ([]FileBackupVersion, error) {
	path, dir, err := fileBackupDirFor(path)
	if err != nil {
		return nil, err
	}
	return fileBackupVersions(path, dir, true), nil
}

// ReadFileBackupVersion returns the content of one version of path.
func ReadFileBackupVersion(path, id string) ([]byte, FileBackupVersion, error) {
	path, dir, err := fileBackupDirFor(path)
	if err != nil {
		return nil, FileBackupVersion{}, err
	}
	return readFileBackupVersion(path, dir, id)
}

// RestoreFileBackup puts version id back at path. A regular file there is
// saved first as a history backup with reason "restore"; the new backup's ID
// is returned ("" when its content was already the latest backup). A link at
// path is replaced only when unlink is set. An origin recorded as "no file"
// removes the file.
func RestoreFileBackup(path, id string, unlink bool) (string, error) {
	path, dir, err := fileBackupDirFor(path)
	if err != nil {
		return "", err
	}
	data, v, err := readFileBackupVersion(path, dir, id)
	if err != nil {
		return "", err
	}
	info, statErr := os.Lstat(path)
	exists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}
	isLink := exists && utils.IsSymlinkOrJunction(path)
	if isLink && !unlink {
		dest, _ := os.Readlink(path)
		return "", &FileBackupLinkError{Path: path, LinkTo: dest}
	}
	if exists && !isLink && !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}

	saved := ""
	if exists && !isLink {
		name, err := storeExtraBackup(path, BackupReasonRestore)
		if err != nil {
			return "", err
		}
		saved = strings.TrimSuffix(name, ".bak")
	}

	switch {
	case v.None:
		if exists {
			if err := os.Remove(path); err != nil {
				return saved, err
			}
		}
	case v.LinkTo != "":
		if exists {
			if err := os.Remove(path); err != nil {
				return saved, err
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return saved, err
		}
		if v.junction {
			err = utils.CreateJunction(path, v.LinkTo)
		} else {
			err = os.Symlink(v.LinkTo, path)
		}
		if err != nil {
			return saved, err
		}
	default:
		perm := os.FileMode(0644)
		if exists && !isLink {
			perm = info.Mode().Perm()
		}
		if isLink {
			if err := os.Remove(path); err != nil {
				return saved, err
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return saved, err
		}
		if err := os.WriteFile(path, data, perm); err != nil {
			return saved, err
		}
	}
	return saved, nil
}

// fileBackupDirFor resolves path to its backup folder, which must exist and
// name that very path: only backed-up files can be read through it.
func fileBackupDirFor(path string) (string, string, error) {
	if path == "" {
		return "", "", ErrFileBackupNotFound
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	abs = filepath.Clean(abs)
	dir := extraBackupDir(abs)
	recorded, err := os.ReadFile(filepath.Join(dir, "path"))
	if err != nil || string(recorded) != abs {
		return "", "", fmt.Errorf("%w for %s", ErrFileBackupNotFound, abs)
	}
	return abs, dir, nil
}

// extraBackupStamp returns the nanosecond prefix of a backup file name.
func extraBackupStamp(name string) string {
	stamp, _, _ := strings.Cut(strings.TrimSuffix(name, ".bak"), ".")
	return stamp
}

func fileBackupVersions(path, dir string, withContent bool) []FileBackupVersion {
	var versions []FileBackupVersion
	for _, kind := range []string{FileVersionHistory, FileVersionDrift} {
		sub := dir
		prefix := ""
		if kind == FileVersionDrift {
			sub, prefix = filepath.Join(dir, "drift"), "drift:"
		}
		for _, name := range extraBackupNames(sub) {
			base := strings.TrimSuffix(name, ".bak")
			if !fileBackupStamp.MatchString(base) {
				continue
			}
			v := stampedVersion(kind, prefix+base, base)
			if withContent {
				if data, err := os.ReadFile(filepath.Join(sub, name)); err == nil {
					fillVersionContent(&v, data)
				}
			}
			versions = append(versions, v)
		}
	}
	sort.SliceStable(versions, func(i, j int) bool { return versions[i].Time.After(versions[j].Time) })
	if record, at := extraAttachRecord(path); record != "" {
		v := FileBackupVersion{ID: FileVersionOrigin, Kind: FileVersionOrigin, Time: at}
		if withContent || record != attachRestore {
			if data, err := os.ReadFile(filepath.Join(dir, record)); err == nil {
				fillOriginContent(&v, record, data)
			}
		}
		versions = append(versions, v)
	}
	return versions
}

func stampedVersion(kind, id, base string) FileBackupVersion {
	stamp, reason, _ := strings.Cut(base, ".")
	n, _ := strconv.ParseInt(stamp, 10, 64)
	return FileBackupVersion{ID: id, Kind: kind, Reason: reason, Time: time.Unix(0, n)}
}

func fillVersionContent(v *FileBackupVersion, data []byte) {
	v.Size = int64(len(data))
	// Drift backups of a link hold "symlink: <dest>" or "junction: <dest>".
	if v.Kind == FileVersionDrift {
		text := string(data)
		for _, kind := range []string{"symlink", "junction"} {
			if dest, ok := strings.CutPrefix(text, kind+": "); ok && strings.Count(dest, "\n") == 1 && strings.HasSuffix(dest, "\n") {
				v.LinkTo, v.junction, v.Size = strings.TrimSuffix(dest, "\n"), kind == "junction", 0
				return
			}
		}
	}
	v.Preview = firstLine(data)
}

func fillOriginContent(v *FileBackupVersion, record string, data []byte) {
	switch record {
	case attachCreated:
		v.None = true
	case attachRestoreLink, attachRestoreJunction:
		v.LinkTo, v.junction = string(data), record == attachRestoreJunction
	default:
		v.Size = int64(len(data))
		v.Preview = firstLine(data)
	}
}

func readFileBackupVersion(path, dir, id string) ([]byte, FileBackupVersion, error) {
	if id == FileVersionOrigin {
		record, at := extraAttachRecord(path)
		if record == "" {
			return nil, FileBackupVersion{}, fmt.Errorf("%w: %s", ErrFileBackupNotFound, id)
		}
		data, err := os.ReadFile(filepath.Join(dir, record))
		if err != nil {
			return nil, FileBackupVersion{}, err
		}
		v := FileBackupVersion{ID: id, Kind: FileVersionOrigin, Time: at}
		fillOriginContent(&v, record, data)
		if v.None || v.LinkTo != "" {
			data = nil
		}
		return data, v, nil
	}
	kind, sub, base := FileVersionHistory, dir, id
	if rest, ok := strings.CutPrefix(id, "drift:"); ok {
		kind, sub, base = FileVersionDrift, filepath.Join(dir, "drift"), rest
	}
	if !fileBackupStamp.MatchString(base) {
		return nil, FileBackupVersion{}, fmt.Errorf("%w: %q", ErrInvalidFileBackupID, id)
	}
	data, err := os.ReadFile(filepath.Join(sub, base+".bak"))
	if os.IsNotExist(err) {
		return nil, FileBackupVersion{}, fmt.Errorf("%w: %s", ErrFileBackupNotFound, id)
	}
	if err != nil {
		return nil, FileBackupVersion{}, err
	}
	v := stampedVersion(kind, id, base)
	fillVersionContent(&v, data)
	if v.LinkTo != "" {
		data = nil
	}
	return data, v, nil
}

// firstLine returns the first non-empty line of data, cut to 120 characters.
func firstLine(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > 120 {
			line = string([]rune(line)[:120])
		}
		return line
	}
	return ""
}

// PathInside reports whether path is root or lies below it; project mode
// shows only the file history of paths inside the project.
func PathInside(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
