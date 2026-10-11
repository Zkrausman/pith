package parser

import (
	"strings"
	"testing"
)

func TestParserFidelityDefects(t *testing.T) {
	t.Run("git status keeps rename paths next to other changes", func(t *testing.T) {
		p := &GitStatusParser{}
		input := "On branch main\nChanges not staged for commit:\n\tmodified: keep.go\n\trenamed: old name.go -> new name.go\n"
		got := p.Parse(input)
		if !strings.Contains(got, "\trenamed: old name.go -> new name.go") {
			t.Fatalf("rename path was lost from mixed status output: %q", got)
		}
		if !strings.Contains(got, "modified: keep.go") {
			t.Fatalf("ordinary status row was lost: %q", got)
		}
	})

	t.Run("large git status preserves middle rename rows", func(t *testing.T) {
		p := &GitStatusParser{}
		rows := []string{"On branch main", "Changes not staged for commit:"}
		for i := 0; i < 21; i++ {
			if i == 10 {
				rows = append(rows, "\trenamed: old-middle.go -> new-middle.go")
			} else {
				rows = append(rows, "\tmodified: file-"+string(rune('a'+i))+".go")
			}
		}
		input := strings.Join(rows, "\n") + "\n"
		if got := p.Parse(input); got != input {
			t.Fatalf("large status must preserve every row byte-for-byte: got %q, want %q", got, input)
		}
	})

	t.Run("git status keeps indented untracked filename that resembles a header", func(t *testing.T) {
		p := &GitStatusParser{}
		input := "On branch main\nChanges not staged for commit:\n\tmodified: keep.go\nUntracked files:\n\tTo report.txt\n\t(use guide.txt\n"
		want := "modified: keep.go\nTo report.txt\n(use guide.txt"
		if got := p.Parse(input); got != want {
			t.Fatalf("untracked path must survive structural header recognition: got %q, want %q", got, want)
		}
	})

	t.Run("ls preserves long-looking text without invocation profile", func(t *testing.T) {
		p := &LsParser{}
		input := "total 4\n-rw-r--r-- 1 user group 42 Oct 11 10:00 release notes final.txt\n-rw-r--r-- 1 user group 5 Oct 11 10:01 another file.txt\n"
		if got := p.Parse(input); got != input {
			t.Fatalf("without invocation profile, long-looking text must remain intact: got %q, want %q", got, input)
		}
	})

	t.Run("ls preserves a fake long-row filename", func(t *testing.T) {
		p := &LsParser{}
		input := "-rw-r--r-- 1 user group 42 Oct 11 10:00 file.txt\n"
		if got := p.Parse(input); got != input {
			t.Fatalf("row-shaped filename must remain byte-for-byte intact: got %q, want %q", got, input)
		}
	})

	t.Run("ls preserves valid-row-shaped continuation bytes", func(t *testing.T) {
		p := &LsParser{}
		row := "-rw-r--r-- 1 user group 42 Oct 11 10:00 file.txt\n"
		input := row + row
		if got := p.Parse(input); got != input {
			t.Fatalf("row-shaped continuation must remain byte-for-byte intact: got %q, want %q", got, input)
		}
	})

	t.Run("ls preserves ambiguous native listing", func(t *testing.T) {
		p := &LsParser{}
		input := "file one    file two\n"
		if got := p.Parse(input); got != input {
			t.Fatalf("ambiguous listing should stay raw: got %q, want %q", got, input)
		}
	})

	t.Run("ls preserves invalid or ambiguous rows exactly", func(t *testing.T) {
		p := &LsParser{}
		valid := "-rw-r--r-- 1 user group 42 Oct 11 10:00 file.txt"
		for _, tc := range []struct {
			name  string
			input string
		}{
			{name: "carriage return in filename", input: "-rw-r--r-- 1 user group 42 Oct 11 10:00 file\rname.txt\n"},
			{name: "blank continuation", input: valid + "\n\n" + valid + "\n"},
			{name: "header-looking continuation", input: valid + "\nMode hidden\n"},
			{name: "malformed size", input: "-rw-r--r-- 1 user group 1.. Oct 11 10:00 file.txt\n"},
			{name: "invalid time", input: "-rw-r--r-- 1 user group 42 Oct 11 99:99 file.txt\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := p.Parse(tc.input); got != tc.input {
					t.Fatalf("ambiguous listing must remain byte-for-byte intact: got %q, want %q", got, tc.input)
				}
			})
		}
	})

	t.Run("source strings and block markers do not suppress code", func(t *testing.T) {
		p := &SourceParser{}
		input := "const url = \"https://example.invalid/#fragment\"; // retained\nconst marker = \"/*\";\nconst after = 1;\n//go:generate tool\n"
		if got := p.Parse(input); got != input {
			t.Fatalf("ambiguous source should stay byte-for-byte intact: got %q, want %q", got, input)
		}
	})

	t.Run("get-content requires an actual command identity", func(t *testing.T) {
		p := &GetContentParser{}
		for _, tc := range []struct {
			name string
			cmd  string
			args []string
			want bool
		}{
			{name: "powershell cmdlet", cmd: "powershell", args: []string{"-Command", "Get-Content file.json"}, want: true},
			{name: "pwsh cmdlet", cmd: "pwsh", args: []string{"-c", "Get-Content file.json"}, want: true},
			{name: "direct cmdlet", cmd: "Get-Content", args: []string{"file.json"}, want: true},
			{name: "direct cat alias", cmd: "cat", args: []string{"file.json"}, want: true},
			{name: "direct type alias", cmd: "type", args: []string{"file.json"}, want: true},
			{name: "cmd builtin type", cmd: "cmd.exe", args: []string{"/c", "type", "file.json"}, want: true},
			{name: "quoted cmd builtin type", cmd: "cmd.exe", args: []string{"/c", `"type file.json"`}, want: true},
			{name: "inline switch keeps following arguments", cmd: "powershell", args: []string{"-Command:Get-Content", "file.json"}, want: true},
			{name: "cmdlet substring in catalog", cmd: "powershell", args: []string{"-Command", "Get-Catalog file.json"}, want: false},
			{name: "argument contains cat", cmd: "echo", args: []string{"Catalog"}, want: false},
			{name: "text mentions cmdlet", cmd: "powershell", args: []string{"-Command", "Write-Output Get-Content"}, want: false},
			{name: "unrelated command with type argument", cmd: "powershell", args: []string{"-Command", "Write-Output stereotype"}, want: false},
			{name: "compound command", cmd: "powershell", args: []string{"-Command", "Get-Content file.json; Remove-Item file.json"}, want: false},
			{name: "subexpression", cmd: "powershell", args: []string{"-Command", "Get-Content $(Get-Location)"}, want: false},
			{name: "parenthesized command", cmd: "powershell", args: []string{"-Command", "Get-Content (Get-Location)"}, want: false},
			{name: "redirection", cmd: "powershell", args: []string{"-Command", "Get-Content file.json > out.json"}, want: false},
			{name: "inline switch sees later compound arguments", cmd: "powershell", args: []string{"-Command:Get-Content", "file.json; Remove-Item file.json"}, want: false},
			{name: "cmd is not a PowerShell alias profile", cmd: "cmd", args: []string{"/c", "cat file.json"}, want: false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := p.CanParse(tc.cmd, tc.args); got != tc.want {
					t.Fatalf("CanParse(%q, %q) = %v, want %v", tc.cmd, tc.args, got, tc.want)
				}
			})
		}

		t.Run("registry dispatch respects PowerShell parser precedence", func(t *testing.T) {
			for _, tc := range []struct {
				name string
				cmd  string
				args []string
				want string
			}{
				{name: "direct cmdlet", cmd: "Get-Content", args: []string{"file.json"}, want: "get-content"},
				{name: "wrapped cmdlet", cmd: "powershell", args: []string{"-Command", "Get-Content file.json"}, want: "windows-shell"},
				{name: "wrapped unrelated command", cmd: "pwsh", args: []string{"-Command", "Get-Catalog file.json"}, want: "windows-shell"},
				{name: "incidental argument substring", cmd: "echo", args: []string{"Catalog"}, want: ""},
			} {
				t.Run(tc.name, func(t *testing.T) {
					got := ""
					for _, candidate := range GetAllParsers() {
						if candidate.CanParse(tc.cmd, tc.args) {
							got = candidate.Name()
							break
						}
					}
					if got != tc.want {
						t.Fatalf("registry selected %q, want %q", got, tc.want)
					}
				})
			}
		})
	})
}
