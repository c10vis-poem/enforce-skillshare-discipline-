package sync

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Managed content block markers. prepend and append write the source file's
// content between them, for tools that do not follow @path lines. The source
// is named so several extras can share one target file, and the hash tells a
// hand edit from a source update.
const contentBlockEnd = "<!-- /skillshare:extra -->"

var contentBlockBeginRe = regexp.MustCompile(`^<!-- skillshare:extra src="([^"]+)" sha256=([0-9a-f]{16}) -->$`)

func contentBlockBegin(src, body string) string {
	return fmt.Sprintf(`<!-- skillshare:extra src="%s" sha256=%s -->`, filepath.ToSlash(src), contentBlockHash(body))
}

func contentBlockHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])[:16]
}

// isContentBlockMode reports a mode that keeps a managed content block.
func isContentBlockMode(mode string) bool { return mode == "prepend" || mode == "append" }

// contentBlock is one parsed block: its line range [start, end] and body.
type contentBlock struct {
	src, hash  string
	start, end int
	body       string
}

// parseContentBlocks finds managed blocks outside Markdown code: fenced
// blocks and lines indented four or more spaces are examples, not markers. Any
// marker problem makes the whole file malformed, so nothing in it is rewritten.
func parseContentBlocks(lines []string) ([]contentBlock, bool) {
	var out []contentBlock
	var open *contentBlock
	fenceChar, fenceLen := byte(0), 0
	for i, raw := range lines {
		raw = strings.TrimSuffix(raw, "\r")
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		line := strings.TrimSpace(raw)
		if open == nil && indent < 4 && !strings.HasPrefix(raw, "\t") {
			if c, n := fenceRun(line); n >= 3 {
				switch {
				case fenceLen == 0:
					fenceChar, fenceLen = c, n
					continue
				case c == fenceChar && n >= fenceLen && strings.TrimLeft(line, string(c)) == "":
					fenceChar, fenceLen = 0, 0
					continue
				}
			}
		}
		if fenceLen > 0 || indent >= 4 || strings.HasPrefix(raw, "\t") {
			continue
		}
		switch {
		case line == contentBlockEnd:
			if open == nil {
				return nil, false
			}
			open.end = i
			body := make([]string, 0, i-open.start-1)
			for _, l := range lines[open.start+1 : i] {
				body = append(body, strings.TrimSuffix(l, "\r"))
			}
			open.body = strings.Join(body, "\n")
			out = append(out, *open)
			open = nil
		case strings.HasPrefix(line, "<!-- skillshare:extra "):
			m := contentBlockBeginRe.FindStringSubmatch(line)
			if m == nil || open != nil {
				return nil, false
			}
			open = &contentBlock{src: m[1], start: i, hash: m[2]}
		}
	}
	return out, open == nil
}

// fenceRun returns the fence character and run length a line starts with.
func fenceRun(line string) (byte, int) {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return 0, 0
	}
	n := len(line) - len(strings.TrimLeft(line, line[:1]))
	if line[0] == '`' && strings.Contains(line[n:], "`") {
		return 0, 0 // a backtick fence's info string cannot contain backticks
	}
	return line[0], n
}

// blockSrc is the source name written into the block: the same path the
// import line would use, so the two modes name a source alike.
func (f ExtraFile) blockSrc() string {
	return filepath.ToSlash(strings.TrimPrefix(f.importLine(), "@"))
}

// ownsContentBlock matches a block written for this source by either of the
// names importLine can produce.
func (f ExtraFile) ownsContentBlock(b contentBlock) bool {
	return b.src == filepath.ToSlash(f.Source) || b.src == filepath.ToSlash(strings.TrimPrefix(f.relativeImportLine(), "@"))
}

// blockBody is the source content as it is written between the markers,
// without trailing line breaks so the end marker always sits on its own line.
func (f ExtraFile) blockBody() (string, error) {
	data, err := os.ReadFile(f.Source)
	if err != nil {
		return "", fmt.Errorf("failed to read source: %w", err)
	}
	return strings.TrimRight(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), nil
}

