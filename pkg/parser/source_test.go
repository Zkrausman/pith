package parser

import "testing"

func TestSourceParserQuotedURLs(t *testing.T) {
	p := &SourceParser{}
	for _, tc := range []struct {
		name, input, want string
	}{
		{
			name:  "https literal",
			input: `endpoint := "https://example.invalid/api"`,
			want:  `endpoint := "https://example.invalid/api"`,
		},
		{
			name:  "http literal",
			input: `endpoint := "http://example.invalid/api"`,
			want:  `endpoint := "http://example.invalid/api"`,
		},
		{
			name:  "multiple URL literals",
			input: `urls := []string{"http://example.invalid/a", "https://example.invalid/b"}`,
			want:  `urls := []string{"http://example.invalid/a", "https://example.invalid/b"}`,
		},
		{
			name:  "URL path and fragment with ordinary inline comment",
			input: `endpoint := "https://example.invalid/a//b#section" // ordinary comment`,
			want:  `endpoint := "https://example.invalid/a//b#section"`,
		},
		{
			name:  "ordinary hash comment after URL",
			input: `endpoint = "http://example.invalid/api" # generic comment`,
			want:  `endpoint = "http://example.invalid/api"`,
		},
		{
			name:  "keyword in URL does not make ordinary comment high-signal",
			input: `endpoint := "https://example.invalid/TODO" // generic comment`,
			want:  `endpoint := "https://example.invalid/TODO"`,
		},
		{
			name:  "high-signal inline comment after URL",
			input: `endpoint := "https://example.invalid/api" // TODO: keep this`,
			want:  `endpoint := "https://example.invalid/api" // TODO: keep this`,
		},
		{
			name:  "ordinary and high-signal standalone comments",
			input: "// generic comment\n// BUG: keep this\n# generic text\n# NOTE: keep this\nvalue := 1 // generic inline",
			want:  "// BUG: keep this\n# NOTE: keep this\nvalue := 1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.Parse(tc.input); got != tc.want {
				t.Errorf("Parse(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
