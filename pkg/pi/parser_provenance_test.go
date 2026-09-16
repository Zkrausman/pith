package pi

import "testing"

func TestParserReductionMarkerForms(t *testing.T) {
	for _, marker := range []string{
		"... [1 lines minimized by Pith] ...",
		"... [41 lines minimized by Pith] ...",
		"... [7 bytes minimized by Pith]",
		"... [7 bytes, 1 lines minimized by Pith PiOptimize] ...",
	} {
		t.Run(marker, func(t *testing.T) {
			original := "before\nomitted\nafter"
			parsed := "before\n" + marker + "\nafter"
			lines, bytes, known := parserReduction(original, parsed)
			// Removing only the marker leaves its surrounding newlines intact.
			// This is net representation reduction, not exact omitted source lines.
			if !known || lines != 0 || bytes != len("omitted") {
				t.Fatalf("got (%d, %d, %v), want (0, 7, true)", lines, bytes, known)
			}
		})
	}
	if _, _, known := parserReduction("source", "summary without a marker"); known {
		t.Fatal("unmarked parser prose must not claim known marker-excluded reduction")
	}
}
