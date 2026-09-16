package parser

import (
	"strings"
	"testing"
)

const defaultGitLog = "commit 0123456789abcdef0123456789abcdef01234567\nAuthor: Example Author <author@example.invalid>\nDate:   Sun Mar 15 22:49:38 2026 -0400\n\n    A subject\n"

func TestGitLogParserCompleteFormat(t *testing.T) {
	want := "0123456 | Example Author | Mar 15 2026 | A subject"
	for _, input := range []string{defaultGitLog, strings.TrimSuffix(defaultGitLog, "\n"), strings.Replace(defaultGitLog, "0123456789abcdef0123456789abcdef01234567", strings.Repeat("a", 64), 1)} {
		expected := want
		if strings.Contains(input, strings.Repeat("a", 64)) {
			expected = "aaaaaaa" + want[7:]
		}
		if got := (&GitLogParser{}).Parse(input); got != expected {
			t.Fatalf("got %q, want %q", got, expected)
		}
	}
	if got := (&GitLogParser{}).Parse(defaultGitLog + "\n" + defaultGitLog); got != want+"\n"+want {
		t.Fatalf("complete multiple records: %q", got)
	}
}

func TestGitLogParserUnsupportedPreserved(t *testing.T) {
	for name, input := range map[string]string{
		"empty":                     "",
		"oneline":                   "abc1234 subject\n123abcd another\n",
		"custom":                    "0123456|Example Author|A subject\n",
		"graph":                     "* abc1234 subject\n| * 123abcd another\n",
		"prefix":                    "unrecognized prefix\n" + defaultGitLog,
		"suffix":                    defaultGitLog + "unrecognized suffix\n",
		"indented-suffix":           defaultGitLog + "    not safely attributable\n",
		"body":                      defaultGitLog + "    \n    Additional message\n",
		"stat":                      defaultGitLog + "\n file.go | 1 +\n 1 file changed, 1 insertion(+)\n",
		"patch":                     defaultGitLog + "\ndiff --git a/x b/x\n@@ -1 +1 @@\n-a\n+b\n",
		"merge":                     strings.Replace(defaultGitLog, "Author:", "Merge: abc1234 def5678\nAuthor:", 1),
		"decoration":                strings.Replace(defaultGitLog, "01234567\n", "01234567 (HEAD -> main)\n", 1),
		"short-hash":                strings.Replace(defaultGitLog, "0123456789abcdef0123456789abcdef01234567", "abc1234", 1),
		"missing-author":            strings.Replace(defaultGitLog, "Author: Example Author <author@example.invalid>\n", "", 1),
		"missing-subject":           strings.Replace(defaultGitLog, "    A subject\n", "", 1),
		"short-date-three-fields":   strings.Replace(defaultGitLog, "Sun Mar 15 22:49:38 2026 -0400", "Sun Mar 15", 1),
		"short-date-four-fields":    strings.Replace(defaultGitLog, "Sun Mar 15 22:49:38 2026 -0400", "Sun Mar 15 22:49:38", 1),
		"custom-date":               strings.Replace(defaultGitLog, "Sun Mar 15 22:49:38 2026 -0400", "2026-03-15", 1),
		"crlf":                      strings.ReplaceAll(defaultGitLog, "\n", "\r\n"),
		"extra-newline":             defaultGitLog + "\n",
		"second-record-unsupported": defaultGitLog + "\nabc1234 subject\n",
		"second-record-incomplete":  defaultGitLog + "\ncommit 0123456789abcdef0123456789abcdef01234567\n",
		"no-record-separator":       defaultGitLog + defaultGitLog,
	} {
		t.Run(name, func(t *testing.T) {
			if got := (&GitLogParser{}).Parse(input); got != input {
				t.Fatalf("unsupported output changed: got %q, want %q", got, input)
			}
		})
	}
}
