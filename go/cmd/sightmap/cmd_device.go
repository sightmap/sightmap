package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sightmap/sightmap/go/device"
)

func deviceUsage() {
	fmt.Fprint(os.Stderr, `sightmap device — drive an app in the local iOS Simulator (macOS, Apple Silicon, Xcode 26+)

Session:
  doctor [--fix]                                    check prerequisites; builds the simulator helper on first use
  start [--device NAME|UDID] [--app PATH | --bundle-id ID] [--label L] [--console-limit N] [--detach]
                                                    boot, enable accessibility, install, launch, run the daemon
                                                    (blocks the shell; --detach backgrounds it). On a running
                                                    session: reinstall (--app) and relaunch the same app
  status                                            session health
  stop [--shutdown]                                 stop the daemon and the app (--shutdown: the simulator too)
  launch | relaunch                                 foreground the app | restart it (re-enables accessibility)

Observe:
  snapshot [--view V] [--include-offscreen] [--out F] [--tree-out F]
  screenshot --out F.png
  console list [--limit N] [--grep S] [--level L[,L]] [--all] [--since INDEX] [--follow]
                                                    levels: debug info default error fault stdout stderr;
                                                    --grep is case-insensitive; --all adds Apple frameworks;
                                                    --since pages forward from an index; --follow streams
  console get INDEX

Act (TARGET: snapshot id, accessibility identifier, component query, or a selector):
  click TARGET
  fill TARGET TEXT [--clear]
  scroll up|down|left|right [--in TARGET] [--until TARGET] [--max N]
  swipe X1 Y1 X2 Y2 [--duration S]                  screen points (not pixels), origin top-left
  tap-xy X Y                                        screen points; off-screen points are refused
  type TEXT
  key NAME                                          return delete tab escape space up down left right …
  button NAME                                       home lock siri side apple_pay volume_up volume_down
  openurl URL

Run log (.sightmap/.device-runs/<run>.jsonl: one line per action):
  run new [--label L] | run current
  run note TEXT [--kind K]                          record an annotation, e.g. --kind session-start
  run tail [--follow] [--since SEQ] [--run ID]

Every command takes --json and --sightmap-dir DIR (default .sightmap).
`)
}

// deviceFlags is the flag set every device command shares.
type deviceFlags struct {
	fs          *flag.FlagSet
	json        *bool
	sightmapDir *string
}

func newDeviceFlags(name string) *deviceFlags {
	fs := flag.NewFlagSet("device "+name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return &deviceFlags{
		fs:          fs,
		json:        fs.Bool("json", false, "machine-readable output"),
		sightmapDir: fs.String("sightmap-dir", ".sightmap", "path to .sightmap/ dir"),
	}
}

// deviceExit carries a device command's exit status; runDevice has already
// printed whatever the user needs to see.
type deviceExit struct{ code int }

func (e deviceExit) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func usageErr(format string, args ...any) error {
	return &device.Error{Code: "usage", Message: fmt.Sprintf(format, args...), Hint: "run `sightmap device help`"}
}

func runDevice(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		deviceUsage()
		return nil
	}
	cmd, rest := args[0], args[1:]
	if cmd == "_daemon" {
		return runDeviceDaemon(rest)
	}
	f := newDeviceFlags(cmd)
	err := deviceCommand(cmd, rest, f)
	if err == nil {
		return nil
	}
	var exit deviceExit
	if errors.As(err, &exit) {
		os.Exit(exit.code)
	}
	if errors.Is(err, flag.ErrHelp) {
		deviceUsage()
		return nil
	}
	e := device.AsError(err)
	if *f.json {
		printJSON(map[string]any{"ok": false, "error": e})
	} else {
		fmt.Fprintln(os.Stderr, "error: "+e.Message)
		if e.Hint != "" {
			fmt.Fprintln(os.Stderr, "Next: "+e.Hint)
		}
	}
	if e.Code == "usage" {
		os.Exit(2)
	}
	os.Exit(1)
	return nil
}

func printJSON(v any) {
	data, _ := json.Marshal(v)
	fmt.Println(string(data))
}

