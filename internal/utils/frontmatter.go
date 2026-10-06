package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseSkillName reads the SKILL.md and extracts the "name" from frontmatter.
func ParseSkillName(skillPath string) (string, error) {
	data, err := readHead(filepath.Join(skillPath, "SKILL.md"))
	if err != nil {
		return "", err
	}

	for raw := range bytes.Lines(locateFrontmatter(data, lenientBlock).raw) {
		line := strings.TrimSpace(string(raw))
		if strings.HasPrefix(line, "name:") {
			// Extract value: "name: my-skill" -> "my-skill"
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[1])
				// Remove quotes if present
				name = strings.Trim(name, `"'`)
				return name, nil
			}
		}
	}

	return "", nil // Name not found
}

// isYAMLBlockIndicator returns true for YAML block scalar indicators (>, >-, >+, |, |-, |+).
func isYAMLBlockIndicator(s string) bool {
	switch s {
	case ">", ">-", ">+", "|", "|-", "|+":
		return true
	}
	return false
}

// resolveField looks up a field in the frontmatter map.
// Priority: metadata.<field> > top-level <field>.
// Returns nil when the field is absent in both locations.
func resolveField(fm map[string]any, field string) any {
	if md, ok := fm["metadata"]; ok {
		if mdMap, ok := md.(map[string]any); ok {
			if val, ok := mdMap[field]; ok {
				return val
			}
		}
	}
	val, ok := fm[field]
	if !ok {
		return nil
	}
	return val
}

// ParseFrontmatterList reads a SKILL.md file and extracts a YAML list field from frontmatter.
// Supports both inline [a, b] and block (- a\n- b) formats.
// Returns nil when the field is absent or the file cannot be read.
func ParseFrontmatterList(filePath, field string) []string {
	data, err := readHead(filePath)
	if err != nil {
		return nil
	}
	return ParseFrontmatterListFromBytes(data, field)
}

// ParseFrontmatterListFromBytes parses a YAML list field from pre-read content.
// Same as ParseFrontmatterList but avoids re-reading the file.
func ParseFrontmatterListFromBytes(content []byte, field string) []string {
	list, _ := resolveField(lenientFrontmatterMap(content), field).([]any)
	var result []string
	for _, item := range list {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// lenientFrontmatterMap decodes the block the lenient readers see. It is nil when there
// is no block or the block is not a YAML mapping.
func lenientFrontmatterMap(content []byte) map[string]any {
	var fm map[string]any
	if err := yaml.Unmarshal(locateFrontmatter(content, lenientBlock).withoutLastNewline(), &fm); err != nil {
		return nil
	}
	return fm
}

// ParseFrontmatterMap returns the complete YAML frontmatter of SKILL.md content.
// The Agent Skills format requires the file to begin with it, closed by a second ---.
func ParseFrontmatterMap(content []byte) (map[string]any, error) {
	block := locateFrontmatter(content, strictBlock)
	if !block.open {
		return nil, fmt.Errorf("no frontmatter at the start")
	}
	if !block.closed {
		return nil, fmt.Errorf("unclosed frontmatter: no closing ---")
	}
	if len(bytes.TrimSpace(block.raw)) == 0 {
		return nil, fmt.Errorf("no frontmatter")
	}
	var fm map[string]any
	if err := yaml.Unmarshal(block.raw, &fm); err != nil {
		return nil, fmt.Errorf("invalid frontmatter: %w", err)
	}
	// A non-string key decodes to map[any]any, which JSON cannot carry.
	if _, err := json.Marshal(fm); err != nil {
		return nil, fmt.Errorf("frontmatter JSON cannot carry: %w", err)
	}
	return fm, nil
}

// ParseFrontmatterFields reads a SKILL.md file once and returns the values of
// multiple frontmatter fields. This avoids opening the same file repeatedly
// when multiple fields are needed (e.g. description + license).
// Note: does not resolve metadata.<field> — only reads top-level fields.
func ParseFrontmatterFields(filePath string, fields []string) map[string]string {
	result := make(map[string]string, len(fields))
	if len(fields) == 0 {
		return result
	}

	data, err := readHead(filePath)
	if err != nil {
		return result
	}
	fm := lenientFrontmatterMap(data)

	for _, field := range fields {
		val, ok := fm[field]
		if !ok || val == nil {
			continue
		}
		switch v := val.(type) {
		case string:
			result[field] = v
		case int:
			result[field] = fmt.Sprintf("%d", v)
		case float64:
			result[field] = fmt.Sprintf("%g", v)
		case bool:
			result[field] = fmt.Sprintf("%t", v)
		}
	}

	return result
}

// ReadSkillBody reads a file and returns everything after the YAML frontmatter.
// If no frontmatter is present, the entire content is returned.
// Returns "" on read error.
func ReadSkillBody(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}

	block := locateFrontmatter(scanLines(data), bodyBlock)
	if !block.open {
		return strings.TrimSpace(string(data))
	}
	if !block.closed {
		return ""
	}
	return strings.TrimSpace(string(block.body))
}

// ParseFrontmatterField reads a SKILL.md file and extracts the value of a given frontmatter field.
// It supports both inline values and YAML block scalars (>, >-, |, |-).
func ParseFrontmatterField(filePath, field string) string {
	data, err := readHead(filePath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(locateFrontmatter(data, lenientBlock).raw), "\n")
	prefix := field + ":"

	for i, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, prefix) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				// Handle YAML block scalar indicators — read indented continuation lines
				if isYAMLBlockIndicator(val) {
					var blockParts []string
					for _, next := range lines[i+1:] {
						// Block continues while lines are indented
						if len(next) > 0 && (next[0] == ' ' || next[0] == '\t') {
							blockParts = append(blockParts, strings.TrimSpace(next))
						} else {
							break
						}
					}
					return strings.Join(blockParts, " ")
				}
				val = strings.Trim(val, `"'`)
				return val
			}
		}
	}

	return ""
}
