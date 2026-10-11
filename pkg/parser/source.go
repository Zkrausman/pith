package parser

type SourceParser struct{}

func (s *SourceParser) Name() string {
	return "source"
}

func (s *SourceParser) CanParse(cmd string, args []string) bool {
	return cmd == "cat" || cmd == "type"
}

func (s *SourceParser) Parse(output string) string {
	// This parser receives content without the source language or filename, so
	// it cannot distinguish comments from string literals, Python // operators,
	// preprocessor directives, or language-specific block syntax. Preserve the
	// capture byte-for-byte until dispatch can supply enough context for a
	// sound lexer; heuristic comment removal can change executable code.
	return output
}