func deviceCommand(cmd string, args []string, f *deviceFlags) error {
	switch cmd {
	case "doctor":
		return deviceDoctor(args, f)
	case "start":
		return deviceStart(args, f)
	case "status":
		return deviceStatus(args, f)
	case "stop":
		return deviceStop(args, f)
	case "run":
		return deviceRun(args, f)
	}
	req, err := deviceRequest(cmd, args, f)
	if err != nil {
		return err
	}
	session, err := device.RequireSession(*f.sightmapDir)
	if err != nil {
		return err
	}
	if req.follow {
		return followConsole(session, req.args.(device.ConsoleFilter), *f.json)
	}
	var result json.RawMessage
	if err := device.Call(session.Socket, req.cmd, req.args, &result, 2*time.Minute); err != nil {
		return err
	}
	// A snapshot with nothing to act on is printed, but fails: the app is most
	// likely still loading, and a script must not take it for a real screen.
	var empty *device.Error
	if req.cmd == "snapshot" {
		var r device.SnapshotResult
		json.Unmarshal(result, &r)
		if r.Coverage.Interactive == 0 {
			empty = &device.Error{Code: "empty", Message: "0 interactive elements; the app may still be loading", Hint: "wait a moment and snapshot again"}
		}
	}
	switch {
	case *f.json && empty != nil:
		printJSON(map[string]any{"ok": false, "error": empty, "result": result})
	case *f.json:
		printJSON(map[string]any{"ok": true, "result": result})
	default:
		fmt.Println(formatDeviceResult(req.cmd, result))
		if empty != nil {
			fmt.Fprintf(os.Stderr, "error: %s\nNext: %s\n", empty.Message, empty.Hint)
		}
	}
	if empty != nil {
		return deviceExit{1}
	}
	return nil
}

type deviceReq struct {
	cmd    string
	args   any
	follow bool
}

func parseNum(s, name string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, usageErr("%s must be a number, got %q", name, s)
	}
	return v, nil
}

