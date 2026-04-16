package app

import (
	"testing"
)

func TestStripDeferLoading(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no tools",
			input:    `{"model":"gpt-4"}`,
			expected: `{"model":"gpt-4"}`,
		},
		{
			name:     "tools without defer_loading",
			input:    `{"tools":[{"name":"t1"},{"name":"t2"}]}`,
			expected: `{"tools":[{"name":"t1"},{"name":"t2"}]}`,
		},
		{
			name:     "single tool with defer_loading",
			input:    `{"tools":[{"name":"t1","defer_loading":true}]}`,
			expected: `{"tools":[{"name":"t1"}]}`,
		},
		{
			name:     "multiple tools with defer_loading",
			input:    `{"tools":[{"name":"t1","defer_loading":true},{"name":"t2","defer_loading":false},{"name":"t3","defer_loading":true}]}`,
			expected: `{"tools":[{"name":"t1"},{"name":"t2"},{"name":"t3"}]}`,
		},
		{
			name:     "mixed tools with and without defer_loading",
			input:    `{"tools":[{"name":"t1"},{"name":"t2","defer_loading":true},{"name":"t3"}]}`,
			expected: `{"tools":[{"name":"t1"},{"name":"t2"},{"name":"t3"}]}`,
		},
		{
			name:     "tools array with other fields",
			input:    `{"model":"gpt-4","tools":[{"name":"search","description":"Search tool","defer_loading":true,"parameters":{"type":"object"}}]}`,
			expected: `{"model":"gpt-4","tools":[{"name":"search","description":"Search tool","parameters":{"type":"object"}}]}`,
		},
		{
			name:     "empty tools array",
			input:    `{"tools":[]}`,
			expected: `{"tools":[]}`,
		},
		{
			name:     "nested defer_loading in other places (should not be touched)",
			input:    `{"data":{"defer_loading":true},"tools":[{"name":"t1"}]}`,
			expected: `{"data":{"defer_loading":true},"tools":[{"name":"t1"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripDeferLoading([]byte(tt.input))
			if string(result) != tt.expected {
				t.Errorf("StripDeferLoading() = %v, want %v", string(result), tt.expected)
			}
		})
	}
}
