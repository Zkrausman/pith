package parser

import (
	"strings"
	"testing"
)

const compositeCommit = "commit 0123456789abcdef0123456789abcdef01234567\nAuthor: Example Author <author@example.invalid>\nDate:   Sun Mar 15 22:49:38 2026 -0400\n\n    A subject\n"

func TestCompositeGitIncompleteRecordsPreserved(t *testing.T) {
	cases := map[string]string{
		"short-date-three":      strings.Replace(compositeCommit, "Sun Mar 15 22:49:38 2026 -0400", "Sun Mar 15", 1),
		"short-date-four":       strings.Replace(compositeCommit, "Sun Mar 15 22:49:38 2026 -0400", "Sun Mar 15 22:49:38", 1),
		"invalid-date":          strings.Replace(compositeCommit, "Mar 15", "Mar 99", 1),
		"missing-author":        strings.Replace(compositeCommit, "Author: Example Author <author@example.invalid>\n", "", 1),
		"missing-date":          strings.Replace(compositeCommit, "Date:   Sun Mar 15 22:49:38 2026 -0400\n", "", 1),
		"missing-subject":       strings.Replace(compositeCommit, "    A subject\n", "", 1),
		"empty-subject":         strings.Replace(compositeCommit, "A subject", "", 1),
		"short-hash":            strings.Replace(compositeCommit, "0123456789abcdef0123456789abcdef01234567", "abc1234", 1),
		"decorated":             strings.Replace(compositeCommit, "01234567\n", "01234567 (HEAD -> main)\n", 1),
		"merge":                 strings.Replace(compositeCommit, "Author:", "Merge: abc1234 def5678\nAuthor:", 1),
		"multiline":             compositeCommit + "    body retained 雪\n",
		"multiline-separated":   compositeCommit + "\n    body retained\n",
		"signature":             compositeCommit + "gpg: signature details\n",
		"extra-metadata":        compositeCommit + "Author: second identity\n",
		"crlf":                  strings.ReplaceAll(compositeCommit, "\n", "\r\n"),
		"later-incomplete":      compositeCommit + "\ncommit abc1234\n",
		"prefix-and-incomplete": "On branch main\n\x1b[31mretained prefix\x1b[0m\n\ncommit abc1234\n\n",
		"trailing-unrecognized": compositeCommit + "\ncapture stopped\t\n\n",
	}
	for n := 1; n < 5; n++ {
		cases["prefix-"+strings.Repeat("x", n)] = strings.Join(strings.Split(compositeCommit, "\n")[:n], "\n")
	}
	for _, p := range []Parser{&CompositeGitParser{}, &GitShowParser{}} {
		for name, input := range cases {
			t.Run(p.Name()+"/"+name, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("parser panicked: %v", r)
					}
				}()
				if got := p.Parse(input); got != input {
					t.Fatalf("capture changed: got %q, want %q", got, input)
				}
			})
		}
	}
}

func TestGitShowNonCommitPreserved(t *testing.T) {
	p := fallbackParserFor(t, "git show HEAD:fixture", "git_show")
	for _, input := range []string{"", " \t\r\n", "On branch main\n", "index retained\n", "@@ literal\n", "\x1b[31mblob text\x1b[0m\n", "{\n  \"commit\":", "tag fixture\nTagger: Example\n\n" + compositeCommit} {
		if got := p.Parse(input); got != input {
			t.Errorf("non-commit capture changed: got %q, want %q", got, input)
		}
	}
}

func TestCompositeGitSupportedRecords(t *testing.T) {
	summary := "0123456 | Example Author | Mar 15 2026 | A subject"
	diff := "diff --git a/x b/x\nindex abc..def 100644\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n"
	wantDiff := "--- x\n+++ x\n@@\n-old\n+new\n"
	for _, p := range []Parser{&CompositeGitParser{}, &GitShowParser{}} {
		for _, tc := range []struct{ input, want string }{
			{compositeCommit, summary},
			{strings.TrimSuffix(compositeCommit, "\n"), summary},
			{compositeCommit + "\n" + compositeCommit, summary + "\n" + summary},
			{compositeCommit + "\n" + diff, summary + "\n" + wantDiff},
			{strings.Replace(compositeCommit, "0123456789abcdef0123456789abcdef01234567", strings.Repeat("a", 64), 1), "aaaaaaa | Example Author | Mar 15 2026 | A subject"},
		} {
			if got := p.Parse(tc.input); got != tc.want {
				t.Errorf("%s supported output = %q, want %q", p.Name(), got, tc.want)
			}
		}
	}
	p := &CompositeGitParser{}
	if got := p.Parse("On branch main\nmodified:   x\n\n" + compositeCommit); got != "modified:   x\n\n"+summary {
		t.Errorf("status/log control changed: %q", got)
	}
}

func TestCompositeGitRegistrySelection(t *testing.T) {
	for _, command := range []string{"git status; git show", "git status & git log"} {
		found := false
		for _, p := range GetAllParsers() {
			if !p.CanParse(command, nil) {
				continue
			}
			if p.Name() != "git_composite" {
				t.Fatalf("%q selected %q", command, p.Name())
			}
			input := "On branch main\ncommit abc1234\n"
			if got := p.Parse(input); got != input {
				t.Errorf("registry fallback changed %q", got)
			}
			found = true
			break
		}
		if !found {
			t.Fatalf("no parser selected for %q", command)
		}
	}
}