func absPath(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// deviceRequest maps one daemon-backed command line to its request.
func deviceRequest(cmd string, args []string, f *deviceFlags) (*deviceReq, error) {
	fs := f.fs
	var (
		view, out, treeOut, in, until, grep, level *string
		offscreen, clear, all, ignoreCase, follow  *bool
		maxScrolls, limit, since                   *int
		duration                                   *float64
	)
	switch cmd {
	case "snapshot":
		view = fs.String("view", "", "apply this view's components instead of inferring the view")
		offscreen = fs.Bool("include-offscreen", false, "list off-screen elements too")
		out = fs.String("out", "", "also write the snapshot here, with a .tree.json companion for the offline tools")
		treeOut = fs.String("tree-out", "", "write the ComponentNode tree JSON here")
	case "fill":
		clear = fs.Bool("clear", false, "empty the field first")
	case "scroll":
		in = fs.String("in", "", "scroll inside this element")
		until = fs.String("until", "", "scroll until this target is tappable")
		maxScrolls = fs.Int("max", 20, "give up after this many swipes")
	case "swipe":
		duration = fs.Float64("duration", 0, "swipe duration in seconds")
	case "screenshot":
		out = fs.String("out", "", "PNG file to write")
	case "console":
		limit = fs.Int("limit", 50, "the most recent N entries (0: all)")
		grep = fs.String("grep", "", "case-insensitive substring of the message or subsystem")
		ignoreCase = fs.Bool("i", false, "accepted for familiarity; --grep is always case-insensitive")
		level = fs.String("level", "", "comma-separated levels")
		all = fs.Bool("all", false, "include Apple framework entries")
		since = fs.Int("since", -1, "only entries after this index, oldest first; --limit then takes the first N")
		follow = fs.Bool("follow", false, "keep printing new entries until interrupted")
	}
	p, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return nil, usageErr("%v", err)
	}
	_ = ignoreCase
	need := func(n int, usage string) error {
		if len(p) < n {
			return usageErr("usage: sightmap device %s", usage)
		}
		return nil
	}
	switch cmd {
	case "snapshot":
		return &deviceReq{cmd: "snapshot", args: device.SnapshotArgs{View: *view, IncludeOffscreen: *offscreen, Out: absPath(*out), TreeOut: absPath(*treeOut), JSON: *f.json}}, nil
	case "click":
		if err := need(1, "click TARGET"); err != nil {
			return nil, err
		}
		return &deviceReq{cmd: "click", args: device.TargetArgs{Target: p[0]}}, nil
	case "fill":
		if err := need(2, "fill TARGET TEXT [--clear]"); err != nil {
			return nil, err
		}
		return &deviceReq{cmd: "fill", args: device.TargetArgs{Target: p[0], Text: p[1], Clear: *clear}}, nil
	case "scroll":
		if err := need(1, "scroll up|down|left|right [--in TARGET] [--until TARGET] [--max N]"); err != nil {
			return nil, err
		}
		switch p[0] {
		case "up", "down", "left", "right":
		default:
			return nil, usageErr("scroll direction must be up, down, left or right, not %q", p[0])
		}
		return &deviceReq{cmd: "scroll", args: device.ScrollArgs{Direction: p[0], In: *in, Until: *until, Max: *maxScrolls}}, nil
	case "swipe":
		if err := need(4, "swipe X1 Y1 X2 Y2 [--duration S]"); err != nil {
			return nil, err
		}
		var v [4]float64
		for i, name := range []string{"x1", "y1", "x2", "y2"} {
			if v[i], err = parseNum(p[i], name); err != nil {
				return nil, err
			}
		}
		return &deviceReq{cmd: "swipe", args: device.PointArgs{X1: v[0], Y1: v[1], X2: v[2], Y2: v[3], Duration: *duration}}, nil
	case "tap-xy":
		if err := need(2, "tap-xy X Y  (screen points, not pixels)"); err != nil {
			return nil, err
		}
		x, err := parseNum(p[0], "x")
		if err != nil {
			return nil, err
		}
		y, err := parseNum(p[1], "y")
		if err != nil {
			return nil, err
		}
		return &deviceReq{cmd: "tapXY", args: device.PointArgs{X: x, Y: y}}, nil
	case "type":
		if err := need(1, "type TEXT"); err != nil {
			return nil, err
		}
		return &deviceReq{cmd: "type", args: device.ValueArgs{Text: p[0]}}, nil
	case "key":
		if err := need(1, "key NAME"); err != nil {
			return nil, err
		}
		return &deviceReq{cmd: "key", args: device.ValueArgs{Key: p[0]}}, nil
	case "button":
		if err := need(1, "button NAME"); err != nil {
			return nil, err
		}
		return &deviceReq{cmd: "button", args: device.ValueArgs{Name: p[0]}}, nil
	case "openurl":
		if err := need(1, "openurl URL"); err != nil {
			return nil, err
		}
		return &deviceReq{cmd: "openurl", args: device.ValueArgs{URL: p[0]}}, nil
	case "screenshot":
		if *out == "" {
			return nil, usageErr("usage: sightmap device screenshot --out F.png")
		}
		return &deviceReq{cmd: "screenshot", args: device.ValueArgs{Out: absPath(*out)}}, nil
	case "launch", "relaunch":
		return &deviceReq{cmd: cmd, args: struct{}{}}, nil
	case "console":
		if len(p) > 0 && p[0] == "list" {
			filter := device.ConsoleFilter{Limit: *limit, Grep: *grep, Level: *level, All: *all}
			if *since >= 0 {
				filter.Since = since
			}
			return &deviceReq{cmd: "consoleList", args: filter, follow: *follow}, nil
		}
		if len(p) > 1 && p[0] == "get" {
			i, err := strconv.Atoi(p[1])
			if err != nil {
				return nil, usageErr("console get takes an index, got %q", p[1])
			}
			return &deviceReq{cmd: "consoleGet", args: device.ValueArgs{Index: i}}, nil
		}
		return nil, usageErr("usage: sightmap device console list [--limit N] [--grep S] [--level L] | console get INDEX")
	}
	return nil, usageErr("unknown device command %q", cmd)
}

func consoleLine(e device.ConsoleEntry) string {
	where := ""
	if e.Subsystem != "" {
		where = " " + e.Subsystem
		if e.Category != "" {
			where += ":" + e.Category
		}
	}
	msg, _, _ := strings.Cut(e.Message, "\n")
	return fmt.Sprintf("#%d %-7s%s  %s", e.Index, e.Level, where, msg)
}

// consoleLines renders a console list; following, it leaves out the notes
// about an empty or truncated list.
func consoleLines(l device.ConsoleList, following bool) []string {
	var lines []string
	if l.Dropped > 0 {
		lines = append(lines, fmt.Sprintf("(%d earlier entries dropped; the console keeps the latest %d)", l.Dropped, l.Capacity))
	}
	for _, e := range l.Entries {
		lines = append(lines, consoleLine(e))
	}
	if following {
		return lines
	}
	if len(l.Entries) == 0 {
		lines = append(lines, "(no console entries match)")
	} else if more := l.Total - len(l.Entries); more > 0 {
		lines = append(lines, fmt.Sprintf("(%d more match; raise --limit, or page on with --since %d)", more, l.Last))
	}
	return lines
}

