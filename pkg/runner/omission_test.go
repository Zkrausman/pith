package runner

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"pith/pkg/config"
)

func TestMiddleOutOmissionAccounting(t *testing.T) {
	beforeHot := func(n int) string {
		return fmt.Sprintf("\n... [%d lines of non-critical output removed by Pith] ...\n", n)
	}
	beforeTail := func(n int) string {
		return fmt.Sprintf("\n... [%d lines removed by Pith middle-out truncation] ...\n", n)
	}
	for _, tc := range []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "documented-single-line-gap",
			input: []string{"head1", "head2", "hidden3", "context4", "ERROR failure5", "context6", "tail7", "tail8"},
			want:  []string{"head1", "head2", beforeHot(1), "context4", "ERROR failure5", "context6", "tail7", "tail8"},
		},
		{
			name:  "ordinary-middle",
			input: []string{"head1", "head2", "hidden3", "hidden4", "hidden5", "hidden6", "tail7", "tail8"},
			want:  []string{"head1", "head2", beforeTail(4), "tail7", "tail8"},
		},
		{
			name:  "first-middle-line-hot",
			input: []string{"head1", "head2", "ERROR 3", "context4", "hidden5", "hidden6", "tail7", "tail8"},
			want:  []string{"head1", "head2", "ERROR 3", "context4", beforeTail(2), "tail7", "tail8"},
		},
		{
			name:  "last-middle-line-hot",
			input: []string{"head1", "head2", "hidden3", "hidden4", "context5", "ERROR 6", "tail7", "tail8"},
			want:  []string{"head1", "head2", beforeHot(2), "context5", "ERROR 6", "tail7", "tail8"},
		},
		{
			name:  "single-line-gap-before-tail",
			input: []string{"head1", "head2", "context3", "ERROR 4", "context5", "hidden6", "tail7", "tail8"},
			want:  []string{"head1", "head2", "context3", "ERROR 4", "context5", beforeTail(1), "tail7", "tail8"},
		},
		{
			name:  "single-line-gap-between-windows",
			input: []string{"head1", "head2", "context3", "ERROR 4", "context5", "hidden6", "context7", "FAIL 8", "context9", "tail10", "tail11"},
			want:  []string{"head1", "head2", "context3", "ERROR 4", "context5", beforeHot(1), "context7", "FAIL 8", "context9", "tail10", "tail11"},
		},
		{
			name:  "multiple-omissions",
			input: []string{"head1", "head2", "hidden3", "context4", "ERROR 5", "context6", "hidden7", "hidden8", "context9", "FAIL 10", "context11", "hidden12", "tail13", "tail14"},
			want:  []string{"head1", "head2", beforeHot(1), "context4", "ERROR 5", "context6", beforeHot(2), "context9", "FAIL 10", "context11", beforeTail(1), "tail13", "tail14"},
		},
		{
			name:  "omitted-and-retained-blank-lines",
			input: []string{"head1", "head2", "", "context4", "ERROR 5", "context6", "", ""},
			want:  []string{"head1", "head2", beforeHot(1), "context4", "ERROR 5", "context6", "", ""},
		},
		{
			name:  "retained-bytes",
			input: []string{" head1\r", "head2\t", "hidden3", "\tcontext4\r", "ERROR 雪 🙂\r", "context6\t", "tail7\r", "tail8 "},
			want:  []string{" head1\r", "head2\t", beforeHot(1), "\tcontext4\r", "ERROR 雪 🙂\r", "context6\t", "tail7\r", "tail8 "},
		},
	} {
		for _, terminated := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/terminated=%t", tc.name, terminated), func(t *testing.T) {
				input, want := strings.Join(tc.input, "\n"), strings.Join(tc.want, "\n")
				if terminated {
					input += "\n"
					want += "\n"
				}
				r := &Runner{cfg: &config.Config{MaxLines: 5, HeadLines: 2, TailLines: 2}}
				if got := r.ApplyMiddleOutTruncation(input); got != want {
					t.Fatalf("truncated output = %q, want %q", got, want)
				}
			})
		}
	}
	for _, input := range []string{"", "\n", "1\n2\n3\n4\n5", "1\n2\n3\n4\n5\n", "1\n2\n3\n4\n\n"} {
		r := &Runner{cfg: &config.Config{MaxLines: 5, HeadLines: 2, TailLines: 2}}
		if got := r.ApplyMiddleOutTruncation(input); got != input {
			t.Errorf("at/below threshold: output = %q, want %q", got, input)
		}
	}
}

func TestMiddleOutOmissionCoverage(t *testing.T) {
	// Exhaust all hot-line placements, including adjacent/overlapping windows,
	// head/tail overlap, and zero-sized head/tail regions. Unique source lines
	// let us verify order and exactly one occurrence independently of rendering.
	const n = 8
	marker := regexp.MustCompile(`^\.\.\. \[([0-9]+) lines (?:of non-critical output removed by Pith|removed by Pith middle-out truncation)\] \.\.\.$`)
	for _, bounds := range [][2]int{{0, 0}, {0, 2}, {2, 0}, {1, 1}, {2, 2}, {3, 4}, {4, 4}, {5, 5}} {
		for mask := 0; mask < 1<<n; mask++ {
			lines := make([]string, n)
			keep := make([]bool, n)
			head, tail := bounds[0], bounds[1]
			for i := range lines {
				lines[i] = fmt.Sprintf("line %02d", i)
				if mask&(1<<i) != 0 {
					lines[i] += " ERROR"
				}
				keep[i] = i < head || i >= n-tail
				for j := head; j < n-tail; j++ {
					if mask&(1<<j) != 0 && i >= j-1 && i <= j+1 {
						keep[i] = true
					}
				}
			}
			for _, suffix := range []string{"", "\n"} {
				r := &Runner{cfg: &config.Config{MaxLines: 5, HeadLines: head, TailLines: tail}}
				got := r.ApplyMiddleOutTruncation(strings.Join(lines, "\n") + suffix)
				next := 0
				for _, line := range strings.Split(got, "\n") {
					if line == "" { // Marker padding, absent from this fixture's source.
						continue
					}
					if match := marker.FindStringSubmatch(line); match != nil {
						count, _ := strconv.Atoi(match[1])
						start := next
						for next < n && !keep[next] {
							next++
						}
						if count == 0 || count != next-start {
							t.Fatalf("bounds=%v mask=%d suffix=%q: marker count=%d, missing=%d in %q", bounds, mask, suffix, count, next-start, got)
						}
					} else {
						if next >= n || !keep[next] || line != lines[next] {
							t.Fatalf("bounds=%v mask=%d suffix=%q: unexpected line %q at source index %d in %q", bounds, mask, suffix, line, next, got)
						}
						next++
					}
				}
				if next != n {
					t.Fatalf("bounds=%v mask=%d suffix=%q: accounted for %d of %d lines in %q", bounds, mask, suffix, next, n, got)
				}
			}
		}
	}
}
