package telemetry

import (
	"database/sql"
	"fmt"
	"time"
)

func (t *Telemetry) requirePrivateCodexStore() error {
	if t == nil || t.DB == nil || !t.private {
		return fmt.Errorf("Codex observations require private telemetry storage")
	}
	return nil
}

// RecordCodexProposedFamily records one fixed-label Bash tool proposal. It
// stores no command text, arguments, paths, or tool-call identifiers.
func (t *Telemetry) RecordCodexProposedFamily(family string) error {
	return t.RecordCodexProposal(family, "")
}

// RecordCodexProposal records a fixed eligibility reason; empty means a legacy observation.
func (t *Telemetry) RecordCodexProposal(family, bypassReason string) error {
	if err := t.requirePrivateCodexStore(); err != nil {
		return err
	}
	if !IsCodexObservationFamilyAllowed(family) {
		return fmt.Errorf("Codex observation family is not allowlisted")
	}
	if !IsCodexBypassReasonAllowed(bypassReason) {
		return fmt.Errorf("Codex bypass reason is not allowlisted")
	}
	return t.recordCodexObservation("proposal", family, bypassReason, nil)
}

// IsCodexObservationFamilyAllowed reports whether family is one of the fixed
// labels accepted by the private observation recorder.
func IsCodexObservationFamilyAllowed(family string) bool {
	_, ok := codexObservationFamilies[family]
	return ok
}

// IsCodexBypassReasonAllowed accepts only fixed labels, or an absent legacy reason.
func IsCodexBypassReasonAllowed(reason string) bool {
	switch reason {
	case "", "invalid_literal", "shell_syntax", "shell_reserved_or_assignment", "interpreter", "recursive_wrapper", "shell_builtin", "shell_environment", "execution_cwd_unavailable", "executable_unresolved_or_format", "wrapper_encoding_failed":
		return true
	default:
		return false
	}
}

// RecordCodexCapture records bounded capture outcome metadata after stdout has
// already been published. Nil values stay SQL NULL; zero remains measurable.
func (t *Telemetry) RecordCodexCapture(observation CodexCaptureObservation) error {
	if err := t.requirePrivateCodexStore(); err != nil {
		return err
	}
	if err := ValidateCodexCaptureObservation(observation); err != nil {
		return err
	}
	return t.recordCodexObservation("capture", "", "", &observation)
}

// ValidateCodexCaptureObservation checks the closed outcomes and bounded,
// nullable measurements before private storage is opened or changed.
func ValidateCodexCaptureObservation(observation CodexCaptureObservation) error {
	switch observation.Outcome {
	case CodexCaptureComplete, CodexCaptureDeadline, CodexCaptureCap, CodexCaptureIncomplete:
	default:
		return fmt.Errorf("Codex capture outcome is invalid")
	}
	switch observation.Selection {
	case CodexSelectionReduced, CodexSelectionRaw, CodexSelectionUnknown:
	default:
		return fmt.Errorf("Codex capture selection is invalid")
	}
	if observation.ElapsedMs != nil && (*observation.ElapsedMs < 0 || *observation.ElapsedMs > CodexObservationMaxElapsedMs) {
		return fmt.Errorf("Codex capture elapsed time is out of range")
	}
	if (observation.OriginalBytes == nil) != (observation.DeliveredBytes == nil) {
		return fmt.Errorf("Codex capture byte counts must both be known or both be unknown")
	}
	if observation.OriginalBytes != nil {
		if *observation.OriginalBytes < 0 || *observation.DeliveredBytes < 0 ||
			*observation.OriginalBytes > CodexObservationMaxBytes || *observation.DeliveredBytes > CodexObservationMaxBytes {
			return fmt.Errorf("Codex capture byte count is out of range")
		}
	}
	if observation.Outcome == CodexCaptureIncomplete {
		if observation.Selection != CodexSelectionUnknown || observation.OriginalBytes != nil {
			return fmt.Errorf("incomplete Codex captures must have unknown selection and byte counts")
		}
		return nil
	}
	if observation.Selection == CodexSelectionUnknown {
		return fmt.Errorf("complete Codex captures must have a known selection")
	}
	if observation.Outcome != CodexCaptureComplete && observation.Selection != CodexSelectionRaw {
		return fmt.Errorf("streamed Codex captures must use raw selection")
	}
	if observation.OriginalBytes != nil {
		if observation.Selection == CodexSelectionRaw && *observation.OriginalBytes != *observation.DeliveredBytes {
			return fmt.Errorf("raw Codex capture byte counts must match")
		}
		if observation.Selection == CodexSelectionReduced && *observation.DeliveredBytes >= *observation.OriginalBytes {
			return fmt.Errorf("reduced Codex capture must be smaller than its original")
		}
	}
	return nil
}