// followConsole prints the entries filter selects, then new ones as they
// arrive, until interrupted; with --json, one entry per line.
func followConsole(session *device.SessionInfo, filter device.ConsoleFilter, asJSON bool) error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	for first := true; ; first = false {
		var l device.ConsoleList
		if err := device.Call(session.Socket, "consoleList", filter, &l, 30*time.Second); err != nil {
			return err
		}
		if asJSON {
			for _, e := range l.Entries {
				printJSON(e)
			}
		} else if lines := consoleLines(l, true); len(lines) > 0 {
			fmt.Println(strings.Join(lines, "\n"))
		}
		last := l.Last
		filter.Since, filter.Limit = &last, 0
		if first && !asJSON {
			fmt.Fprintln(os.Stderr, "(following; Ctrl-C to stop)")
		}
		select {
		case <-sig:
			return nil
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func waited(r map[string]any) string {
	if n, _ := r["retries"].(float64); n > 0 {
		return fmt.Sprintf(" after %d re-read(s) while the screen settled", int(n))
	}
	return ""
}

// formatDeviceResult renders a daemon result for people, in the shape of
// `sightmap browser` output.
func formatDeviceResult(cmd string, raw json.RawMessage) string {
	var r map[string]any
	json.Unmarshal(raw, &r)
	str := func(k string) string { s, _ := r[k].(string); return s }
	num := func(k string) string { return strconv.FormatFloat(asFloat(r[k]), 'f', -1, 64) }
	switch cmd {
	case "snapshot":
		return str("text")
	case "click":
		return fmt.Sprintf("✓ clicked %s at (%s, %s)%s", str("target"), num("x"), num("y"), waited(r))
	case "fill":
		line := fmt.Sprintf("✓ filled %s → value %q", str("target"), str("value"))
		if p := str("placeholder"); p != "" {
			line += fmt.Sprintf(" (empty; the field shows its placeholder %q)", p)
		}
		return line + waited(r)
	case "scroll":
		if found := str("found"); found != "" {
			return fmt.Sprintf("✓ %s is on screen after %s scroll(s) %s; snapshot ids refreshed, act on it by id %s or by the same target", found, num("scrolls"), str("direction"), str("id"))
		}
		return fmt.Sprintf("✓ scrolled %s; take a new snapshot", str("direction"))
	case "swipe":
		return fmt.Sprintf("✓ swiped (%s, %s) → (%s, %s)", num("x1"), num("y1"), num("x2"), num("y2"))
	case "tapXY":
		return fmt.Sprintf("✓ tapped (%s, %s)", num("x"), num("y"))
	case "type":
		return fmt.Sprintf("✓ typed %q", str("text"))
	case "key":
		return "✓ pressed " + str("key")
	case "button":
		return "✓ pressed " + str("name")
	case "openurl":
		return "✓ opened " + str("url")
	case "screenshot":
		return "✓ wrote " + str("out")
	case "launch":
		return fmt.Sprintf("✓ launched %s (pid %s)", str("bundleId"), num("pid"))
	case "relaunch", "restart":
		what := "relaunched"
		if installed := str("installed"); installed != "" {
			what = "installed " + installed + " and relaunched"
		}
		line := fmt.Sprintf("✓ %s %s (pid %s)", what, str("bundleId"), num("pid"))
		if cmd == "restart" {
			line += " in the running session"
		}
		if ready, ok := r["ready"].(bool); ok && !ready {
			line += "; accessibility tree not readable yet, snapshot again shortly"
		}
		return line
	case "runNew", "runCurrent":
		if str("run") == "" {
			return "(no run yet)"
		}
		return str("run") + "\n" + str("path")
	case "consoleList":
		var l device.ConsoleList
		json.Unmarshal(raw, &l)
		return strings.Join(consoleLines(l, false), "\n")
	case "consoleGet":
		var e device.ConsoleEntry
		json.Unmarshal(raw, &e)
		lines := []string{fmt.Sprintf("#%d %s", e.Index, e.Level), "time:      " + e.Time, "source:    " + e.Source}
		if e.Subsystem != "" {
			sub := "subsystem: " + e.Subsystem
			if e.Category != "" {
				sub += " (" + e.Category + ")"
			}
			lines = append(lines, sub)
		}
		return strings.Join(append(lines, "", e.Message), "\n")
	}
	out, _ := json.MarshalIndent(r, "", "  ")
	return string(out)
}

func asFloat(v any) float64 {
	f, _ := v.(float64)
	return f
}

func deviceDoctor(args []string, f *deviceFlags) error {
	fix := f.fs.Bool("fix", false, "run the unprivileged fixes, then check again")
	if _, err := parseFlagsAroundArgs(f.fs, args); err != nil {
		return usageErr("%v", err)
	}
	var progress io.Writer = os.Stderr
	if *f.json {
		progress = io.Discard
	}
	checks := device.Doctor(context.Background(), *fix, progress)
	failed := 0
	for _, c := range checks {
		if !c.OK {
			failed++
		}
	}
	if *f.json {
		printJSON(map[string]any{"ok": failed == 0, "result": checks})
	} else {
		width := 0
		for _, c := range checks {
			width = max(width, len(c.Name))
		}
		for _, c := range checks {
			icon := "ok  "
			if !c.OK {
				icon = "FAIL"
			}
			fmt.Printf("[%s] %-*s  %s\n", icon, width, c.Name, c.Detail)
			if c.Fix != "" {
				fmt.Printf("       %s  fix: %s\n", strings.Repeat(" ", width), c.Fix)
			}
		}
		if failed > 0 {
			fmt.Printf("\n%d check(s) failed.\n", failed)
		} else {
			fmt.Println("\nAll checks passed.")
		}
	}
	if failed > 0 {
		return deviceExit{1}
	}
	return nil
}

// daemonConfig is what `start` hands the detached daemon.
type daemonConfig struct {
	device.Config
	Helper  string `json:"helper"`
	LogFile string `json:"logFile"`
}

func deviceStart(args []string, f *deviceFlags) error {
	fs := f.fs
	deviceName := fs.String("device", "", "simulator name or UDID (default: a booted one, else the newest iPhone)")
	appFlag := fs.String("app", "", "the .app bundle built for the simulator, to install")
	bundleID := fs.String("bundle-id", "", "an installed app's bundle id")
	label := fs.String("label", "", "a label for the run log")
	detach := fs.Bool("detach", false, "run the daemon in the background and return once it is ready")
	consoleLimit := fs.Int("console-limit", 5000, "how many console entries the session keeps")
	if _, err := parseFlagsAroundArgs(fs, args); err != nil {
		return usageErr("%v", err)
	}
	ctx := context.Background()
	sightmapDir := absPath(*f.sightmapDir)
	appPath := absPath(*appFlag)
	if appPath != "" {
		if _, err := os.Stat(appPath); err != nil {
			return &device.Error{Code: "bad_app", Message: appPath + " does not exist", Hint: "build the app for the simulator and pass the .app path"}
		}
	}
	progress := func(s string) {
		if !*f.json {
			fmt.Fprintln(os.Stderr, s)
		}
	}

	if existing := device.LiveSession(sightmapDir); existing != nil {
		if *deviceName != "" && *deviceName != existing.UDID && !strings.HasPrefix(existing.DeviceName, *deviceName+",") {
			return &device.Error{Code: "session_running", Message: fmt.Sprintf("a session is already running on %s (%s)", existing.DeviceName, existing.UDID), Hint: "run `sightmap device stop` first"}
		}
		wanted := *bundleID
		if wanted == "" && appPath != "" {
			var err error
			if wanted, err = device.PlistValue(appPath, "CFBundleIdentifier"); err != nil {
				return err
			}
		}
		if wanted != "" && wanted != existing.BundleID {
			again := "--bundle-id " + wanted
			if appPath != "" {
				again = "--app " + strconv.Quote(appPath)
			}
			return &device.Error{Code: "session_running",
				Message: fmt.Sprintf("a session for %s is already running on %s; it can't switch to %s", existing.BundleID, existing.DeviceName, wanted),
				Hint:    fmt.Sprintf("run `sightmap device stop`, then `sightmap device start %s --detach`", again)}
		}
		var result json.RawMessage
		if err := device.Call(existing.Socket, "restart", device.ValueArgs{App: appPath}, &result, 5*time.Minute); err != nil {
			return err
		}
		if *f.json {
			printJSON(map[string]any{"ok": true, "result": result})
		} else {
			fmt.Println(formatDeviceResult("restart", result))
		}
		return nil
	}

	started := time.Now()
	helper, err := device.BuildHelper(ctx, writerFunc(progress))
	if err != nil {
		return err
	}
	devices, err := device.ListDevices(ctx)
	if err != nil {
		return err
	}
	d, err := device.PickDevice(devices, *deviceName)
	if err != nil {
		return err
	}
	if err := device.PrepareDevice(ctx, d, progress); err != nil {
		return err
	}
	if *bundleID == "" && appPath == "" {
		return usageErr("start needs --app PATH or --bundle-id ID")
	}
	cfg := device.Config{UDID: d.UDID, DeviceName: d.Label(), BundleID: *bundleID, AppPath: appPath, SightmapDir: sightmapDir, RunLabel: *label, ConsoleLimit: *consoleLimit,
		RunDir: filepath.Join(device.CacheDir(), "sessions", d.UDID)}
	if appPath != "" {
		if cfg.BundleID == "" {
			if cfg.BundleID, err = device.PlistValue(appPath, "CFBundleIdentifier"); err != nil {
				return err
			}
		}
		progress("installing " + appPath + "…")
		if err := (device.Simctl{}).Install(ctx, d.UDID, appPath); err != nil {
			return err
		}
	}
	installed := appPath
	if installed == "" {
		if installed, err = device.AppContainer(ctx, d.UDID, cfg.BundleID); err != nil {
			return err
		}
	}
	if cfg.Process, err = device.PlistValue(installed, "CFBundleExecutable"); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.RunDir, 0o755); err != nil {
		return err
	}
	dc := daemonConfig{Config: cfg, Helper: helper, LogFile: filepath.Join(cfg.RunDir, "daemon.log")}

	var info *device.SessionInfo
	var foreground *device.Daemon
	if *detach {
		if info, err = spawnDeviceDaemon(dc, sightmapDir); err != nil {
			return err
		}
	} else {
		if foreground, err = serveDevice(dc, os.Stderr); err != nil {
			return err
		}
		info = &foreground.Info
	}
	var ready map[string]any
	if err := device.Call(info.Socket, "ready", nil, &ready, 2*time.Minute); err != nil {
		ready = map[string]any{"ready": false, "error": device.AsError(err)}
	}
	if *f.json {
		printJSON(map[string]any{"ok": true, "result": map[string]any{"session": info, "sessionFile": info.File, "ready": ready, "seconds": time.Since(started).Seconds()}})
	} else {
		fmt.Println(formatDeviceStart(info, ready, time.Since(started)))
	}
	if foreground != nil {
		if !*f.json {
			fmt.Fprintln(os.Stderr, "daemon running in the foreground; Ctrl-C or `sightmap device stop` ends it (use --detach in scripts)")
		}
		stopOnSignal(foreground)
		foreground.Wait()
	}
	return nil
}

type writerFunc func(string)

func (w writerFunc) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		w(line)
	}
	return len(p), nil
}

