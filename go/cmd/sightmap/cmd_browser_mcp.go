// WebMCP tooling: `browser mcp list` / `browser mcp call`. A growing number of
// pages expose callable actions to an in-browser agent via WebMCP —
// `document.modelContext` with getTools() / executeTool(). These commands
// enumerate and invoke those tools through the standard surface (over the same
// awaited-eval path as `browser eval`), so an agent can call a named action
// instead of driving the page blind, and get a loud, actionable message when a
// page has no WebMCP at all.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sightmap/sightmap/go/browser"
)

func runMCP(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: browser mcp <list|call> [flags]")
	}
	switch args[0] {
	case "list":
		return runMCPList(args[1:])
	case "call":
		return runMCPCall(args[1:])
	default:
		return fmt.Errorf("browser mcp: unknown subcommand %q (want list|call)", args[0])
	}
}

// ── shared ──────────────────────────────────────────────────────────────────

type mcpToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type mcpListResult struct {
	Present bool          `json:"present"`
	Native  bool          `json:"native"`
	Tools   []mcpToolInfo `json:"tools"`
}

type mcpCallResult struct {
	Present bool            `json:"present"`
	Found   bool            `json:"found"`
	Names   []string        `json:"names"`
	Result  json.RawMessage `json:"result"`
	Error   string          `json:"error"`
}

// mcpAbsentError is the loud, actionable message for a page with no WebMCP
// surface — the "absent" arm of the native/polyfilled/absent detection. Native
// WebMCP is behind Chrome flags, so it names them; a page can also register
// tools via its own script.
func mcpAbsentError() error {
	return fmt.Errorf("no WebMCP tools here: document.modelContext is not available on this page.\n" +
		"  Native WebMCP needs Chrome flags — start the session with:\n" +
		"    sightmap browser start \\\n" +
		"      --chrome-flag=--enable-blink-features=ModelContext,ModelContextTesting \\\n" +
		"      --chrome-flag=--enable-features=DevToolsWebMCPSupport\n" +
		"  A page can also register tools via its own script; if you expect tools here, confirm it ran.")
}

func mcpConnect(addr, tab, sightmapDir string) (*browser.CDPConn, error) {
	return browser.Connect(resolveCDPAddr(addr, sightmapDir), tab)
}

// ── list ────────────────────────────────────────────────────────────────────

// mcpListScript enumerates the page's WebMCP tools. It distinguishes absent
// (no modelContext) from present, and reports whether the implementation looks
// native (a UA-provided getTools reads as "[native code]") vs a JS polyfill.
const mcpListScript = `(async () => {
  const mc = document.modelContext;
  if (!mc || typeof mc.getTools !== 'function') return { present: false };
  const native = /\[native code\]/.test(Function.prototype.toString.call(mc.getTools));
  const tools = (await mc.getTools()) || [];
  return {
    present: true,
    native: native,
    tools: tools.map(t => ({ name: t && t.name, description: t && t.description, inputSchema: t && t.inputSchema }))
  };
})()`

