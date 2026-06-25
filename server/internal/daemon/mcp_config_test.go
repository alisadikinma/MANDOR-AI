package daemon

import (
	"reflect"
	"testing"
)

func TestParseDisabledMcpServers(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"nil", "", nil},
		{"null", "null", nil},
		{"empty object", "{}", nil},
		{"empty list", `{"disabledMcpServers":[]}`, []string{}},
		{"names", `{"disabledMcpServers":["github","figma"]}`, []string{"github", "figma"}},
		{"malformed", `not json`, nil},
		{"wrong type", `{"disabledMcpServers":"github"}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDisabledMcpServers([]byte(tt.in))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseDisabledMcpServers(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