func formatDeviceStart(info *device.SessionInfo, ready map[string]any, took time.Duration) string {
	app := fmt.Sprintf("launched %s (already installed)", info.BundleID)
	if info.AppPath != "" {
		app = fmt.Sprintf("installed %s and launched %s", info.AppPath, info.BundleID)
	}
	file := info.File
	if rel, err := filepath.Rel(cwd(), file); err == nil && !strings.HasPrefix(rel, "..") {
		file = rel
	}
	lines := []string{
		fmt.Sprintf("✓ simulator session started in %.1f s", took.Seconds()),
		fmt.Sprintf("  device:  %s (%s)", info.DeviceName, info.UDID),
		"  app:     " + app,
		fmt.Sprintf("  daemon:  pid %d; session file %s; log %s", info.PID, file, info.LogFile),
		fmt.Sprintf("  run:     %s (`sightmap device run tail --follow` to follow)", info.Run),
	}
	if e, ok := ready["error"].(map[string]any); ok {
		lines = append(lines, fmt.Sprintf("  ⚠ accessibility tree not readable yet: %v. Next: %v", e["message"], e["hint"]))
	} else if e, ok := ready["error"].(*device.Error); ok {
		lines = append(lines, fmt.Sprintf("  ⚠ accessibility tree not readable yet: %s. Next: %s", e.Message, e.Hint))
	} else if r, _ := ready["ready"].(bool); !r {
		lines = append(lines, "  ⚠ the app shows no interactive elements yet; snapshot again shortly")
	}
	return strings.Join(append(lines, "  next:    sightmap device snapshot"), "\n")
}

