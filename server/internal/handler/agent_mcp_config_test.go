package handler

import "testing"

func TestValidateAgentMcpConfig(t *testing.T) {
	ok := []string{
		``,
		`null`,
		`{}`,
		`{"disabledMcpServers":[]}`,
		`{"disabledMcpServers":["github","figma"]}`,
	}
	for _, in := range ok {
		if msg, valid := validateAgentMcpConfig([]byte(in)); !valid {
			t.Errorf("expected %q valid, got error %q", in, msg)
		}
	}

	bad := []string{
		`{"mcpServers":{"github":{"command":"npx"}}}`,           // server definitions no longer allowed
		`{"disabledMcpServers":["x"],"mcpServers":{}}`,          // mixed
		`{"disabledMcpServers":"github"}`,                       // not an array
		`{"disabledMcpServers":[1,2]}`,                          // not strings
		`[]`,                                                    // not an object
		`{"somethingElse":true}`,                               // unknown key
	}
	for _, in := range bad {
		if _, valid := validateAgentMcpConfig([]byte(in)); valid {
			t.Errorf("expected %q rejected, but it passed", in)
		}
	}
}
