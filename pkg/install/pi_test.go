package install

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiExtensionRejectsMalformedPithResponse(t *testing.T) {
	if !strings.Contains(piExtension, "try { resolve(JSON.parse(out)); } catch (error) { reject(error); }") {
		t.Fatal("Pi extension must reject malformed Pith JSON so its hook falls back to Pi's original result")
	}
	if !strings.Contains(piExtension, "catch { return; } // Pith failure always preserves Pi's original result.") {
		t.Fatal("Pi extension must preserve Pi result when Pith fails")
	}
}

func TestPiExtensionPreservesHostDetailsAndUsesTransformOnly(t *testing.T) {
	for _, field := range []string{"...event.details", "exitCode", "pith", "pi", "transform"} {
		if !strings.Contains(piExtension, field) {
			t.Errorf("Pi extension missing integration field %q", field)
		}
	}
	if strings.Contains(piExtension, "spawn(command") || strings.Contains(piExtension, "exec(command") {
		t.Fatal("Pi extension must not execute or rewrite the original command")
	}
}

func TestPiExtensionPassesModelPricing(t *testing.T) {
	for _, field := range []string{"ctx.model", "inputCostPerMillion", "Number.isFinite(model.cost.input)", "provider + \"/\" + modelID"} {
		if !strings.Contains(piExtension, field) {
			t.Errorf("Pi extension is missing model pricing field %q", field)
		}
	}
}

func TestPiExtensionSafetyGuards(t *testing.T) {
	for _, guard := range []string{
		"if (!ctx.isProjectTrusted()) return;",
		"blocks.length !== 1",
		"if (signal?.aborted) return Promise.reject",
		"process.env.PITH_BIN || join(homedir(), \".pith\", \"bin\", process.platform === \"win32\" ? \"pith.exe\" : \"pith\")",
	} {
		if !strings.Contains(piExtension, guard) {
			t.Errorf("Pi extension is missing safety guard %q", guard)
		}
	}
}

func TestPiExtensionExecutableIntegration(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required for generated-extension integration test")
	}
	source, err := json.Marshal(piExtension)
	if err != nil {
		t.Fatal(err)
	}
	harness := `const source = %s;
const calls = []; let failNext = false; let registered;
function spawn(binary, args) {
  calls.push({binary, args}); const listeners = {};
  return { stdout: {on(n, f) {listeners[n] = f;}},
    stdin: {end(data) { if (failNext) {listeners.error(new Error("mock failure")); return;}
      calls[calls.length-1].request = JSON.parse(data);
      listeners.data(JSON.stringify({output:"MINIMIZED", parser:"shell", passthrough:false, originalLineCount:9, retainedLineCount:2, originalByteCount:90, retainedByteCount:20, omittedLineCount:7, omittedByteCount:70, parserNetReductionKnown:true, parserNetLineReduction:7, parserNetByteReduction:70, minimizationStrategy:"deterministic", upstreamTruncated:true})); listeners.close(0);}},
    once(n, f) {listeners[n] = f;}, kill() {}};
}
const readFile = async () => {throw new Error("no config")}; const homedir = () => "/isolated-home"; const join = (...p) => p.join("/");
const executable = source.replace(/^import .*\n/gm, "").replace(/^type Config = .*\n/gm, "").replace("async function config(cwd: string): Promise<Config>", "async function config(cwd)").replace("function transform(binary: string, request: unknown, signal?: AbortSignal): Promise<any>", "function transform(binary, request, signal)").replace("(event.input as { command?: unknown })", "event.input").replace("(event.details as any)", "event.details").replace("event.content as Array<{ type: string; text?: string }>", "event.content").replace("ctx.model as { provider?: unknown; id?: unknown; cost?: { input?: unknown } } | undefined", "ctx.model").replace("export default function (pi: ExtensionAPI)", "function extension(pi)");
const extension = new Function("spawn","readFile","homedir","join",executable+"\nreturn extension;")(spawn,readFile,homedir,join);
extension({on(n, f) {registered = f;}}); const ctx = {cwd:"/workspace", signal:undefined, isProjectTrusted:()=>true, model:{provider:"acme",id:"model-7",cost:{input:2.5}}};
const event = {toolName:"bash", input:{command:"printf 'do not execute' && rm -rf /"}, content:[{type:"text",text:"line1\\nline2"}], isError:false, details:{exitCode:17,truncated:true,fullOutputPath:"/tmp/full-output"}};
const result = await registered(event,ctx); failNext=true; const failure = await registered(event,ctx); console.log(JSON.stringify({calls,result,failure}));
`
	harness = fmt.Sprintf(harness, string(source))
	path := filepath.Join(t.TempDir(), "pi-harness.mjs")
	if err := os.WriteFile(path, []byte(harness), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, path).CombinedOutput()
	if err != nil {
		t.Fatalf("generated Pi extension harness failed: %v\n%s", err, output)
	}
	var got struct {
		Calls []struct {
			Binary  string         `json:"binary"`
			Args    []string       `json:"args"`
			Request map[string]any `json:"request"`
		} `json:"calls"`
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Details map[string]any `json:"details"`
		} `json:"result"`
		Failure any `json:"failure"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("invalid harness output: %v\n%s", err, output)
	}
	if len(got.Calls) != 2 {
		t.Fatalf("expected two transform attempts, got %d", len(got.Calls))
	}
	for _, call := range got.Calls {
		if len(call.Args) != 2 || call.Args[0] != "pi" || call.Args[1] != "transform" {
			t.Errorf("unexpected spawn arguments: %#v", call.Args)
		}
	}
	request := got.Calls[0].Request
	if request["command"] != "printf 'do not execute' && rm -rf /" {
		t.Errorf("original command changed: %v", request["command"])
	}
	if request["exitCode"] != float64(17) || request["model"] != "acme/model-7" || request["inputCostPerMillion"] != 2.5 {
		t.Errorf("request provenance not forwarded: %#v", request)
	}
	if len(got.Result.Content) != 1 || got.Result.Content[0].Text != "MINIMIZED" {
		t.Errorf("unexpected transformed result: %#v", got.Result)
	}
	for _, field := range []string{"exitCode", "truncated", "fullOutputPath", "pith"} {
		if _, ok := got.Result.Details[field]; !ok {
			t.Errorf("result lost detail %q", field)
		}
	}
	if got.Failure != nil {
		t.Errorf("Pith failure did not preserve original result: %v", got.Failure)
	}
}
