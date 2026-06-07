package daemon

import (
	"encoding/json"
	"testing"
)

func TestRuntimeMcpConfig(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // "" means a nil/empty result
	}{
		{
			name: "empty input passes through",
			in:   "",
			want: "",
		},
		{
			name: "no disabled key passes through verbatim",
			in:   `{"mcpServers":{"a":{"command":"x"}}}`,
			want: `{"mcpServers":{"a":{"command":"x"}}}`,
		},
		{
			name: "strips disabledMcpServers, keeps active servers",
			in:   `{"mcpServers":{"a":{"command":"x"}},"disabledMcpServers":{"b":{"command":"y"}}}`,
			want: `{"mcpServers":{"a":{"command":"x"}}}`,
		},
		{
			name: "all-disabled collapses to nil so the CLI default applies",
			in:   `{"disabledMcpServers":{"b":{"command":"y"}}}`,
			want: "",
		},
		{
			name: "a disabled entry wins over an active one of the same name",
			in:   `{"mcpServers":{"a":{"command":"x"},"b":{"command":"keep"}},"disabledMcpServers":{"a":{"command":"x"}}}`,
			want: `{"mcpServers":{"b":{"command":"keep"}}}`,
		},
		{
			name: "shadowing the only active server collapses to nil",
			in:   `{"mcpServers":{"a":{"command":"x"}},"disabledMcpServers":{"a":{"command":"x"}}}`,
			want: "",
		},
		{
			name: "non-object passes through untouched",
			in:   `"not-an-object"`,
			want: `"not-an-object"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var in json.RawMessage
			if tt.in != "" {
				in = json.RawMessage(tt.in)
			}
			got := runtimeMcpConfig(in)

			if tt.want == "" {
				if len(got) != 0 {
					t.Fatalf("expected empty result, got %s", got)
				}
				return
			}
			// Compare structurally so key ordering does not matter.
			var gotV, wantV any
			if err := json.Unmarshal(got, &gotV); err != nil {
				t.Fatalf("result is not valid JSON: %v (%s)", err, got)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantV); err != nil {
				t.Fatalf("bad want fixture: %v", err)
			}
			gotN, _ := json.Marshal(gotV)
			wantN, _ := json.Marshal(wantV)
			if string(gotN) != string(wantN) {
				t.Fatalf("runtimeMcpConfig() = %s, want %s", gotN, wantN)
			}
		})
	}
}
