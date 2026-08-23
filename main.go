package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	"runtime"
	"strings"
	"time"
)

const version = "v2.4.0"

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
		Short: "Run a command and bypass all parsers (escape hatch)",
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
	cfg, err := config.LoadConfig()
	if err == nil {
		_ = config.MigrateStorage(cfg.StoragePath)
	}
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runRoot(cmd *cobra.Command, args []string) error {
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
	var input pi.HookRequest
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
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	input.EnabledParsers = cfg.EnabledParsers
	if input.StoragePath == "" {
		input.StoragePath = cfg.StoragePath
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(pi.OptimizeHook(input))
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
	if len(families) == 0 {
		cmd.Println("No unparsed command families discovered yet. Run some commands through Pith to gather data!")
		return nil
	}

	cmd.Println("--- Opportunity Discovery (Unparsed Command Families) ---")
	cmd.Printf("%-24s | %-12s | %-8s | %-15s | %-15s\n", "Command Family", "Harness", "Count", "Total Tokens", "Est. USD Saved")
	cmd.Println(strings.Repeat("-", 86))
	for _, family := range families {
		estSavedTokens := float64(family.TotalRawTokens) * 0.7
		estSavingsUSD := (estSavedTokens / 1000000.0) * cfg.USDPerMillionTokens
		cmd.Printf("%-24s | %-12s | %-8d | %-15d | $%-15.3f\n", family.Family, family.Harness, family.InvocationCount, family.TotalRawTokens, estSavingsUSD)
	}
	cmd.Println("\nUse 'pith discover --details <family>' for bounded redacted samples.")
	return nil
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
	parsers := parser.GetAllParsers()
	var p parser.Parser
	cmdName := cmdParts[0]
	pArgs := cmdParts[1:]
	for _, parser := range parsers {
		enabled, ok := cfg.EnabledParsers[parser.Name()]
		if (!ok || enabled) && parser.CanParse(cmdName, pArgs) {
			p = parser
			break
		}
	}
	if p == nil && cmdName == "&" && len(cmdParts) > 1 {
		cmdName = cmdParts[1]
		pArgs = cmdParts[2:]
		if strings.HasPrefix(cmdName, "\"") && strings.HasSuffix(cmdName, "\"") {
			cmdName = strings.Trim(cmdName, "\"")
		}
		for _, parser := range parsers {
			enabled, ok := cfg.EnabledParsers[parser.Name()]
			if (!ok || enabled) && parser.CanParse(cmdName, pArgs) {
				p = parser
				break
			}
		}
	}
	if p == nil && strings.HasPrefix(command, "\"") {
		endQuote := strings.Index(command[1:], "\"")
		if endQuote != -1 {
			cmdName = command[1 : endQuote+1]
			remaining := strings.TrimSpace(command[endQuote+2:])
			pArgs = strings.Fields(remaining)
			for _, parser := range parsers {
				enabled, ok := cfg.EnabledParsers[parser.Name()]
				if (!ok || enabled) && parser.CanParse(cmdName, pArgs) {
					p = parser
					break
				}
			}
		}
	}
	var compressed string
	parserUsed := "none"
	isPassthrough := true
	if p != nil {
		compressed = p.Parse(originalOutput)
		parserUsed = p.Name()
		isPassthrough = false
	} else {
		compressed = originalOutput
	}
	truncated := run.ApplyMiddleOutTruncation(compressed)
	wasTruncated := truncated != compressed
	origTokens := runner.EstimateTokensWithHeuristic(originalOutput, cfg.TokenHeuristic)
	compTokens := runner.EstimateTokensWithHeuristic(truncated, cfg.TokenHeuristic)
	savedTokens := origTokens - compTokens
	if tel != nil {
		_ = tel.Record(telemetry.ExecutionRecord{
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

func runHookPreTool(cmd *cobra.Command, args []string) error {
	inputData, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(PreToolHookOutput{Decision: "allow"})
	}

	var input PreToolHookInput
	if err := json.Unmarshal(inputData, &input); err != nil {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(PreToolHookOutput{Decision: "allow"})
	}

	if input.ToolCall.Name != "run_command" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(PreToolHookOutput{Decision: "allow"})
	}

	if isDaemon, ok := input.ToolCall.Args["IsDaemon"].(bool); ok && isDaemon {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(PreToolHookOutput{Decision: "allow"})
	}

	rawCmd, ok := input.ToolCall.Args["CommandLine"].(string)
	if !ok || strings.TrimSpace(rawCmd) == "" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(PreToolHookOutput{Decision: "allow"})
	}

	trimmed := strings.TrimSpace(rawCmd)
	if strings.HasPrefix(trimmed, "pith") ||
		strings.HasPrefix(trimmed, "pith.exe") ||
		strings.Contains(trimmed, "\\pith.exe") ||
		strings.Contains(trimmed, "/pith") ||
		(strings.HasPrefix(trimmed, "& \"") && strings.Contains(trimmed, "pith")) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(PreToolHookOutput{Decision: "allow"})
	}

	home, _ := os.UserHomeDir()
	exePath := filepath.Join(home, ".pith", "bin", "pith.exe")
	if runtime.GOOS != "windows" {
		exePath = filepath.Join(home, ".pith", "bin", "pith")
	}

	source, _ := cmd.Flags().GetString("source")
	if source == "" {
		source = "antigravity"
	}

	modelName := input.ModelName
	if modelName == "" || modelName == "unknown" || modelName == "auto" {
		modelName = "gemini-auto"
	}
	modelArg := fmt.Sprintf(" --model %s", modelName)

	var rewritten string
	if runtime.GOOS == "windows" {
		escaped := strings.ReplaceAll(rawCmd, "'", "''")
		rewritten = fmt.Sprintf("& \"%s\" --source %s%s -- '%s'", exePath, source, modelArg, escaped)
	} else {
		escaped := strings.ReplaceAll(rawCmd, "'", "'\\''")
		rewritten = fmt.Sprintf("\"%s\" --source %s%s -- '%s'", exePath, source, modelArg, escaped)
	}

	output := PreToolHookOutput{
		Decision: "allow",
		Overwrite: map[string]interface{}{
			"CommandLine": rewritten,
		},
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
}

func runInstall(cmd *cobra.Command, args []string) error {
	if err := install.Install(); err != nil {
		return err
	}
	all, _ := cmd.Flags().GetBool("all")
	gemini, _ := cmd.Flags().GetBool("gemini")
	claude, _ := cmd.Flags().GetBool("claude")
	codex, _ := cmd.Flags().GetBool("codex")
	piHook, _ := cmd.Flags().GetBool("pi")
	antigravity, _ := cmd.Flags().GetBool("antigravity")
	global, _ := cmd.Flags().GetBool("global")
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
	if all || antigravity {
		if err := install.SetupAntigravityHook(global); err != nil {
			cmd.PrintErrf("Antigravity hook failed: %v\n", err)
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
	return run.RunWithOptions(args, true)
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
