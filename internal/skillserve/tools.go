package skillserve

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tools for clients without the Skills extension, which is most of them: the
// model lists and reads skills itself. Content read this way is ordinary text,
// not a loaded skill, so the client's own skill approval does not apply.
// A client that declares the extension sees no tools and loads skills natively.

// listLimit caps one list_skills answer so a large catalog does not flood the
// model's context; the query narrows it.
const listLimit = 200

type listToolInput struct {
	Query string `json:"query,omitempty" jsonschema:"Only skills whose name or description contains this text, ignoring case"`
}

type readToolInput struct {
	URI string `json:"uri" jsonschema:"A skill:// URI from list_skills, or one of the files read_skill lists for a skill"`
}

func (s *service) addTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_skills",
		Description: "List the available skills: name, when to use it, and its URI. Read a skill with read_skill before following it.",
	}, s.listTool)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_skill",
		Description: "Read a skill's SKILL.md, or one of its other files, by skill:// URI. Reading SKILL.md also lists the skill's other files.",
	}, s.readTool)
	srv.AddReceivingMiddleware(hideToolsFromNativeClients)
}

// hideToolsFromNativeClients leaves a client that declares the Skills
// extension without the tools, so it does not see each skill twice.
func hideToolsFromNativeClients(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		res, err := next(ctx, method, req)
		if r, ok := req.(*mcp.ListToolsRequest); ok && err == nil {
			if caps := r.ClientCapabilities(); caps != nil {
				if _, native := caps.Extensions[extensionID]; native {
					res.(*mcp.ListToolsResult).Tools = []*mcp.Tool{}
				}
			}
		}
		return res, err
	}
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func (s *service) listTool(_ context.Context, _ *mcp.CallToolRequest, in listToolInput) (*mcp.CallToolResult, any, error) {
	c, err := s.catalog()
	if err != nil {
		return nil, nil, err
	}
	query := strings.ToLower(in.Query)
	var b strings.Builder
	n := 0
	for _, skill := range c.Skills {
		name, _ := skill.Frontmatter["name"].(string)
		desc, _ := skill.Frontmatter["description"].(string)
		if query != "" && !strings.Contains(strings.ToLower(name+"\n"+desc), query) {
			continue
		}
		if n++; n <= listLimit {
			fmt.Fprintf(&b, "- %s: %s\n  %s\n", name, strings.Join(strings.Fields(desc), " "), skill.URI)
		}
	}
	switch {
	case n == 0:
		return textResult("No skills match."), nil, nil
	case n > listLimit:
		fmt.Fprintf(&b, "\n%d more skills match; pass a query to narrow the list.\n", n-listLimit)
	}
	return textResult(b.String()), nil, nil
}

func (s *service) readTool(_ context.Context, _ *mcp.CallToolRequest, in readToolInput) (*mcp.CallToolResult, any, error) {
	c, err := s.catalog()
	if err != nil {
		return nil, nil, err
	}
	data, err := c.Read(in.URI)
	if errors.Is(err, errNotFound) {
		return nil, nil, fmt.Errorf("no skill file %s; list_skills gives the skill URIs", in.URI)
	}
	if err != nil {
		return nil, nil, err
	}
	if !utf8.Valid(data) {
		return nil, nil, fmt.Errorf("%s is a binary file", in.URI)
	}
	text := string(data)
	if skill, ok := c.Skill(in.URI); ok && len(skill.Resources) > 1 {
		var b strings.Builder
		b.WriteString(text)
		b.WriteString("\n\n---\nOther files in this skill (read them with read_skill):\n")
		for _, f := range skill.Resources {
			if f.URI != skill.URI {
				fmt.Fprintf(&b, "- %s\n", f.URI)
			}
		}
		text = b.String()
	}
	return textResult(text), nil, nil
}
