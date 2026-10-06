package utils

import (
	"bufio"
	"bytes"
	"io"
	"os"
)

// frontmatterPolicy is the rule one reader uses to find the --- delimiters. The readers
// do not agree on it (issue #449), so each difference is a field here and each reader
// names its policy below.
type frontmatterPolicy struct {
	// skipBOM ignores a UTF-8 byte order mark before the opening delimiter.
	skipBOM bool
	// dashGuard gives up unless the content, after leading whitespace, starts with ---.
	// The delimiter itself may still be a later line.
	dashGuard bool
	// firstLine requires the opening delimiter to be the first line. Otherwise the first
	// delimiter line anywhere in the content opens the block.
	firstLine bool
	// delim reports whether a line, without its "\n", is a delimiter.
	delim func(line []byte) bool
}

var (
	// strictBlock is the Agent Skills format: the file begins with exactly ---, and the
	// block ends at the next line that is exactly ---. CRLF is fine. ParseFrontmatterMap.
	strictBlock = frontmatterPolicy{skipBOM: true, firstLine: true, delim: delimExact}

	// lenientBlock takes the first two lines that are --- after trimming all whitespace,
	// wherever they are. ParseSkillName, ParseFrontmatterList, ParseFrontmatterListFromBytes,
	// ParseFrontmatterFields, ParseFrontmatterField.
	lenientBlock = frontmatterPolicy{delim: delimTrimmed}

	// bodyBlock is lenientBlock for content that starts with ---. ReadSkillBody.
	bodyBlock = frontmatterPolicy{dashGuard: true, delim: delimTrimmed}

	// rewriteBlock allows trailing spaces and tabs but no indent and no "\r", so that an
	// indented --- inside a YAML block scalar stays content. splitFrontmatterAndBody.
	rewriteBlock = frontmatterPolicy{dashGuard: true, delim: delimColumn0}
)

func delimExact(line []byte) bool   { return string(bytes.TrimRight(line, "\r")) == "---" }
func delimTrimmed(line []byte) bool { return string(bytes.TrimSpace(line)) == "---" }
func delimColumn0(line []byte) bool { return string(bytes.TrimRight(line, " \t")) == "---" }

// frontmatterBlock is where the frontmatter sits in the content. raw and body point into
// the content; nothing is copied.
type frontmatterBlock struct {
	open   bool   // an opening delimiter was found
	closed bool   // and a closing one
	raw    []byte // the lines between the delimiters; up to the end of the content when unclosed
	body   []byte // everything after the closing delimiter line; nil when unclosed
}

// joined returns raw the way the readers that split the content into lines rebuilt it:
// joined with "\n", which leaves a closed block without its final newline. A block
// scalar on the last line decodes with or without a trailing newline accordingly, so
// ParseFrontmatterMap, which never split, decodes raw instead.
func (b frontmatterBlock) joined() []byte {
	if b.closed {
		return bytes.TrimSuffix(b.raw, []byte("\n"))
	}
	return b.raw
}

// locateFrontmatter finds the frontmatter block under the given policy. It walks the
// lines only as far as the closing delimiter and allocates nothing, so a large body
// costs no memory. Whether an unclosed block counts is the caller's decision.
func locateFrontmatter(content []byte, p frontmatterPolicy) frontmatterBlock {
	if p.skipBOM {
		content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	}
	if p.dashGuard && !bytes.HasPrefix(bytes.TrimSpace(content), []byte("---")) {
		return frontmatterBlock{}
	}

	start, pos := -1, 0
	for line := range bytes.Lines(content) {
		next := pos + len(line)
		isDelim := p.delim(bytes.TrimSuffix(line, []byte("\n")))
		switch {
		case start >= 0 && isDelim:
			return frontmatterBlock{open: true, closed: true, raw: content[start:pos], body: content[next:]}
		case isDelim:
			start = next
		case start < 0 && p.firstLine:
			return frontmatterBlock{}
		}
		pos = next
	}
	if start < 0 {
		return frontmatterBlock{}
	}
	return frontmatterBlock{open: true, raw: content[start:]}
}

// readScannable opens a file for the readers that take a path. Only a failed open is an
// error: a failed read ends the content early, as it did when they read through a
// bufio.Scanner.
func readScannable(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	return scannable(data), nil
}

// scannable returns the text a default bufio.Scanner delivers from data line by line,
// joined with "\n": it stops, without an error, before the first line of
// bufio.MaxScanTokenSize bytes or more, each line loses one trailing "\r", and the last
// line has no newline. The readers that take a path have always seen only this; the ones
// that take bytes (ParseFrontmatterListFromBytes, ParseFrontmatterMap,
// RewriteFrontmatterList) see the content as it is.
func scannable(data []byte) []byte {
	pos := 0
	for line := range bytes.Lines(data) {
		if len(bytes.TrimSuffix(line, []byte("\n"))) >= bufio.MaxScanTokenSize {
			data = data[:pos]
			break
		}
		pos += len(line)
	}
	if bytes.IndexByte(data, '\r') >= 0 {
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	}
	if rest, ok := bytes.CutSuffix(data, []byte("\n")); ok {
		return rest
	}
	return bytes.TrimSuffix(data, []byte("\r"))
}
