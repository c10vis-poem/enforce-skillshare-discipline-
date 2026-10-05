//go:build windows

package utils

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// ioReparseTagMountPoint is the reparse tag of a junction (IO_REPARSE_TAG_MOUNT_POINT).
const ioReparseTagMountPoint = 0xA0000003

// platformIsJunction reports whether path is a junction. Other reparse points,
// such as OneDrive placeholders or deduplicated files, are not links.
func platformIsJunction(path string) bool {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	var data syscall.Win32finddata
	h, err := syscall.FindFirstFile(ptr, &data)
	if err != nil {
		return false
	}
	syscall.FindClose(h)
	// For a reparse point, FindFirstFile reports its tag in Reserved0.
	return data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 && data.Reserved0 == ioReparseTagMountPoint
}

// CreateJunction creates a directory junction (no admin required). It never
// falls back to a symlink, so callers can restore a junction exactly.
func CreateJunction(linkPath, sourcePath string) error {
	var stderr bytes.Buffer
	cmd := exec.Command("cmd")
	// Go quotes an argument only when it has whitespace, so a path with
	// cmd metacharacters such as & would split into a second command. A
	// Windows path cannot contain a double quote, so quoting both is safe.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `cmd /c mklink /J "` + linkPath + `" "` + sourcePath + `"`,
	}
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mklink /J: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