// spawnDeviceDaemon starts the daemon in its own session (so it survives a
// one-shot agent shell) and waits until it is serving.
func spawnDeviceDaemon(dc daemonConfig, sightmapDir string) (*device.SessionInfo, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	logf, err := os.OpenFile(dc.LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	defer logf.Close()
	config, _ := json.Marshal(dc)
	cmd := exec.Command(self, "device", "_daemon", string(config))
	cmd.Stdout, cmd.Stderr = logf, logf
	configureDetachedDaemon(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.After(90 * time.Second)
	for {
		select {
		case err := <-done:
			return nil, &device.Error{Code: "daemon_failed", Message: fmt.Sprintf("the daemon exited during startup (%v):\n%s", err, tailFile(dc.LogFile, 2000)), Hint: "see " + dc.LogFile}
		case <-deadline:
			return nil, &device.Error{Code: "daemon_timeout", Message: "the daemon did not start within 90 s", Hint: "see " + dc.LogFile}
		case <-time.After(150 * time.Millisecond):
		}
		if info := device.ReadSession(sightmapDir); info != nil && info.PID == cmd.Process.Pid {
			return info, nil
		}
	}
}

// serveDevice runs the daemon in this process, logging to log.
func serveDevice(dc daemonConfig, log io.Writer) (*device.Daemon, error) {
	logf := func(s string) { fmt.Fprintf(log, "[%s] %s\n", time.Now().UTC().Format(time.RFC3339), s) }
	s := device.NewSession(dc.Config, device.NewHelperClient(dc.Helper, logf), device.Simctl{})
	s.Log = logf
	d, err := device.Serve(s, true, dc.LogFile)
	if err != nil {
		s.Close()
		return nil, err
	}
	return d, nil
}

func stopOnSignal(d *device.Daemon) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		d.Stop(false)
	}()
}

