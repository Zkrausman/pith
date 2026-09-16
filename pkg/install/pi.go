package install

import (
	"fmt"
	"os"
	"path/filepath"
)

// SetupPiHook installs Pith's global Pi extension. Project policy stays in the
// trusted project's .pi/pith.json; no consumer repository policy is embedded.
func SetupPiHook() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".pi", "agent", "extensions", "pith")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "index.ts")
	if err := os.WriteFile(path, []byte(piExtension), 0644); err != nil {
		return err
	}
	fmt.Printf("Installed Pi hook at %s\n", path)
	return nil
}

const piExtension = `import { spawn } from "node:child_process";
import { readFile } from "node:fs/promises";
import { homedir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

type Config = { enabled?: boolean; rawBypass?: boolean };
async function config(cwd: string): Promise<Config> {
  try { return JSON.parse(await readFile(join(cwd, ".pi", "pith.json"), "utf8")); } catch { return {}; }
}
function transform(binary: string, request: unknown, signal?: AbortSignal): Promise<any> {
  if (signal?.aborted) return Promise.reject(new Error("Pith transform aborted"));
  return new Promise((resolve, reject) => {
    const child = spawn(binary, ["pi", "transform"], { stdio: ["pipe", "pipe", "ignore"] });
    let out = "";
    child.stdout.on("data", (chunk) => { out += String(chunk); });
    child.once("error", reject);
    child.once("close", (code) => {
      if (code !== 0) return reject(new Error("pith exited " + code));
      try { resolve(JSON.parse(out)); } catch (error) { reject(error); }
    });
    signal?.addEventListener("abort", () => child.kill(), { once: true });
    child.stdin.end(JSON.stringify(request));
  });
}
export default function (pi: ExtensionAPI) {
  pi.on("tool_result", async (event, ctx) => {
    if (!ctx.isProjectTrusted()) return;
    const command = (event.input as { command?: unknown }).command;
    if (event.toolName !== "bash" || typeof command !== "string") return;
    const blocks = event.content as Array<{ type: string; text?: string }>;
    if (!Array.isArray(blocks) || blocks.length !== 1 || blocks[0]?.type !== "text" || typeof blocks[0].text !== "string") return;
    const output = blocks[0].text;
    const cfg = await config(ctx.cwd);
    if (cfg.enabled === false || cfg.rawBypass) return;
    try {
      const binary = process.env.PITH_BIN || join(homedir(), ".pith", "bin", process.platform === "win32" ? "pith.exe" : "pith");
      const exitCode = event.isError ? 1 : Number((event.details as any)?.exitCode ?? 0);
      const model = ctx.model as { provider?: unknown; id?: unknown; cost?: { input?: unknown } } | undefined;
      const provider = typeof model?.provider === "string" ? model.provider : "";
      const modelID = typeof model?.id === "string" ? model.id : "unknown";
      const inputCostPerMillion = typeof model?.cost?.input === "number" && Number.isFinite(model.cost.input) && model.cost.input >= 0 ? model.cost.input : undefined;
      const response = await transform(binary, { command, output, exitCode, telemetryEnabled: true, model: provider && modelID !== "unknown" ? provider + "/" + modelID : modelID, inputCostPerMillion }, ctx.signal);
      if (typeof response?.output !== "string") return;
      return { content: [{ type: "text", text: response.output }], details: { ...event.details, pith: {
        parser: response.parser, passthrough: response.passthrough,
        originalLineCount: response.originalLineCount, retainedLineCount: response.retainedLineCount,
        originalByteCount: response.originalByteCount, retainedByteCount: response.retainedByteCount,
        omittedLineCount: response.omittedLineCount, omittedByteCount: response.omittedByteCount,
        parserNetReductionKnown: response.parserNetReductionKnown,
        parserNetLineReduction: response.parserNetLineReduction, parserNetByteReduction: response.parserNetByteReduction,
        minimizationStrategy: response.minimizationStrategy, upstreamTruncated: response.upstreamTruncated
      } } };
    } catch { return; } // Pith failure always preserves Pi's original result.
  });
}
`