// findContentBlock returns this source's block in lines. It fails on damaged
// markers and when the source has more than one block.
func (f ExtraFile) findContentBlock(lines []string) (*contentBlock, error) {
	blocks, ok := parseContentBlocks(lines)
	if !ok {
		return nil, fmt.Errorf("%s has a damaged managed block; restore or repair it before syncing", f.Target)
	}
	var found *contentBlock
	for i := range blocks {
		if !f.ownsContentBlock(blocks[i]) {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("%s holds the block of %s twice; remove one before syncing", f.Target, f.Source)
		}
		found = &blocks[i]
	}
	return found, nil
}

func renderContentBlock(src, body, eol string) string {
	lines := append([]string{contentBlockBegin(src, body)}, strings.Split(body, "\n")...)
	lines = append(lines, contentBlockEnd)
	return strings.Join(lines, eol) + eol
}

// syncExtraBlock writes the source's content into the target's managed block:
// inserted at the top (prepend) or bottom (append) when absent, replaced in
// place when the source changed. Content outside the block is kept byte for
// byte. A block edited by hand is refused.
func syncExtraBlock(f ExtraFile, dryRun bool) (*ExtraResult, error) {
	return applyExtraBlock(f, dryRun, false)
}

// applyExtraBlock is syncExtraBlock; with overwriteEdited a hand-edited block
// is rewritten from the source instead of refused.
func applyExtraBlock(f ExtraFile, dryRun, overwriteEdited bool) (*ExtraResult, error) {
	result := &ExtraResult{Synced: 1}
	body, err := f.blockBody()
	if err != nil {
		return nil, err
	}
	attached := extraAttached(f.Target)

	// A link to the source is left over from symlink mode; writing through it
	// would edit the source, so it is replaced by a new file.
	ourLink := f.isOurLink()
	var data []byte
	if ourLink {
		data = []byte(extraRestoreBase(f.Target))
	} else {
		data, err = os.ReadFile(f.Target)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read target: %w", err)
		}
	}
	exists := ourLink || err == nil
	content := string(data)
	eol := "\n"
	if strings.Contains(content, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(content, "\n")
	found, err := f.findContentBlock(lines)
	if err != nil {
		return nil, err
	}

	var updated string
	switch {
	case found == nil:
		block := renderContentBlock(f.blockSrc(), body, eol)
		switch {
		case content == "":
			updated = block
		case f.Mode == "prepend":
			updated = block + eol + content
		default:
			if !strings.HasSuffix(content, "\n") {
				content += eol
			}
			updated = content + eol + block
		}
	case contentBlockHash(found.body) != found.hash && !overwriteEdited:
		return nil, fmt.Errorf("the managed block of %s in %s was edited by hand; collect the edit into the source or remove the block before syncing", f.Source, f.Target)
	case found.body == body && contentBlockHash(found.body) == found.hash:
		return result, nil
	default:
		block := strings.Split(strings.TrimSuffix(renderContentBlock(f.blockSrc(), body, eol), "\n"), "\n")
		next := append(append(append([]string{}, lines[:found.start]...), block...), lines[found.end+1:]...)
		updated = strings.Join(next, "\n")
	}
	if dryRun {
		return result, nil
	}

	if ourLink {
		if err := os.Remove(f.Target); err != nil {
			return nil, fmt.Errorf("failed to remove leftover symlink: %w", err)
		}
	}
	perm := os.FileMode(0644)
	if info, statErr := os.Stat(f.Target); statErr == nil {
		perm = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(f.Target), 0755); err != nil {
		return nil, fmt.Errorf("failed to create parent dir: %w", err)
	}
	if err := os.WriteFile(f.Target, []byte(updated), perm); err != nil {
		return nil, fmt.Errorf("failed to write target: %w", err)
	}
	clearExtraWritten(f.Target)
	if (!exists || ourLink) && !attached {
		if err := markExtraCreated(f.Target); err != nil {
			return nil, fmt.Errorf("failed to record created file: %w", err)
		}
	}
	return result, nil
}