func runDeviceDaemon(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: sightmap device _daemon CONFIG-JSON (started by `device start --detach`)")
	}
	var dc daemonConfig
	if err := json.Unmarshal([]byte(args[0]), &dc); err != nil {
		return err
	}
	d, err := serveDevice(dc, os.Stderr)
	if err != nil {
		return err
	}
	stopOnSignal(d)
	d.Wait()
	return nil
}

func deviceStatus(args []string, f *deviceFlags) error {
	if _, err := parseFlagsAroundArgs(f.fs, args); err != nil {
		return usageErr("%v", err)
	}
	session := device.LiveSession(*f.sightmapDir)
	if session == nil {
		if *f.json {
			printJSON(map[string]any{"ok": true, "result": map[string]bool{"running": false}})
		} else {
			fmt.Println("✗ no running session. Next: run `sightmap device start --detach`")
		}
		return deviceExit{1}
	}
	var s device.Status
	if err := device.Call(session.Socket, "status", nil, &s, 10*time.Second); err != nil {
		return err
	}
	if *f.json {
		printJSON(map[string]any{"ok": true, "result": map[string]any{"running": true, "status": s}})
		return nil
	}
	helper := "idle"
	if s.Helper.Alive {
		helper = "running"
	}
	if s.Helper.Restarts > 0 {
		helper += fmt.Sprintf(", restarted %dx", s.Helper.Restarts)
	}
	logs := ""
	if !s.Console.Streaming {
		logs = " (log stream not running)"
	}
	app := s.BundleID
	if s.AppPID > 0 {
		app += fmt.Sprintf(" (pid %d)", s.AppPID)
	}
	if s.AppPath != "" {
		app += "  " + s.AppPath
	}
	fmt.Printf("✓ session running (daemon pid %d, since %s)\n  device:  %s (%s)\n  app:     %s\n  helper:  %s\n  console: %d entries%s\n  session: %s\n  run:     %s (%d entries)\n",
		s.PID, s.StartedAt, s.DeviceName, s.UDID, app, helper, s.Console.Entries, logs, session.File, s.Run.Run, s.Run.Seq)
	return nil
}

func deviceStop(args []string, f *deviceFlags) error {
	shutdown := f.fs.Bool("shutdown", false, "shut the simulator down too")
	if _, err := parseFlagsAroundArgs(f.fs, args); err != nil {
		return usageErr("%v", err)
	}
	session := device.ReadSession(*f.sightmapDir)
	if session == nil || device.LiveSession(*f.sightmapDir) == nil {
		if *f.json {
			printJSON(map[string]any{"ok": true, "result": map[string]bool{"stopped": false}})
		} else {
			fmt.Println("(no running session)")
		}
		return nil
	}
	if err := device.Call(session.Socket, "stop", device.StopArgs{Shutdown: *shutdown}, nil, time.Minute); err != nil {
		return err
	}
	for i := 0; i < 100 && device.ReadSession(*f.sightmapDir) != nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if *f.json {
		printJSON(map[string]any{"ok": true, "result": map[string]bool{"stopped": true, "shutdown": *shutdown}})
	} else if *shutdown {
		fmt.Println("✓ stopped; simulator shut down")
	} else {
		fmt.Println("✓ stopped")
	}
	return nil
}

