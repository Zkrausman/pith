package parser

import (
	"strings"
	"testing"
)

func TestEnvParser(t *testing.T) {
	p := &EnvParser{}

	// Test CanParse
	if !p.CanParse("env", []string{}) {
		t.Error("EnvParser should handle env command")
	}
	if !p.CanParse("set", []string{}) {
		t.Error("EnvParser should handle set command")
	}

	input := `PATH=/usr/bin
USER=zkrau
GITHUB_TOKEN=secret_token
PASSWORD=12345
SHELL=/bin/bash
`
	output := p.Parse(input)
	if strings.Contains(output, "GITHUB_TOKEN") || strings.Contains(output, "PASSWORD") {
		t.Error("EnvParser should redact secrets")
	}
	if !strings.Contains(output, "USER=zkrau") {
		t.Error("EnvParser should keep normal variables")
	}
}

func TestDockerPsParser(t *testing.T) {
	p := &DockerPsParser{}

	// Test CanParse
	if !p.CanParse("docker", []string{"ps"}) {
		t.Error("DockerPsParser should handle docker ps")
	}
	if !p.CanParse("docker", []string{"images"}) {
		t.Error("DockerPsParser should handle docker images")
	}

	input := `CONTAINER ID   IMAGE     COMMAND   CREATED   STATUS    PORTS     NAMES
1234567890ab   nginx     "entry"   1h ago    Up        80/tcp    my-nginx
`
	output := p.Parse(input)
	if !strings.Contains(output, "1234567890ab") || !strings.Contains(output, "my-nginx") {
		t.Errorf("DockerPsParser missing essential info: %s", output)
	}
}

func TestDependencyParser(t *testing.T) {
	p := &DependencyParser{}

	// Test CanParse
	if !p.CanParse("npm", []string{"list"}) {
		t.Error("DependencyParser should handle npm list")
	}
	if !p.CanParse("pip", []string{"list"}) {
		t.Error("DependencyParser should handle pip list")
	}

	input := `pith@0.3.0 E:\Repos\Pith
├── github.com/spf13/cobra@v1.8.0
└── github.com/AlecAivazis/survey/v2@v2.3.7
`
	output := p.Parse(input)
	if strings.Contains(output, "├──") {
		t.Error("DependencyParser should strip visual tree characters")
	}
	if !strings.Contains(output, "github.com/spf13/cobra") {
		t.Error("DependencyParser missing package name")
	}
}

func TestTestParser(t *testing.T) {
	p := &TestParser{}

	// Test CanParse
	if !p.CanParse("npm", []string{"test"}) {
		t.Error("TestParser should handle npm test")
	}
	if !p.CanParse("go", []string{"test"}) {
		t.Error("TestParser should handle go test")
	}
	if !p.CanParse("pytest", []string{}) {
		t.Error("TestParser should handle pytest")
	}

	input := `Running tests...
PASS: TestOne
FAIL: TestTwo
    error: expected true, got false
DONE: 1 passed, 1 failed
`
	output := p.Parse(input)
	if !strings.Contains(output, "FAIL: TestTwo") || !strings.Contains(output, "1 passed") {
		t.Errorf("TestParser missing summary or failure: %s", output)
	}
}

func TestGitHubParser(t *testing.T) {
	p := &GitHubParser{}
	input := `Zkrausman/Pith         private 2026-03-16T06:01:28Z
Zkrausman/resume       public  2026-01-01T12:00:00Z
`
	// Test CanParse
	if !p.CanParse("gh", []string{"repo", "list"}) {
		t.Error("GitHubParser should handle gh repo list")
	}
	if !p.CanParse("gh", []string{"issue", "list"}) {
		t.Error("GitHubParser should handle gh issue list")
	}

	output := p.Parse(input)
	if !strings.Contains(output, "Zkrausman/Pith         private 2026-03-16T06:01:28Z") {
		t.Errorf("GitHubParser failed to pass through repo list, got:\n%s", output)
	}
}

func TestGoToolCoverParser(t *testing.T) {
	p := &GoToolCoverParser{}

	// Test CanParse
	if !p.CanParse("go", []string{"tool", "cover", "-func", "profile.cov"}) {
		t.Error("GoToolCoverParser should handle go tool cover")
	}

	input := `pith/pkg/parser/fs.go:25:	LsParser.Name		100.0%
pith/pkg/parser/fs.go:30:	LsParser.CanParse	50.0%
pith/pkg/parser/infra.go:10:	EnvParser.Parse		100.0%
total:				(statements)		95.1%
`
	output := p.Parse(input)
	if strings.Contains(output, "LsParser.Name") {
		t.Error("GoToolCoverParser should strip 100.0% coverage lines")
	}
	if !strings.Contains(output, "LsParser.CanParse	50.0%") {
		t.Error("GoToolCoverParser should keep non-100% lines")
	}
	if !strings.Contains(output, "total:				(statements)		95.1%") {
		t.Error("GoToolCoverParser should keep total line")
	}
}