// extraBlockStatus reports synced, modified (edited by hand) or drift for a
// content block target that exists.
func extraBlockStatus(f ExtraFile) string {
	data, err := os.ReadFile(f.Target)
	if err != nil || f.isOurLink() {
		return "drift"
	}
	found, err := f.findContentBlock(strings.Split(string(data), "\n"))
	if err != nil || found == nil {
		return "drift"
	}
	if contentBlockHash(found.body) != found.hash {
		return "modified"
	}
	if body, err := f.blockBody(); err == nil && body == found.body {
		return "synced"
	}
	return "drift"
}

// collectExtraBlock resolves a "modified" block target by keeping the edit: the
// block's body becomes the source (the old source is backed up), then the
// block is rewritten so its hash matches again. The rest of the target file is
// not touched.
func collectExtraBlock(f ExtraFile) error {
	data, err := os.ReadFile(f.Target)
	if err != nil {
		return fmt.Errorf("failed to read target: %w", err)
	}
	found, err := f.findContentBlock(strings.Split(string(data), "\n"))
	if err != nil {
		return err
	}
	if found == nil {
		return fmt.Errorf("%s has no managed block of %s to collect", f.Target, f.Source)
	}
	body := []byte(found.body + "\n")
	perm := os.FileMode(0644)
	if info, statErr := os.Stat(f.Source); statErr == nil {
		existing, _ := os.ReadFile(f.Source)
		if !bytes.Equal(existing, body) {
			if err := backupExtraFile(f.Source, BackupReasonCollect); err != nil {
				return err
			}
		}
		perm = info.Mode().Perm()
	}
	if err := os.WriteFile(f.Source, body, perm); err != nil {
		return fmt.Errorf("failed to write source: %w", err)
	}
	_, err = applyExtraBlock(f, false, true)
	return err
}

// reapplyExtraBlock resolves a "modified" block target by keeping the source:
// the edited file is kept as a drift backup, then the block is rewritten.
func reapplyExtraBlock(f ExtraFile) error {
	if err := backupExtraDrift(f.Target, DriftReasonOverwrite); err != nil {
		return err
	}
	_, err := applyExtraBlock(f, false, true)
	return err
}

// removeContentBlock returns content without this source's block and the
// blank line that separates it from the rest, and whether anything was removed.
func (f ExtraFile) removeContentBlock(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	found, err := f.findContentBlock(lines)
	if err != nil || found == nil {
		return content, false
	}
	start, end := found.start, found.end+1
	switch {
	case end < len(lines)-1 && strings.TrimSpace(lines[end]) == "":
		end++ // prepend: the blank line after the block
	case start > 0 && strings.TrimSpace(lines[start-1]) == "":
		start-- // append: the blank line before it
	}
	return strings.Join(append(append([]string{}, lines[:start]...), lines[end:]...), "\n"), true
}

// restoreExtraBlock removes this source's block when the target is detached.
// A file skillshare created is deleted once nothing else is left in it.
func restoreExtraBlock(f ExtraFile) (bool, error) {
	data, err := os.ReadFile(f.Target)
	if os.IsNotExist(err) {
		clearExtraAttach(f.Target)
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read target: %w", err)
	}
	updated, changed := f.removeContentBlock(string(data))
	if !changed {
		return false, nil
	}
	if strings.TrimSpace(updated) == "" && extraAttached(f.Target) {
		if err := os.Remove(f.Target); err != nil {
			return false, fmt.Errorf("failed to remove target: %w", err)
		}
		return true, putBackExtraRestorePoint(f.Target)
	}
	info, err := os.Stat(f.Target)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(f.Target, []byte(updated), info.Mode().Perm()); err != nil {
		return false, fmt.Errorf("failed to write target: %w", err)
	}
	return true, nil
}
