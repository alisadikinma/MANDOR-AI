package handler

import (
	_ "embed"
	"encoding/json"
)

// mcpConnectorsSeedJSON is the curated, global MCP connector catalog. It is
// embedded at build time (mirroring reserved_slugs.json) and parsed once into
// mcpConnectorSeed. A parse failure is a programming error — the JSON is
// checked into the repo and shipped inside the binary — so we panic at init
// rather than let a malformed catalog surface as a runtime error on the first
// list request.
//
//go:embed mcp_connectors_seed.json
var mcpConnectorsSeedJSON []byte

// mcpConnectorSeedEntry is one curated connector from the embedded catalog.
// input_schema and mcp_template are kept as raw JSON: input_schema is consumed
// verbatim by the frontend form renderer, and mcp_template is rendered against
// the user's answers and deep-merged into the agent's mcp_config — neither
// needs a Go-side struct shape.
type mcpConnectorSeedEntry struct {
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	Icon        string          `json:"icon"`
	Description string          `json:"description"`
	Popularity  int32           `json:"popularity"`
	InputSchema json.RawMessage `json:"input_schema"`
	McpTemplate json.RawMessage `json:"mcp_template"`
}

type mcpConnectorsSeedFile struct {
	Connectors []mcpConnectorSeedEntry `json:"connectors"`
}

var mcpConnectorSeed = loadMcpConnectorSeed()

func loadMcpConnectorSeed() []mcpConnectorSeedEntry {
	var file mcpConnectorsSeedFile
	if err := json.Unmarshal(mcpConnectorsSeedJSON, &file); err != nil {
		panic("handler: parse mcp_connectors_seed.json: " + err.Error())
	}
	if len(file.Connectors) == 0 {
		panic("handler: mcp_connectors_seed.json contains no connectors")
	}
	for _, c := range file.Connectors {
		if c.Slug == "" || c.Name == "" {
			panic("handler: mcp_connectors_seed.json entry missing slug or name")
		}
		if len(c.McpTemplate) == 0 {
			panic("handler: mcp_connectors_seed.json entry " + c.Slug + " missing mcp_template")
		}
	}
	return file.Connectors
}
