package memory

import "os"

// commitNote publishes a prepared draft after backup and temporary-file writes.
func commitNote(root, rel, draft, version string) error {
	path, err := notePath(root, rel)
	if err != nil {
		return err
	}
	if version == "" {
		// Another process may have created this note since the initial read.
		return createNote(path, draft)
	}
	latest, err := Read(root, rel)
	if os.IsNotExist(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if latest.Version != version {
		return ErrConflict
	}
	return os.Rename(draft, path)
}

func createNote(path, draft string) error {
	data, err := os.ReadFile(draft)
	if err != nil {
		return err
	}
	err = createExclusive(path, func(file *os.File) error {
		if err := file.Chmod(0644); err != nil {
			return err
		}
		_, err := file.Write(data)
		return err
	})
	if os.IsExist(err) {
		return ErrConflict
	}
	return err
}

// createExclusive creates path and fills it with write. It fails with an
// os.IsExist error, leaving the file alone, when path already exists; a file
// it created but could not finish is removed.
func createExclusive(path string, write func(*os.File) error) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	err = write(file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

// removeNote commits a deletion after the backup has completed.
func removeNote(root, rel, version string) error {
	path, err := notePath(root, rel)
	if err != nil {
		return err
	}
	latest, err := Read(root, rel)
	if os.IsNotExist(err) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if version == "" || latest.Version != version {
		return ErrConflict
	}
	return os.Remove(path)
}
