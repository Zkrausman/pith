package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"pith/pkg/advisor"
	"pith/pkg/anomaly"
	"pith/pkg/config"
	"pith/pkg/gui"
	"pith/pkg/install"
	"pith/pkg/parser"
	"pith/pkg/pi"
	"pith/pkg/runner"
	"pith/pkg/selfupdate"
	"pith/pkg/telemetry"
)

//go:embed integrations/codex/pre_tool_use_pith.py
var codexPreToolUseAdapter []byte

var installCodexHooksCommand = func(cmd *cobra.Command) error {
	result, err := install.SetupCodexPreToolUseHook(codexPreToolUseAdapter)
	if err != nil {
		for _, backup := range result.Backups {
			cmd.PrintErrf("Backup retained: %s\n", backup)
		}
		return err
	}
	cmd.Printf("Installed Codex PreToolUse/Bash hook in %s.\n", result.SettingsPath)
	cmd.Printf("Adapter: %s\nPith binary: %s\n", result.AdapterPath, result.BinaryPath)
	for _, backup := range result.Backups {
		cmd.Printf("Backup: %s\n", backup)
	}
	cmd.Println("The hook is not trusted automatically. Review it in Codex with /hooks before enabling it.")
	return nil
}

var installPithBinaryCommand = install.Install

const version = "v3.5.2"

const (
	codexWrapperRecordMaxJSONBytes = 2048
	codexWrapperRecordMaxRunes     = 1 << 20
	codexWrapperRecordMaxDuration  = int64(24 * time.Hour / time.Millisecond)
)

type codexWrapperRecordRequest struct {
	Kind           string `json:"kind"`
	Family         string `json:"family"`
	BypassReason   string `json:"bypassReason"`
	Parser         string `json:"parser"`
	CaptureOutcome string `json:"captureOutcome"`
	ElapsedMs      *int64 `json:"elapsedMs"`
	Selection      string `json:"selection"`
	OriginalBytes  *int64 `json:"originalBytes"`
	DeliveredBytes *int64 `json:"deliveredBytes"`
	OriginalRunes  *int   `json:"originalRunes"`
	DeliveredRunes *int   `json:"deliveredRunes"`
	DurationMs     *int64 `json:"durationMs"`
	Outcome        string `json:"outcome"`
}

