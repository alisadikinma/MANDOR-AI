package handler

import (
	"encoding/json"
	"testing"
)

// TestMcpConnectorSeedParses asserts the embedded curated catalog parses into
// the seed struct and contains real, well-formed entries — every connector
// must have a slug, a name, a non-empty mcp_template carrying an mcpServers
// fragment, and an input_schema. This is the M1 contract: the seed is real
// data, not stubs.
func TestMcpConnectorSeedParses(t *testing.T) {
	if len(mcpConnectorSeed) == 0 {
		t.Fatal("expected at least one seeded connector")
	}

	seen := make(map[string]bool)
	for _, c := range mcpConnectorSeed {
		if c.Slug == "" {
			t.Fatalf("connector %q has empty slug", c.Name)
		}
		if seen[c.Slug] {
			t.Fatalf("duplicate connector slug %q in seed", c.Slug)
		}
		seen[c.Slug] = true

		if c.Name == "" {
			t.Fatalf("connector %q has empty name", c.Slug)
		}

		// mcp_template must carry a real mcpServers fragment with a token.
		var tmpl struct {
			McpServers map[string]json.RawMessage `json:"mcpServers"`
		}
		if err := json.Unmarshal(c.McpTemplate, &tmpl); err != nil {
			t.Fatalf("connector %q mcp_template is not valid JSON: %v", c.Slug, err)
		}
		if len(tmpl.McpServers) == 0 {
			t.Fatalf("connector %q mcp_template has no mcpServers entries", c.Slug)
		}

		// input_schema must be a valid JSON object (form renderer consumes it).
		var schema map[string]json.RawMessage
		if err := json.Unmarshal(c.InputSchema, &schema); err != nil {
			t.Fatalf("connector %q input_schema is not a JSON object: %v", c.Slug, err)
		}
	}

	// The curated catalog must include the headline connectors the directory
	// ships with.
	for _, want := range []string{"github", "slack", "notion", "figma", "atlassian", "gmail", "microsoft-365"} {
		if !seen[want] {
			t.Errorf("expected seeded connector %q to be present", want)
		}
	}
}