func (t *Telemetry) recordCodexObservation(kind, family, bypassReason string, capture *CodexCaptureObservation) error {
	tx, err := t.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var captureOutcome, selection interface{}
	var elapsedMs, originalBytes, deliveredBytes interface{}
	if capture != nil {
		captureOutcome, selection = capture.Outcome, capture.Selection
		if capture.ElapsedMs != nil {
			elapsedMs = *capture.ElapsedMs
		}
		if capture.OriginalBytes != nil {
			originalBytes, deliveredBytes = *capture.OriginalBytes, *capture.DeliveredBytes
		}
	}
	var familyValue interface{}
	if family != "" {
		familyValue = family
	}
	var reasonValue interface{}
	if bypassReason != "" {
		reasonValue = bypassReason
	}
	if _, err := tx.Exec(`INSERT INTO codex_wrapper_observations
		(kind, family, capture_outcome, elapsed_ms, selection, original_bytes, delivered_bytes, bypass_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, kind, familyValue, captureOutcome, elapsedMs, selection, originalBytes, deliveredBytes, reasonValue); err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-time.Duration(CodexObservationRetentionDays) * 24 * time.Hour)
	if _, err := tx.Exec("DELETE FROM codex_wrapper_observations WHERE timestamp < ?", cutoff); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM codex_wrapper_observations
		WHERE id NOT IN (SELECT id FROM codex_wrapper_observations ORDER BY id DESC LIMIT ?)`, CodexObservationMaxRows); err != nil {
		return err
	}
	return tx.Commit()
}

// GetCodexObservationSummary returns only recent aggregate counts and numeric
// reductions. The table's data never participates in gain accounting.
func (t *Telemetry) GetCodexObservationSummary() (CodexObservationSummary, error) {
	summary := CodexObservationSummary{
		ProposalCounts:  make(map[string]int),
		BypassCounts:    make(map[string]int),
		CaptureOutcomes: make(map[string]int),
		SelectionCounts: make(map[string]int),
	}
	rows, err := t.DB.Query(`SELECT kind, COALESCE(family, ''), COALESCE(capture_outcome, ''), elapsed_ms,
		COALESCE(selection, ''), original_bytes, delivered_bytes, COALESCE(bypass_reason, 'unknown')
		FROM codex_wrapper_observations
		WHERE timestamp >= datetime('now', ?)`, fmt.Sprintf("-%d days", CodexObservationRetentionDays))
	if err != nil {
		return summary, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, family, outcome, selection, bypassReason string
		var elapsed, original, delivered sql.NullInt64
		if err := rows.Scan(&kind, &family, &outcome, &elapsed, &selection, &original, &delivered, &bypassReason); err != nil {
			return summary, err
		}
		if kind == "proposal" {
			summary.ProposalCounts[family]++
			summary.BypassCounts[bypassReason]++
			continue
		}
		summary.CaptureRecords++
		summary.CaptureOutcomes[outcome]++
		summary.SelectionCounts[selection]++
		if elapsed.Valid {
			summary.ElapsedKnown++
		} else {
			summary.ElapsedUnknown++
		}
		if original.Valid && delivered.Valid {
			summary.ByteCountsKnown++
			if selection == CodexSelectionReduced && elapsed.Valid && elapsed.Int64 > 2000 {
				summary.SlowReductionCount++
				summary.SlowReductionBytes += original.Int64 - delivered.Int64
			}
		} else {
			summary.ByteCountsUnknown++
		}
	}
	return summary, rows.Err()
}
