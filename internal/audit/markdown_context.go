package audit

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

type markdownCodeBlock struct {
	start, end int // zero-based source lines, excluding fence markers
	language   string
	lines      []string
	inHTML     bool
}

// Markdown structure supplies context, not trust. Keep the original source
// lines for static findings, and parsed block contents for shell analysis.
func markdownCodeBlocks(content []byte) []markdownCodeBlock {
	var blocks []markdownCodeBlock
	doc := goldmark.DefaultParser().Parse(text.NewReader(content))
	offset, line := 0, 0
	rendered := make(map[int]bool)
	hasHTML := false
	collect := func(doc ast.Node, security bool) {
		_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			if _, ok := node.(*ast.HTMLBlock); ok {
				hasHTML = true
				return ast.WalkSkipChildren, nil
			}
			block, ok := node.(*ast.FencedCodeBlock)
			if !ok || block.Lines().Len() == 0 {
				return ast.WalkContinue, nil
			}
			first := block.Lines().At(0)
			start := first.Start
			line += bytes.Count(content[offset:start], []byte{'\n'})
			offset = start
			lines := make([]string, block.Lines().Len())
			for i := range lines {
				segment := block.Lines().At(i)
				lines[i] = strings.TrimSuffix(string(segment.Value(content)), "\n")
			}
			if !security {
				rendered[start] = true
			}
			language := ""
			if fields := strings.Fields(string(block.Language(content))); len(fields) > 0 {
				language = strings.ToLower(fields[0])
			}
			blocks = append(blocks, markdownCodeBlock{
				start: line, end: line + len(lines),
				language: language, lines: lines,
				inHTML: security && !rendered[start],
			})
			return ast.WalkSkipChildren, nil
		})
	}
	collect(doc, false)
	if hasHTML {
		// Parse the whole source without HTML blocks so blank lines and HTML
		// boundaries cannot split a fenced shell flow or hide its closing fence.
		parsers := parser.DefaultBlockParsers()
		for i, p := range parsers {
			if p.Value == parser.NewHTMLBlockParser() {
				parsers = append(parsers[:i], parsers[i+1:]...)
				break
			}
		}
		securityParser := parser.NewParser(parser.WithBlockParsers(parsers...))
		blocks, offset, line = nil, 0, 0
		collect(securityParser.Parse(text.NewReader(content)), true)
	}
	return blocks
}
