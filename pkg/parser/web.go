package parser

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type WebParser struct{}

func (w *WebParser) Name() string { return "web-content" }

func (w *WebParser) CanParse(cmd string, args []string) bool {
	// curl, wget, Invoke-WebRequest, iwr
	return MatchCommand(cmd, "curl") || MatchCommand(cmd, "wget") || MatchCommand(cmd, "Invoke-WebRequest") || MatchCommand(cmd, "iwr")
}

// Parse compacts JSON losslessly. HTML and long plain text retain their
// explicitly labelled, intentionally lossy summaries and established limits.
func (w *WebParser) Parse(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return output
	}

	// JSON compaction is lossless: never decode numbers through float64 or
	// replace complete objects with a key-only summary.
	if compacted, ok := compactJSON(output); ok {
		return compacted
	}
	// Incomplete/unsupported structured captures and binary data are opaque.
	// Keep these out of the deliberately lossy, labelled summaries below.
	if !utf8.ValidString(output) || strings.ContainsRune(output, 0) ||
		strings.HasPrefix(trimmed, "\ufeff") || strings.ContainsAny(trimmed[:1], `{["`) ||
		strings.ContainsAny(trimmed[:1], "-0123456789") ||
		strings.HasPrefix(trimmed, "true") || strings.HasPrefix(trimmed, "false") ||
		strings.HasPrefix(trimmed, "null") {
		return output
	}

	// Try HTML basic extraction. JSON-shaped content before the HTML marker
	// belongs to an opaque capture (for example a response header followed by
	// malformed JSON containing HTML strings), not to an HTML document. JSON
	// after the marker can be an ordinary inline script within HTML.
	htmlStart := strings.Index(trimmed, "<html")
	if doctype := strings.Index(trimmed, "<!DOCTYPE html"); doctype >= 0 && (htmlStart < 0 || doctype < htmlStart) {
		htmlStart = doctype
	}
	if htmlStart >= 0 {
		if hasStructuredLine(trimmed[:htmlStart]) {
			return output
		}
		// Just extract the title if possible
		titleStart := strings.Index(trimmed, "<title>")
		titleEnd := strings.Index(trimmed, "</title>")
		if titleStart != -1 && titleEnd != -1 && titleEnd > titleStart {
			title := trimmed[titleStart+7 : titleEnd]
			return fmt.Sprintf("HTML Content: [%s] (%d chars total)", strings.TrimSpace(title), len(trimmed))
		}
		return fmt.Sprintf("HTML Content (%d chars total)", len(trimmed))
	}

	// A banner or response header may precede an incomplete JSON body. Keep
	// that capture intact; recognized HTML above may legitimately embed JSON.
	if hasStructuredLine(output) {
		return output
	}

	// Default: if it's very long, summarize
	if len(trimmed) > 1000 {
		return fmt.Sprintf("%s\n... (Total: %d chars)", trimmed[:500], len(trimmed))
	}

	return output
}