func runMCPList(args []string) error {
	fs := flag.NewFlagSet("mcp list", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	jsonFlag := fs.Bool("json", false, "Emit the full tool list (with input schemas) as JSON")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}

	conn, err := mcpConnect(*addrFlag, *tabFlag, *sightmapDirFlag)
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), evalTimeout)
	defer cancel()

	raw, err := browser.EvalJSON(ctx, conn, mcpListScript)
	if err != nil {
		return err
	}
	var res mcpListResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return fmt.Errorf("mcp list: parse page response: %w", err)
	}
	if !res.Present {
		return mcpAbsentError()
	}

	if *jsonFlag {
		out, _ := json.MarshalIndent(res.Tools, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	impl := "polyfilled"
	if res.Native {
		impl = "native"
	}
	if len(res.Tools) == 0 {
		fmt.Fprintf(os.Stderr, "WebMCP present (%s) but no tools registered on this page.\n", impl)
		return nil
	}
	fmt.Fprintf(os.Stderr, "WebMCP (%s) — %d tool(s):\n", impl, len(res.Tools))
	for _, t := range res.Tools {
		if t.Description != "" {
			fmt.Printf("  %s — %s\n", t.Name, t.Description)
		} else {
			fmt.Printf("  %s\n", t.Name)
		}
	}
	fmt.Fprintln(os.Stderr, "(use --json for full input schemas; call one with 'browser mcp call <tool> --args {…}')")
	return nil
}

// ── call ────────────────────────────────────────────────────────────────────

// mcpCallScript resolves a tool by name and invokes it via executeTool(tool,
// args). It reports absent / not-found (with the available names) / a thrown
// error distinctly, so the CLI can fail loudly rather than returning a bare {}.
func mcpCallScript(name, argsJSON string) string {
	nameLit, _ := json.Marshal(name) // safe JS string literal
	return fmt.Sprintf(`(async () => {
  const mc = document.modelContext;
  if (!mc || typeof mc.getTools !== 'function' || typeof mc.executeTool !== 'function') return { present: false };
  const tools = (await mc.getTools()) || [];
  const name = %s;
  const tool = tools.find(t => t && t.name === name);
  if (!tool) return { present: true, found: false, names: tools.map(t => t && t.name).filter(Boolean) };
  try {
    const result = await mc.executeTool(tool, %s);
    return { present: true, found: true, result: result };
  } catch (e) {
    return { present: true, found: true, error: String((e && e.message) || e) };
  }
})()`, nameLit, argsJSON)
}

// normalizeMCPArgs validates the --args JSON and returns it (defaulting to an
// empty object). WebMCP tool arguments are a JSON object, so a non-object is
// rejected up front with an actionable message rather than failing in the page.
func normalizeMCPArgs(raw string) (string, error) {
	if raw == "" {
		return "{}", nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", fmt.Errorf("--args must be a JSON object (e.g. '{\"query\":\"ATL to LHR\"}'): %w", err)
	}
	return raw, nil
}

func runMCPCall(args []string) error {
	fs := flag.NewFlagSet("mcp call", flag.ContinueOnError)
	addrFlag := fs.String("addr", "", "CDP address (default: the session for --sightmap-dir)")
	tabFlag := fs.String("tab", "", "Target tab ID (from 'browser start' output)")
	sightmapDirFlag := fs.String("sightmap-dir", ".sightmap", "Path to .sightmap/ dir (keys session lookup)")
	argsFlag := fs.String("args", "", "Tool arguments as a JSON object (default: {})")
	jsonFlag := fs.Bool("json", false, "Emit the raw result value as compact JSON")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("usage: browser mcp call <tool> [--args JSON]")
	}
	toolName := rest[0]
	argsJSON, err := normalizeMCPArgs(*argsFlag)
	if err != nil {
		return err
	}

	conn, err := mcpConnect(*addrFlag, *tabFlag, *sightmapDirFlag)
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), evalTimeout)
	defer cancel()

	raw, err := browser.EvalJSON(ctx, conn, mcpCallScript(toolName, argsJSON))
	if err != nil {
		return err
	}
	var res mcpCallResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return fmt.Errorf("mcp call: parse page response: %w", err)
	}
	if !res.Present {
		return mcpAbsentError()
	}
	if !res.Found {
		if len(res.Names) == 0 {
			return fmt.Errorf("mcp call: tool %q not found (no tools registered on this page)", toolName)
		}
		return fmt.Errorf("mcp call: tool %q not found. Available: %s", toolName, strings.Join(res.Names, ", "))
	}
	if res.Error != "" {
		return fmt.Errorf("mcp call: tool %q threw: %s (args: %s)", toolName, res.Error, argsJSON)
	}

	// The result may carry the tool's own guidance breadcrumbs (e.g. an
	// after_navigation hint toward the next tool); printing it whole surfaces them.
	if len(res.Result) == 0 {
		res.Result = json.RawMessage("null")
	}
	if *jsonFlag {
		fmt.Println(string(res.Result))
		return nil
	}
	var v any
	if err := json.Unmarshal(res.Result, &v); err == nil {
		out, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Println(string(res.Result))
	}
	return nil
}