func formatRunEntry(e device.RunEntry) string {
	if e.Action == "note" {
		kind := ""
		if e.Kind != "" {
			kind = " [" + e.Kind + "]"
		}
		return fmt.Sprintf("%d %s note%s %s", e.Seq, e.TS, kind, e.Text)
	}
	mark := "✓"
	if !e.OK {
		mark = "✗"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s %s %s", e.Seq, e.TS, mark, e.Action)
	keys := make([]string, 0, len(e.Args))
	for k := range e.Args {
		if k != "target" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, _ := json.Marshal(e.Args[k])
		fmt.Fprintf(&b, " %s=%s", k, v)
	}
	if t := e.Target; t != nil {
		what := t.Role
		if t.Component != "" {
			what = "[" + t.Component + "]"
		}
		fmt.Fprintf(&b, " → %s %s", t.ID, what)
		if t.Identifier != "" {
			b.WriteString(" " + t.Identifier)
		}
		if t.Text != "" {
			b.WriteString(" " + strconv.Quote(t.Text))
		}
	}
	if e.View != "" {
		fmt.Fprintf(&b, " (view %s)", e.View)
	}
	if e.Error != nil {
		first, _, _ := strings.Cut(e.Error.Message, "\n")
		b.WriteString(" error: " + first)
	}
	return b.String()
}

func deviceRun(args []string, f *deviceFlags) error {
	fs := f.fs
	label := fs.String("label", "", "a label for the new run")
	kind := fs.String("kind", "", "what a note is, e.g. session-start")
	follow := fs.Bool("follow", false, "keep printing new entries")
	since := fs.Int("since", 0, "only entries after this sequence number")
	runID := fs.String("run", "", "a run id (default: the session's current run, else the newest)")
	p, err := parseFlagsAroundArgs(fs, args)
	if err != nil {
		return usageErr("%v", err)
	}
	sub := ""
	if len(p) > 0 {
		sub = p[0]
	}
	switch sub {
	case "note":
		if len(p) < 2 {
			return usageErr("usage: sightmap device run note TEXT [--kind K]")
		}
		session, err := device.RequireSession(*f.sightmapDir)
		if err != nil {
			return err
		}
		if err := device.Call(session.Socket, "runNote", device.ValueArgs{Text: p[1], Kind: *kind}, nil, 10*time.Second); err != nil {
			return err
		}
		if *f.json {
			printJSON(map[string]any{"ok": true})
		} else {
			fmt.Println("✓ noted")
		}
		return nil
	case "new", "current":
		session, err := device.RequireSession(*f.sightmapDir)
		if err != nil {
			return err
		}
		cmd := map[string]string{"new": "runNew", "current": "runCurrent"}[sub]
		var result json.RawMessage
		if err := device.Call(session.Socket, cmd, device.ValueArgs{Label: *label}, &result, 10*time.Second); err != nil {
			return err
		}
		if *f.json {
			printJSON(map[string]any{"ok": true, "result": result})
		} else {
			fmt.Println(formatDeviceResult(cmd, result))
		}
		return nil
	case "tail":
	default:
		return usageErr("usage: sightmap device run new [--label L] | run current | run note TEXT [--kind K] | run tail [--follow] [--since SEQ] [--run ID]")
	}
	path := ""
	switch {
	case *runID != "":
		path = filepath.Join(*f.sightmapDir, device.RunsDir, strings.TrimSuffix(*runID, ".jsonl")+".jsonl")
	default:
		if session := device.LiveSession(*f.sightmapDir); session != nil {
			var cur device.RunInfo
			if device.Call(session.Socket, "runCurrent", nil, &cur, 10*time.Second) == nil {
				path = cur.Path
			}
		}
		if path == "" {
			path = device.LatestRun(*f.sightmapDir)
		}
	}
	if path == "" {
		return &device.Error{Code: "not_found", Message: "no run log found", Hint: "run `sightmap device start --detach` (or `sightmap device run new`) first"}
	}
	last := *since
	emit := func() error {
		entries, err := device.ReadRun(path, last)
		if err != nil {
			return &device.Error{Code: "not_found", Message: err.Error()}
		}
		for _, e := range entries {
			if *f.json {
				printJSON(e)
			} else {
				fmt.Println(formatRunEntry(e))
			}
			last = e.Seq
		}
		return nil
	}
	if err := emit(); err != nil || !*follow {
		return err
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-sig:
			return nil
		case <-ticker.C:
			if err := emit(); err != nil {
				return err
			}
		}
	}
}
