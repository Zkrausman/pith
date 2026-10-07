package parser

import (
	"strings"
	"testing"
)

func TestStructuredContentFidelity(t *testing.T) {
	tests := []struct{ name, input, want string }{
		{"integer", ` { "id": 9007199254740993, "negative": -9007199254740993 } `, `{"id":9007199254740993,"negative":-9007199254740993}`},
		{"numbers", `[ -0, -0.0, 1e+09, 1E-09, 1e9999, 123456789012345678901234567890 ]`, `[-0,-0.0,1e+09,1E-09,1e9999,123456789012345678901234567890]`},
		{"strings", ` { "s": "two  spaces\t\n\u0020\u00e9雪\\path\"quote", "<>&": "\/" } `, `{"s":"two  spaces\t\n\u0020\u00e9雪\\path\"quote","<>&":"\/"}`},
		{"nested", "{\n \"a\": [ {\"b\": true}, null, [false] ], \"a\": 2\n}", `{"a":[{"b":true},null,[false]],"a":2}`},
		{"scalar", ` "a  b" `, `"a  b"`},
		{"large", ` { "data": "` + strings.Repeat("a  b", 400) + `", "last": 9007199254740993 } `, `{"data":"` + strings.Repeat("a  b", 400) + `","last":9007199254740993}`},
	}
	for _, p := range []Parser{&MinifyParser{}, &WebParser{}} {
		for _, tt := range tests {
			t.Run(p.Name()+"/"+tt.name, func(t *testing.T) {
				if got := p.Parse(tt.input); got != tt.want {
					t.Fatalf("got %q; want %q", got, tt.want)
				}
				if got := p.Parse(tt.want); got != tt.want {
					t.Fatal("compacted output not idempotent")
				}
			})
		}
	}
}

func TestStructuredContentPassthrough(t *testing.T) {
	inputs := []string{
		"", " \t\r\n", " {\"x\": 1, } \n", "{\"x\": 1}\n{\"x\": 2}",
		"{\n// comment\n\"x\": 1\n}", "{\"x\": \"unterminated  string", "[01]", "[NaN]",
		"{\"x\": \"a\x00b\"}", "{\"x\":1}\x00", "{\"x\":\"\xff\"}",
		"\ufeff{\"x\": 1}", "\u00a0{\"x\": 1}\u00a0",
		"/* keep comment */\na::before { content: \"two  spaces\"; }\n",
		"<root value=\"a  b\">  text </root>\n",
		"HTTP/1.1 200 OK\r\n\r\n{\"id\": 9007199254740993}",
		"HTTP/1.1 200 OK\r\n\r\n{\"html\":\"<html><title>Not a page</title>",
		"banner\n{\"html\":\"<!DOCTYPE html><html><title>Not a page</title>",
		" {\"id\": 9007199254740993}\nwarning from stderr\n",
		"{\"data\":\"" + strings.Repeat("long  value", 200) + "\x00\"}",
		"true " + strings.Repeat("extra ", 200),
		"123 " + strings.Repeat("extra ", 200),
		"{\"html\":\"<html><title>Not a page</title>" + strings.Repeat("x", 1200),
		strings.Repeat("[", 10001) + "0" + strings.Repeat("]", 10001),
	}
	for _, p := range []Parser{&MinifyParser{}, &WebParser{}} {
		for i, input := range inputs {
			if got := p.Parse(input); got != input {
				t.Errorf("%s case %d modified unsupported input: got %q", p.Name(), i, got)
			}
		}
	}
}

// Web's deliberate, labelled summaries are separate from JSON compaction.
// Minify has no such summary contract and preserves these formats byte for byte.
func TestStructuredContentIntentionalSummaries(t *testing.T) {
	for _, tc := range []struct{ input, web string }{
		{"<!-- keep -->\n<html><title> Title </title><pre>a  b\n  c</pre></html>\n", "HTML Content: [Title] (68 chars total)"},
		{"<html><title>T</title><script>\n{\"a\":1}\n</script></html>", "HTML Content: [T] (55 chars total)"},
		{strings.Repeat("A", 1000), strings.Repeat("A", 1000)},
		{strings.Repeat("A", 1001), strings.Repeat("A", 500) + "\n... (Total: 1001 chars)"},
	} {
		if got := (&WebParser{}).Parse(tc.input); got != tc.web {
			t.Errorf("labelled Web summary changed: got %q, want %q", got, tc.web)
		}
		if got := (&MinifyParser{}).Parse(tc.input); got != tc.input {
			t.Errorf("Minify changed unsupported content: %q", got)
		}
	}
}
