package parser

import (
	"fmt"
	"strings"
	"testing"
)

func TestDuPathFidelity(t *testing.T) {
	for _, input := range []string{
		"4K\t./My Project\n",
		"4K\t leading\t雪 café \t\n",
		"4K\t./link -> target name\n",
		"4K ./ambiguous path\n",
		"du: cannot access 'missing path': No such file\n",
		"4K\t./My Project\x00",
		"", " \t\n", "4K\t./unterminated",
		strings.Repeat("unrecognized format\n", 22),
		strings.Repeat("4K\t./My Project\n", 21) + "4K\t./NUL\x00record\n",
	} {
		t.Run(fmt.Sprintf("%q", input), func(t *testing.T) {
			if got := (&DuParser{}).Parse(input); got != input {
				t.Fatalf("Parse = %q, want exact %q", got, input)
			}
		})
	}
}

func TestDuPathLimit(t *testing.T) {
	var rows []string
	for i := 0; i < 22; i++ {
		rows = append(rows, fmt.Sprintf("4K\t./Project %d\t雪 ", i))
	}
	want := strings.Join(rows[:20], "\n") + "\n... (+ 2 more)"
	if got := (&DuParser{}).Parse(strings.Join(rows, "\n") + "\n"); got != want {
		t.Fatalf("Parse = %q, want %q", got, want)
	}
}