type HookInput struct {
	ToolResponse struct {
		LlmContent string `json:"llmContent"`
	} `json:"tool_response"`
	ToolCallRequest struct {
		Arguments string `json:"arguments"`
	} `json:"tool_call_request"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

type HookOutput struct {
	Decision      string `json:"decision"`
	Reason        string `json:"reason"`
	SystemMessage string `json:"systemMessage"`
}

type PreToolHookInput struct {
	ToolCall struct {
		Name string                 `json:"name"`
		Args map[string]interface{} `json:"args"`
	} `json:"toolCall"`
	StepIdx        int      `json:"stepIdx"`
	ConversationID string   `json:"conversationId"`
	WorkspacePaths []string `json:"workspacePaths"`
	TranscriptPath string   `json:"transcriptPath"`
	ModelName      string   `json:"modelName"`
}

type PreToolHookOutput struct {
	Decision  string                 `json:"decision"`
	Reason    string                 `json:"reason,omitempty"`
	Overwrite map[string]interface{} `json:"overwrite,omitempty"`
}

type ToolArgs struct {
	Command string `json:"command"`
}

func respondAllow(cmd *cobra.Command) error {
	output := HookOutput{Decision: "allow"}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
}

func NewRootCmd() *cobra.Command {
	var rootCmd = &cobra.Command{
		Use:     "pith [command]",
		Short:   "Pith is a token-optimized CLI proxy",
		Version: version,
		Long:    `Pith intercepts terminal commands, compresses their output, and filters out noise to save tokens for LLMs.`,
		Args:    cobra.ArbitraryArgs,
		RunE:    runRoot,
	}
	rootCmd.PersistentFlags().String("source", "", "Explicit source attribution for telemetry")
	rootCmd.PersistentFlags().String("model", "", "Model name for telemetry attribution")
	rootCmd.PersistentFlags().Float64("model-cost", 0, "Input cost per 1M tokens for the model")
	rootCmd.Flags().Bool("hooks", false, "Install the Codex PreToolUse hook for Bash")

	var configCmd = &cobra.Command{
		Use:   "config",
		Short: "Interactive configuration tool",
		RunE:  runConfig,
	}

	var gainCmd = &cobra.Command{
		Use:     "gain",
		Aliases: []string{"stats"},
		Short:   "Show token savings analytics",
		RunE:    runGain,
	}
	gainCmd.Flags().Bool("by-harness", false, "Group token savings by harness (pi separate from claude/gemini)")

	var discoverCmd = &cobra.Command{
		Use:   "discover",
		Short: "Identify command families that could benefit from dedicated parsers",
		RunE:  runDiscover,
	}
	discoverCmd.Flags().String("details", "", "Show bounded redacted command samples for a command family")
	discoverCmd.Flags().Int("limit", 10, "Maximum command samples to show with --details")

	var piCmd = &cobra.Command{Use: "pi", Short: "Pi integration commands"}
	var piTransformCmd = &cobra.Command{
		Use:   "transform",
		Short: "Transform a completed Pi tool result from JSON stdin",
		Args:  cobra.NoArgs,
		RunE:  runPiTransform,
	}
	piCmd.AddCommand(piTransformCmd)

	codexWrapperRecordCmd := &cobra.Command{
		Use:    "_record-codex-wrapper",
		Short:  "Record bounded Codex wrapper outcome metadata",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE:   runCodexWrapperRecord,
	}

	var hookCmd = &cobra.Command{
		Use:    "_hook",
		Short:  "Internal hook for Gemini CLI",
		Hidden: true,
		RunE:   runHook,
	}

	var hookPreToolCmd = &cobra.Command{
		Use:    "hook-pretool",
		Short:  "Internal PreToolUse hook for Antigravity",
		Hidden: true,
		RunE:   runHookPreTool,
	}
	hookPreToolCmd.Flags().String("source", "antigravity", "The agent triggering the hook")

	var installCmd = &cobra.Command{
		Use:   "install",
		Short: "Install Pith to your system PATH and setup CLI hooks",
		RunE:  runInstall,
	}

	var updateCmd = &cobra.Command{
		Use:   "update",
		Short: "Update Pith to the latest version from GitHub",
		RunE:  runUpdate,
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number of Pith",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("Pith %s\n", version)
		},
	}

	var resetCmd = &cobra.Command{
		Use:   "reset",
		Short: "Reset telemetry data",
		RunE:  runReset,
	}

	var rawCmd = &cobra.Command{
		Use:   "raw [command]",
		Short: "Run a command without parsing or truncation (escape hatch)",
		RunE:  runRaw,
	}

	var dashboardCmd = &cobra.Command{
		Use:   "dashboard",
		Short: "Open the interactive analytics dashboard in your browser",
		RunE:  runDashboard,
	}

	var analyzeCmd = &cobra.Command{
		Use:   "analyze",
		Short: "Run predictive analytics and anomaly detection",
		RunE:  runAnalyze,
	}

	var syncCmd = &cobra.Command{
		Use:   "sync",
		Short: "Synchronize telemetry database with the central Pith server",
		RunE:  runSync,
	}

	syncCmd.Flags().String("export", "", "Export all local telemetry executions to a JSONL file")
	syncCmd.Flags().String("import", "", "Import execution records from a JSONL file")

	resetCmd.Flags().Bool("all", false, "Reset ALL telemetry data (gain and discover)")
	resetCmd.Flags().Bool("discover", false, "Reset only discovery data (passthrough commands)")

	installCmd.Flags().Bool("all", false, "Setup hooks for all supported CLIs")
	installCmd.Flags().Bool("gemini", false, "Setup hook for Gemini CLI")
	installCmd.Flags().Bool("claude", false, "Setup hook for Claude Code")
	installCmd.Flags().Bool("codex", false, "Setup hook for Codex")
	installCmd.Flags().Bool("antigravity", false, "Setup hook for Antigravity")
	installCmd.Flags().Bool("pi", false, "Install the Pi tool_result hook")
	installCmd.Flags().BoolP("global", "g", false, "Install hooks globally in the home directory")

	dashboardCmd.Flags().IntP("port", "p", 8080, "Port to run the dashboard server on")

	hookCmd.Flags().String("source", "unknown", "The agent or CLI triggering the hook")

	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(gainCmd)
	rootCmd.AddCommand(discoverCmd)
	rootCmd.AddCommand(hookCmd)
	rootCmd.AddCommand(hookPreToolCmd)
	rootCmd.AddCommand(piCmd)
	rootCmd.AddCommand(codexWrapperRecordCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(rawCmd)
	rootCmd.AddCommand(dashboardCmd)
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(newAuditCmd())

	return rootCmd
}

func main() {
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(exitCode(err))
	}
}

// exitCode preserves normal child exits without changing generic CLI failures.
// Signal termination has no numeric exit code in Go and keeps the existing
// status 1 fallback; signal forwarding and shell conventions are separate.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
	}
	return 1
}

func runRoot(cmd *cobra.Command, args []string) error {
	setupHooks, _ := cmd.Flags().GetBool("hooks")
	if setupHooks {
		if len(args) != 0 {
			return errors.New("pith --hooks does not accept a command")
		}
		return installCodexHooksCommand(cmd)
	}
	if len(args) == 0 {
		return cmd.Help()
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
		if len(args) == 0 {
			return cmd.Help()
		}
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	if now-cfg.LastUpdateCheck > 86400 {
		cfg.LastUpdateCheck = now
		_ = cfg.Save()
		go func() {
			newTag, _ := selfupdate.CheckForUpdateSilent(version)
			if newTag != "" {
				cmd.PrintErrf("\n[Pith] A new version is available: %s. Run 'pith update' to upgrade!\n", newTag)
			}
		}()
	}
	tel, err := telemetry.NewTelemetry(cfg.StoragePath)
	if err != nil {
		return err
	}
	defer tel.Close()
	run := runner.NewRunner(cfg, tel)
	if src, _ := cmd.Flags().GetString("source"); src != "" {
		run.Source = src
	}
	if model, _ := cmd.Flags().GetString("model"); model != "" {
		run.Model = model
	}
	if cmd.Flags().Changed("model-cost") {
		cost, _ := cmd.Flags().GetFloat64("model-cost")
		run.InputCostPerMillion = &cost
	}
	return run.Run(args)
}

func runPiTransform(cmd *cobra.Command, args []string) error {
	// Preserve default-enabled CLI accounting while honoring explicit false.
	input := pi.HookRequest{TelemetryEnabled: true}
	decoder := json.NewDecoder(cmd.InOrStdin())
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return fmt.Errorf("decode Pi transform request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode Pi transform request: unexpected trailing JSON value")
		}
		return fmt.Errorf("decode Pi transform request: %w", err)
	}
	// Resolve parser settings read-only before consent can affect storage. A
	// legacy config must protect output even when migration is disabled.
	cfg, err := config.LoadConfigWithLegacyFallback()
	if err != nil {
		return err
	}
	input.EnabledParsers = cfg.EnabledParsers
	if input.StoragePath == "" {
		input.StoragePath = cfg.StoragePath
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(pi.OptimizeHook(input))
}

func runCodexWrapperRecord(cmd *cobra.Command, args []string) error {
	data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), codexWrapperRecordMaxJSONBytes+1))
	if err != nil {
		return fmt.Errorf("read wrapper telemetry request")
	}
	if len(data) > codexWrapperRecordMaxJSONBytes {
		return fmt.Errorf("wrapper telemetry request exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var request codexWrapperRecordRequest
	if err := decoder.Decode(&request); err != nil {
		return fmt.Errorf("decode wrapper telemetry request")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("wrapper telemetry request has trailing data")
	}
	var capture *telemetry.CodexCaptureObservation
	var proposalFamily string
	var legacy *codexWrapperRecordRequest
	var privateExecution bool
	switch request.Kind {
	case "proposal":
		if request.Family == "" || request.Parser != "" || request.CaptureOutcome != "" || request.ElapsedMs != nil || request.Selection != "" ||
			request.OriginalBytes != nil || request.DeliveredBytes != nil || request.OriginalRunes != nil ||
			request.DeliveredRunes != nil || request.DurationMs != nil || request.Outcome != "" {
			return fmt.Errorf("wrapper proposal request is invalid")
		}
		if !telemetry.IsCodexObservationFamilyAllowed(request.Family) {
			return fmt.Errorf("wrapper proposal family is invalid")
		}
		if !telemetry.IsCodexBypassReasonAllowed(request.BypassReason) {
			return fmt.Errorf("wrapper proposal bypass reason is invalid")
		}
		proposalFamily = request.Family
	case "execution":
		if request.BypassReason != "" || request.CaptureOutcome != "" || request.ElapsedMs != nil || request.Selection != "" ||
			request.OriginalBytes != nil || request.DeliveredBytes != nil ||
			request.Family == "" || !telemetry.IsCodexObservationFamilyAllowed(request.Family) {
			return fmt.Errorf("wrapper execution metadata is invalid")
		}
		if request.Parser != "" && !codexParserNameAllowed(request.Parser) {
			return fmt.Errorf("wrapper parser name is invalid")
		}
		if request.OriginalRunes == nil || request.DeliveredRunes == nil || request.DurationMs == nil {
			return fmt.Errorf("wrapper execution counts are required")
		}
		originalRunes, deliveredRunes, durationMs := *request.OriginalRunes, *request.DeliveredRunes, *request.DurationMs
		if originalRunes < 0 || originalRunes > codexWrapperRecordMaxRunes ||
			deliveredRunes < 0 || deliveredRunes > codexWrapperRecordMaxRunes ||
			durationMs < 0 || durationMs > codexWrapperRecordMaxDuration {
			return fmt.Errorf("wrapper execution counts are out of range")
		}
		switch request.Outcome {
		case "transformed":
			if request.Parser == "" || deliveredRunes >= originalRunes {
				return fmt.Errorf("transformed wrapper metadata is inconsistent")
			}
		case "raw_fallback":
			if request.Parser != "" || deliveredRunes != originalRunes {
				return fmt.Errorf("raw wrapper metadata is inconsistent")
			}
		default:
			return fmt.Errorf("wrapper execution outcome is invalid")
		}
		legacy = &request
		privateExecution = true
	case "capture":
		if request.BypassReason != "" || request.Family != "" || request.Parser != "" || request.OriginalRunes != nil || request.DeliveredRunes != nil || request.DurationMs != nil || request.Outcome != "" {
			return fmt.Errorf("wrapper capture request is invalid")
		}
		capture = &telemetry.CodexCaptureObservation{
			Outcome: request.CaptureOutcome, ElapsedMs: request.ElapsedMs, Selection: request.Selection,
			OriginalBytes: request.OriginalBytes, DeliveredBytes: request.DeliveredBytes,
		}
		if err := telemetry.ValidateCodexCaptureObservation(*capture); err != nil {
			return fmt.Errorf("wrapper capture request is invalid")
		}
	case "":
		// Continue to accept requests from an already installed v3.3.x adapter.
		if request.BypassReason != "" || request.CaptureOutcome != "" || request.ElapsedMs != nil || request.Selection != "" || request.Parser != "" ||
			request.OriginalBytes != nil || request.DeliveredBytes != nil || request.Family != "" {
			return fmt.Errorf("wrapper telemetry request is invalid")
		}
		if request.OriginalRunes == nil || request.DeliveredRunes == nil || request.DurationMs == nil {
			return fmt.Errorf("wrapper telemetry counts are required")
		}
		originalRunes := *request.OriginalRunes
		deliveredRunes := *request.DeliveredRunes
		durationMs := *request.DurationMs
		if originalRunes < 0 || originalRunes > codexWrapperRecordMaxRunes ||
			deliveredRunes < 0 || deliveredRunes > codexWrapperRecordMaxRunes ||
			durationMs < 0 || durationMs > codexWrapperRecordMaxDuration {
			return fmt.Errorf("wrapper telemetry counts are out of range")
		}
		switch request.Outcome {
		case "transformed":
			if deliveredRunes > originalRunes {
				return fmt.Errorf("transformed wrapper output exceeds original")
			}
		case "raw_fallback":
			if deliveredRunes != originalRunes {
				return fmt.Errorf("raw fallback counts must match")
			}
		default:
			return fmt.Errorf("wrapper telemetry outcome is invalid")
		}
		legacy = &request
	default:
		return fmt.Errorf("wrapper telemetry request kind is invalid")
	}

	// Validate configuration before opening or creating telemetry storage. The
	// wrapper's JSON request cannot select a path, source, family, or model.
	cfg, err := config.LoadConfigWithLegacyFallbackStrict()
	if err != nil {
		return err
	}
	tel, err := telemetry.NewPrivateTelemetry(cfg.StoragePath)
	if err != nil {
		return err
	}
	defer tel.Close()
	if proposalFamily != "" {
		return tel.RecordCodexProposal(proposalFamily, request.BypassReason)
	}
	if capture != nil {
		return tel.RecordCodexCapture(*capture)
	}
	originalRunes := *legacy.OriginalRunes
	deliveredRunes := *legacy.DeliveredRunes
	durationMs := *legacy.DurationMs
	decision := telemetry.DecisionTransformed
	passthrough := false
	parserName := "tests"
	commandFamily := "go test"
	if privateExecution {
		parserName = legacy.Parser
		commandFamily = legacy.Family
		if legacy.Outcome == "raw_fallback" {
			decision = telemetry.DecisionProtectedPassthrough
			passthrough = true
		}
	} else if legacy.Outcome == "raw_fallback" {
		decision = telemetry.DecisionProtectedPassthrough
		passthrough = true
		parserName = ""
	}
	return tel.Record(telemetry.ExecutionRecord{
		Timestamp:        time.Now().UTC(),
		Command:          commandFamily,
		OriginalTokens:   runner.EstimateTokensFromRuneCount(originalRunes, 4),
		CompressedTokens: runner.EstimateTokensFromRuneCount(deliveredRunes, 4),
		DurationMs:       durationMs,
		ParserUsed:       parserName,
		IsPassthrough:    passthrough,
		Source:           telemetry.SourceCodexWrapperPrivate,
		Harness:          "codex",
		DecisionReason:   decision,
	})
}

func codexParserNameAllowed(name string) bool {
	for _, candidate := range parser.GetAllParsers() {
		if candidate.Name() == name {
			return true
		}
	}
	return false
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	parsers := parser.GetAllParsers()
	var names []string
	for _, p := range parsers {
		names = append(names, p.Name())
	}
	return cfg.InteractiveConfig(names)
}

func runGain(cmd *cobra.Command, args []string) error {
	cfg, _ := config.LoadConfig()
	tel, err := telemetry.NewTelemetry(cfg.StoragePath)
	if err != nil {
		return err
	}
	defer tel.Close()

	totalOrig, totalComp, err := tel.GetStats("")
	if err != nil {
		return err
	}

	cmd.Printf("\n=== Pith: Overall Token Savings ===\n")
	if totalOrig == 0 {
		cmd.Println("No telemetry data recorded yet. Run some commands through Pith to start tracking savings!")
		cmd.Println()
		return nil
	}

	saved := totalOrig - totalComp
	percent := (float64(saved) / float64(totalOrig)) * 100
	modelSavings, err := tel.GetSavingsByModel()
	if err != nil {
		return err
	}
	var recordedUSD float64
	var fallbackTokens int
	for _, r := range modelSavings {
		recordedUSD += r.RecordedUSD
		fallbackTokens += r.FallbackTokens
	}
	fallbackUSD := (float64(fallbackTokens) / 1000000.0) * cfg.USDPerMillionTokens

	cmd.Printf("Raw Tokens:        %d\n", totalOrig)
	cmd.Printf("Compressed:        %d\n", totalComp)
	cmd.Printf("Tokens Saved:      %d (%.2f%%)\n", saved, percent)
	cmd.Printf("USD Saved:         $%.2f recorded + $%.2f fallback estimate\n", recordedUSD, fallbackUSD)
	if fallbackTokens > 0 {
		cmd.Printf("Fallback rate:     $%.2f/M tokens (%d saved tokens without an execution-time rate)\n", cfg.USDPerMillionTokens, fallbackTokens)
	}
	if len(modelSavings) > 0 {
		cmd.Printf("\n--- Savings by Model ---\n")
		cmd.Printf("%-30s | %-14s | %-10s | %-12s | %-12s\n", "Model", "Input Cost / 1M", "Saved", "Recorded USD", "Fallback Tokens")
		cmd.Println(strings.Repeat("-", 92))
		for _, r := range modelSavings {
			cost := "unknown"
			if r.InputCostPerMillion != nil {
				cost = fmt.Sprintf("$%.2f", *r.InputCostPerMillion)
			} else if r.FallbackTokens > 0 {
				cost = fmt.Sprintf("fallback $%.2f", cfg.USDPerMillionTokens)
			} else if r.RecordedUSD > 0 {
				cost = "mixed"
			}
			cmd.Printf("%-30s | %-14s | %-10d | $%-11.3f | %-15d\n", r.Model, cost, r.Original-r.Compressed, r.RecordedUSD, r.FallbackTokens)
		}
	}

	byHarness, _ := cmd.Flags().GetBool("by-harness")
	if cmd.CalledAs() == "stats" {
		byHarness = true
	}
	if byHarness {
		byH, err := tel.GetStatsByHarness()
		if err == nil && len(byH) > 0 {
			cmd.Printf("\n--- Breakdown by Harness ---\n")
			cmd.Printf("%-12s | %-10s | %-10s | %-10s | %-10s\n", "Harness", "Raw", "Pith", "Saved", "USD Saved")
			cmd.Println(strings.Repeat("-", 62))
			for _, r := range byH {
				savings := r.Original - r.Compressed
				cmd.Printf("%-12s | %-10d | %-10d | %-10d | %-10s\n", r.Harness, r.Original, r.Compressed, savings, "see models")
			}
		}
		// when by-harness requested, still show per-command breakdown but grouped deterministic: harness+command
		cmd.Println()
		return nil
	}

	byCmd, err := tel.GetStatsByCommand("")
	if err == nil && len(byCmd) > 0 {
		cmd.Printf("\n--- Breakdown by Command and Agent ---\n")
		cmd.Printf("%-25s | %-12s | %-10s | %-10s | %-10s\n", "Command Pattern", "Agent", "Raw", "Pith", "USD Saved")
		cmd.Println(strings.Repeat("-", 76))
		limit := len(byCmd)
		if limit > 20 {
			limit = 20
		}
		for i := 0; i < limit; i++ {
			r := byCmd[i]
			cmd.Printf("%-25s | %-12s | %-10d | %-10d | %-10s\n", r.Command, r.Source, r.Original, r.Compressed, "see models")
		}
		if len(byCmd) > 20 {
			cmd.Printf("... and %d more. Run 'pith dashboard' for full interactive breakdown.\n", len(byCmd)-20)
		}
	}

	unparsed, err := tel.GetUnparsedCommands("")
	if err == nil && len(unparsed) > 0 {
		cmd.Printf("\n--- Top Unparsed Commands (Discovery) ---\n")
		cmd.Printf("%-25s | %-12s | %-8s | %-12s | %-12s\n", "Command Pattern", "Agent", "Count", "Raw Tokens", "Est. Savings")
		cmd.Println(strings.Repeat("-", 80))
		limit := len(unparsed)
		if limit > 10 {
			limit = 10
		}
		for i := 0; i < limit; i++ {
			r := unparsed[i]
			estSavings := float64(r.TotalRawTokens) * 0.7
			cmd.Printf("%-25s | %-12s | %-8d | %-12d | %-12.0f\n", r.Pattern, r.Harness, r.InvocationCount, r.TotalRawTokens, estSavings)
		}
		if len(unparsed) > 10 {
			cmd.Printf("... and %d more. Run 'pith discover' for full list.\n", len(unparsed)-10)
		}
	}

	cmd.Println()
	return nil
}

func runDiscover(cmd *cobra.Command, args []string) error {
	cfg, _ := config.LoadConfig()
	tel, err := telemetry.NewTelemetry(cfg.StoragePath)
	if err != nil {
		return err
	}
	defer tel.Close()

	detailsFamily, _ := cmd.Flags().GetString("details")
	if detailsFamily != "" {
		limit, _ := cmd.Flags().GetInt("limit")
		details, err := tel.GetDiscoveryDetails(detailsFamily, "", limit)
		if err != nil {
			return err
		}
		if len(details) == 0 {
			cmd.Printf("No passthrough samples found for command family %q.\n", detailsFamily)
			return nil
		}
		cmd.Printf("--- Discovery Details: %s ---\n", detailsFamily)
		cmd.Printf("%-20s | %-12s | %-12s | %s\n", "Timestamp", "Harness", "Raw Tokens", "Redacted Command")
		cmd.Println(strings.Repeat("-", 90))
		for _, detail := range details {
			cmd.Printf("%-20s | %-12s | %-12d | %s\n", detail.Timestamp.Format("2006-01-02 15:04:05"), detail.Harness, detail.OriginalTokens, detail.Command)
		}
		return nil
	}

	families, err := tel.GetDiscoveryFamilies("")
	if err != nil {
		return err
	}
	observations, err := tel.GetCodexObservationSummary()
	if err != nil {
		return err
	}
	if len(families) == 0 && observations.CaptureRecords == 0 && len(observations.ProposalCounts) == 0 {
		cmd.Println("No unparsed command families discovered yet. Run some commands through Pith to gather data!")
		return nil
	}

	if len(families) > 0 {
		cmd.Println("--- Opportunity Discovery (Unparsed Command Families) ---")
		cmd.Printf("%-24s | %-12s | %-8s | %-15s | %-15s\n", "Command Family", "Harness", "Count", "Total Tokens", "Est. USD Saved")
		cmd.Println(strings.Repeat("-", 86))
		for _, family := range families {
			estSavedTokens := float64(family.TotalRawTokens) * 0.7
			estSavingsUSD := (estSavedTokens / 1000000.0) * cfg.USDPerMillionTokens
			cmd.Printf("%-24s | %-12s | %-8d | %-15d | $%-15.3f\n", family.Family, family.Harness, family.InvocationCount, family.TotalRawTokens, estSavingsUSD)
		}
		cmd.Println("\nUse 'pith discover --details <family>' for bounded redacted samples.")
	}
	if len(observations.ProposalCounts) > 0 || observations.CaptureRecords > 0 {
		printCodexObservationSummary(cmd, observations)
	}
	return nil
}

func printCodexObservationSummary(cmd *cobra.Command, summary telemetry.CodexObservationSummary) {
	cmd.Printf("\n--- Private Codex Observations (local; %d-day retention, %d-row limit) ---\n",
		telemetry.CodexObservationRetentionDays, telemetry.CodexObservationMaxRows)
	if len(summary.ProposalCounts) > 0 {
		families := make([]string, 0, len(summary.ProposalCounts))
		for family := range summary.ProposalCounts {
			families = append(families, family)
		}
		sort.Strings(families)
		cmd.Println("Unsupported proposed families:")
		for _, family := range families {
			cmd.Printf("  %-20s %d\n", family, summary.ProposalCounts[family])
		}
	}
	if len(summary.BypassCounts) > 0 {
		reasons := make([]string, 0, len(summary.BypassCounts))
		for reason := range summary.BypassCounts {
			reasons = append(reasons, reason)
		}
		sort.Strings(reasons)
		cmd.Println("Eligibility bypass reasons (unknown includes older rows):")
		for _, reason := range reasons {
			cmd.Printf("  %-32s %d\n", reason, summary.BypassCounts[reason])
		}
	}
	if summary.CaptureRecords > 0 {
		cmd.Println("Codex wrapper capture outcomes:")
		for _, outcome := range []string{
			telemetry.CodexCaptureComplete, telemetry.CodexCaptureDeadline,
			telemetry.CodexCaptureCap, telemetry.CodexCaptureIncomplete,
		} {
			cmd.Printf("  %-24s %d\n", outcome, summary.CaptureOutcomes[outcome])
		}
		cmd.Printf("Measurements: captures %d; native elapsed ms known/unknown %d/%d; byte counts known/unknown %d/%d; selection reduced/raw/unknown %d/%d/%d.\n",
			summary.CaptureRecords, summary.ElapsedKnown, summary.ElapsedUnknown,
			summary.ByteCountsKnown, summary.ByteCountsUnknown,
			summary.SelectionCounts[telemetry.CodexSelectionReduced],
			summary.SelectionCounts[telemetry.CodexSelectionRaw],
			summary.SelectionCounts[telemetry.CodexSelectionUnknown])
		cmd.Printf("Recorded reductions after captures longer than 2 seconds: %d captures, %d bytes.\n",
			summary.SlowReductionCount, summary.SlowReductionBytes)
	}
}

func runHook(cmd *cobra.Command, args []string) error {
	inputData, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	var input HookInput
	if err := json.Unmarshal(inputData, &input); err != nil {
		return err
	}
	var command string
	if cmdVal, ok := input.ToolInput["command"].(string); ok {
		command = cmdVal
	} else if input.ToolCallRequest.Arguments != "" {
		var toolArgs ToolArgs
		if err := json.Unmarshal([]byte(input.ToolCallRequest.Arguments), &toolArgs); err == nil {
			command = toolArgs.Command
		}
	}
	cfg, _ := config.LoadConfig()
	tel, _ := telemetry.NewTelemetry(cfg.StoragePath)
	if tel != nil {
		defer tel.Close()
	}
	now := time.Now().Unix()
	if now-cfg.LastUpdateCheck > 86400 {
		cfg.LastUpdateCheck = now
		_ = cfg.Save()
		go func() {
			newTag, _ := selfupdate.CheckForUpdateSilent(version)
			if newTag != "" {
				cmd.PrintErrf("\n[Pith] A new version is available: %s. Run 'pith update' to upgrade!\n", newTag)
			}
		}()
	}
	originalOutput := input.ToolResponse.LlmContent
	if command == "" {
		return respondAllow(cmd)
	}
	var prefix string
	exitCode := 0
	if strings.HasPrefix(originalOutput, "Output: ") {
		prefix = "Output: "
		originalOutput = strings.TrimPrefix(originalOutput, "Output: ")
	} else if strings.HasPrefix(originalOutput, "Error: ") {
		prefix = "Error: "
		originalOutput = strings.TrimPrefix(originalOutput, "Error: ")
		exitCode = 1
	}
	if strings.Contains(originalOutput, "Exit Code: ") && !strings.Contains(originalOutput, "Exit Code: 0") {
		exitCode = 1
	}
	run := runner.NewRunner(cfg, tel)
	run.LogForSnag(command, originalOutput, exitCode)
	source, _ := cmd.Flags().GetString("source")
	if source == "" {
		source = "unknown"
	}
	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		return respondAllow(cmd)
	}
	var p parser.Parser
	reason := telemetry.DecisionProtectedPassthrough
	// Quoted invocations and shell syntax are intentionally preserved rather
	// than normalized into a destructive parser for aggregate output.
	if !parser.MayContainShellSyntax(command) {
		reason = telemetry.DecisionUnsupportedParser
		for _, candidate := range parser.GetAllParsers() {
			enabled, configured := cfg.EnabledParsers[candidate.Name()]
			if (!configured || enabled) && candidate.CanParse(cmdParts[0], cmdParts[1:]) {
				p = candidate
				break
			}
		}
	}

	var compressed string
	parserUsed := "none"
	isPassthrough := true
	if p != nil {
		compressed = p.Parse(originalOutput)
		if compressed == originalOutput && identityParserNames[p.Name()] {
			// These legacy parsers can return the exact source unchanged. Keep
			// that provenance as raw; later middle-out truncation is classified
			// separately below.
			p = nil
			compressed = originalOutput
			reason = telemetry.DecisionUnsupportedParser
		} else {
			parserUsed = p.Name()
			isPassthrough = false
			reason = telemetry.DecisionTransformed
		}
	} else {
		compressed = originalOutput
	}
	truncated := run.ApplyMiddleOutTruncation(compressed)
	wasTruncated := truncated != compressed
	if wasTruncated {
		reason = telemetry.DecisionTransformed
	}
	origTokens := runner.EstimateTokensWithHeuristic(originalOutput, cfg.TokenHeuristic)
	compTokens := runner.EstimateTokensWithHeuristic(truncated, cfg.TokenHeuristic)
	savedTokens := origTokens - compTokens
	if tel != nil {
		_ = tel.Record(telemetry.ExecutionRecord{
			DecisionReason:   reason,
			Command:          command,
			OriginalTokens:   origTokens,
			CompressedTokens: compTokens,
			ParserUsed:       parserUsed,
			IsPassthrough:    isPassthrough,
			Source:           source,
			Harness:          source,
		})
	}
	if p == nil && !wasTruncated {
		return respondAllow(cmd)
	}
	output := HookOutput{
		Decision:      "deny",
		Reason:        prefix + truncated,
		SystemMessage: fmt.Sprintf("Output optimized by Pith (parser: %s, tokens saved: %d, truncated: %v)", parserUsed, savedTokens, wasTruncated),
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
}

var identityParserNames = map[string]bool{
	"source": true, "ls": true, "get-content": true,
}

func runHookPreTool(cmd *cobra.Command, args []string) error {
	return json.NewEncoder(cmd.OutOrStdout()).Encode(PreToolHookOutput{Decision: "allow"})
}

func runInstall(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool("all")
	gemini, _ := cmd.Flags().GetBool("gemini")
	claude, _ := cmd.Flags().GetBool("claude")
	codex, _ := cmd.Flags().GetBool("codex")
	piHook, _ := cmd.Flags().GetBool("pi")
	antigravity, _ := cmd.Flags().GetBool("antigravity")
	global, _ := cmd.Flags().GetBool("global")
	if antigravity {
		return fmt.Errorf("Antigravity integration is disabled pending a supported, reviewed implementation")
	}
	if err := installPithBinaryCommand(); err != nil {
		return err
	}
	if !all && !gemini && !claude && !codex && !piHook && !antigravity {
		all = true
		if !cmd.Flags().Changed("global") {
			global = true
		}
	}
	if all || gemini {
		if err := install.SetupGeminiHook(global); err != nil {
			cmd.PrintErrf("Gemini hook failed: %v\n", err)
		}
	}
	if all || claude {
		if err := install.SetupClaudeHook(global); err != nil {
			cmd.PrintErrf("Claude hook failed: %v\n", err)
		}
	}
	if all || codex {
		if err := install.SetupCodexHook(global); err != nil {
			cmd.PrintErrf("Codex hook failed: %v\n", err)
		}
	}
	if all || piHook {
		if err := install.SetupPiHook(); err != nil {
			cmd.PrintErrf("Pi hook failed: %v\n", err)
		}
	}
	return nil
}

func runUpdate(cmd *cobra.Command, args []string) error {
	updated, err := selfupdate.CheckAndApplyUpdate(version)
	if err != nil {
		return err
	}
	if !updated {
		cmd.Println("Pith is already up to date.")
	}
	return nil
}

func runReset(cmd *cobra.Command, args []string) error {
	cfg, _ := config.LoadConfig()
	tel, err := telemetry.NewTelemetry(cfg.StoragePath)
	if err != nil {
		return err
	}
	defer tel.Close()
	passthrough, _ := cmd.Flags().GetBool("discover")
	all, _ := cmd.Flags().GetBool("all")
	if all {
		if err := tel.ResetAll(); err != nil {
			return err
		}
		for _, name := range []string{"pith.log", "pith.log.1"} {
			if err := os.Remove(filepath.Join(cfg.StoragePath, name)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove diagnostic log: %w", err)
			}
		}
		cmd.Println("All telemetry data has been reset.")
		return nil
	}
	if passthrough {
		if err := tel.ResetPassthrough(); err != nil {
			return err
		}
		cmd.Println("Discovery data (passthrough commands) has been reset.")
		return nil
	}
	return fmt.Errorf("please specify what to reset using --all or --discover")
}

func runRaw(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	tel, err := telemetry.NewTelemetry(cfg.StoragePath)
	if err != nil {
		return err
	}
	defer tel.Close()
	run := runner.NewRunner(cfg, tel)
	return run.RunRaw(args)
}

func runDashboard(cmd *cobra.Command, args []string) error {
	cfg, _ := config.LoadConfig()
	tel, err := telemetry.NewTelemetry(cfg.StoragePath)
	if err != nil {
		return err
	}
	defer tel.Close()
	port, _ := cmd.Flags().GetInt("port")
	return gui.StartDashboard(cfg, tel, port)
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}

	tel, err := telemetry.NewTelemetry(cfg.StoragePath)
	if err != nil {
		return err
	}
	defer tel.Close()

	exportPath, _ := cmd.Flags().GetString("export")
	if exportPath != "" {
		f, err := os.Create(exportPath)
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Printf("Exporting telemetry to %s...\n", exportPath)
		if err := tel.ExportJSONL(f); err != nil {
			return err
		}
		fmt.Println("Export complete.")
		return nil
	}

	importPath, _ := cmd.Flags().GetString("import")
	if importPath != "" {
		f, err := os.Open(importPath)
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Printf("Importing telemetry from %s...\n", importPath)
		if err := tel.ImportJSONL(f); err != nil {
			return err
		}
		fmt.Println("Import complete.")
		return nil
	}

	serverURL := cfg.SyncServerURL
	if serverURL == "" {
		return fmt.Errorf("sync server URL not configured")
	}

	fmt.Printf("Syncing with central Pith server at %s...\n", serverURL)

	var pushBuf strings.Builder
	if err := tel.ExportJSONL(&pushBuf); err != nil {
		return fmt.Errorf("failed to export local executions for sync: %w", err)
	}

	importEndpoint := strings.TrimSuffix(serverURL, "/") + "/api/telemetry/push"
	importReq, err := http.NewRequest("POST", importEndpoint, strings.NewReader(pushBuf.String()))
	if err != nil {
		return err
	}
	importReq.Header.Set("Content-Type", "application/x-jsonlines")

	client := &http.Client{Timeout: 15 * time.Second}
	importResp, err := client.Do(importReq)
	if err != nil {
		return fmt.Errorf("failed to connect to central server for push: %w", err)
	}
	defer importResp.Body.Close()

	if importResp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(importResp.Body)
		return fmt.Errorf("push failed with status %d: %s", importResp.StatusCode, string(respBytes))
	}

	fmt.Println("✓ Local telemetry successfully pushed to central server.")

	pullEndpoint := strings.TrimSuffix(serverURL, "/") + "/api/telemetry/pull?since_id=0"
	pullReq, err := http.NewRequest("GET", pullEndpoint, nil)
	if err != nil {
		return err
	}

	pullResp, err := client.Do(pullReq)
	if err != nil {
		return fmt.Errorf("failed to connect to central server for pull: %w", err)
	}
	defer pullResp.Body.Close()

	if pullResp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(pullResp.Body)
		return fmt.Errorf("pull failed with status %d: %s", pullResp.StatusCode, string(respBytes))
	}

	if err := tel.ImportJSONL(pullResp.Body); err != nil {
		return fmt.Errorf("failed to import remote executions: %w", err)
	}

	fmt.Println("✓ Remote telemetry successfully pulled and merged locally.")
	fmt.Println("Estate telemetry synchronization complete.")

	return nil
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	cfg, _ := config.LoadConfig()
	tel, err := telemetry.NewTelemetry(cfg.StoragePath)
	if err != nil {
		return err
	}
	defer tel.Close()

	fmt.Println("\n🔍 Pith Predictive Analytics")
	fmt.Println("---------------------------------------")

	// 1. Anomaly Detection
	anomalies, err := anomaly.DetectUnusualConsumption(tel.DB)
	if err != nil {
		fmt.Printf("Anomaly Detection Error: %v\n", err)
	} else if len(anomalies) > 0 {
		fmt.Printf("⚠️  Detected %d Anomalies:\n", len(anomalies))
		for _, a := range anomalies {
			fmt.Printf("  - %s\n", a.Reason)
		}
	} else {
		fmt.Println("✅ No unusual token consumption detected.")
	}

	// 2. Cost Advisor
	estimate := advisor.EstimateSessionCost(10, 2000, cfg.USDPerMillionTokens)
	fmt.Printf("\n💰 %s\n", estimate.FormatReport())

	fmt.Println("---------------------------------------")
	return nil
}

func newAuditCmd() *cobra.Command {
	auditCmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit LLM interactions for quality and anomalies",
	}

	anomaliesCmd := &cobra.Command{
		Use:   "anomalies",
		Short: "Scan Overseer logs for hallucinations and low-quality responses",
		RunE:  runAuditAnomalies,
	}
	anomaliesCmd.Flags().Int("lookback", 60, "Lookback window in minutes")
	anomaliesCmd.Flags().Bool("diagnose", false, "Prepare local anomaly diagnostics")
	anomaliesCmd.Flags().Bool("allow-external-ai", false, "Explicitly allow sending bounded diagnostic data to Gemini")
	anomaliesCmd.Flags().Bool("json", false, "Output results in JSON format")

	auditCmd.AddCommand(anomaliesCmd)
	return auditCmd
}

func runAuditAnomalies(cmd *cobra.Command, args []string) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	lookbackMins, _ := cmd.Flags().GetInt("lookback")
	lookback := time.Duration(lookbackMins) * time.Minute

	if !jsonOutput {
		fmt.Printf("\nðŸ”  Auditing Overseer logs (last %d minutes)...\n", lookbackMins)
	}
	anomalies, err := anomaly.AuditOverseerLogs(lookback)
	if jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(anomalies)
	}
	if err != nil {
		return fmt.Errorf("failed to audit logs: %w", err)
	}

	if len(anomalies) == 0 {
		fmt.Println("âœ… No anomalies detected in recent telemetry.")
		return nil
	}

	fmt.Printf("âš ï¸  Detected %d Anomalies:\n", len(anomalies))
	fmt.Println(strings.Repeat("-", 60))
	diagnose, _ := cmd.Flags().GetBool("diagnose")
	allowExternalAI, _ := cmd.Flags().GetBool("allow-external-ai")
	if diagnose && !allowExternalAI {
		return fmt.Errorf("external AI diagnostics require --allow-external-ai; no telemetry was sent")
	}

	for _, a := range anomalies {
		fmt.Printf("[%s] [%s] %s\n", a.Timestamp.Format("15:04:05"), a.Project, a.Reason)
		fmt.Printf("  Model: %s\n", a.Model)
		fmt.Printf("  Prompt: %.50s...\n", a.Prompt)
		fmt.Printf("  Response: %.100s...\n", a.Response)

		if diagnose {
			diagnosis, err := anomaly.Diagnose(a)
			if err != nil {
				fmt.Printf("\n  [Diagnostic Error] %v\n", err)
			} else {
				fmt.Printf("\n  [Forensic Analysis]\n%s\n", diagnosis)
			}
		}
		fmt.Println()
	}

	return nil
}
