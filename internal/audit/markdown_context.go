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
	var htmlParser parser.Parser
	var collect func(ast.Node, []byte, int, bool)
	collect = func(doc ast.Node, source []byte, baseOffset int, inHTML bool) {
		_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			if html, ok := node.(*ast.HTMLBlock); ok && !inHTML && html.Lines().Len() > 0 {
				// HTML rendering must not hide shell examples from security analysis.
				if htmlParser == nil {
					parsers := parser.DefaultBlockParsers()
					for i, p := range parsers {
						if p.Value == parser.NewHTMLBlockParser() {
							parsers = append(parsers[:i], parsers[i+1:]...)
							break
						}
					}
					htmlParser = parser.NewParser(parser.WithBlockParsers(parsers...))
				}
				start := html.Lines().At(0).Start
				end := html.Lines().At(html.Lines().Len() - 1).Stop
				if html.HasClosure() {
					end = html.ClosureLine.Stop
				}
				raw := source[start:end]
				collect(htmlParser.Parse(text.NewReader(raw)), raw, baseOffset+start, true)
				return ast.WalkSkipChildren, nil
			}
			block, ok := node.(*ast.FencedCodeBlock)
			if !ok || block.Lines().Len() == 0 {
				return ast.WalkContinue, nil
			}
			first := block.Lines().At(0)
			start := baseOffset + first.Start
			line += bytes.Count(content[offset:start], []byte{'\n'})
			offset = start
			lines := make([]string, block.Lines().Len())
			for i := range lines {
				segment := block.Lines().At(i)
				lines[i] = strings.TrimSuffix(string(segment.Value(source)), "\n")
			}
			blocks = append(blocks, markdownCodeBlock{
				start: line, end: line + len(lines),
				language: strings.ToLower(string(block.Language(source))), lines: lines, inHTML: inHTML,
			})
			return ast.WalkSkipChildren, nil
		})
	}
	collect(doc, content, 0, false)
	return blocks
}
