// browser explore drives the live tab toward a goal, one typed model question
// per step, over the annotated component tree. It is the CLI adapter for the
// explore package: flags in, progress on stderr, the run (or the bench table)
// on stdout.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sightmap/sightmap/go/browser"
	"github.com/sightmap/sightmap/go/explore"
)

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func runExplore(args []string) error {
	fs := flag.NewFlagSet("explore", flag.ContinueOnError)
	lf := addLiveFlags(fs, "explore")
	goalFlag := fs.String("goal", "", "What to achieve, in plain words")
	specFlag := fs.String("spec", "", "JSON spec file: done_when, values, hint, avoid")
	var doneWhen, values, avoid stringList
	fs.Var(&doneWhen, "done-when", "Deterministic finish check, repeatable: view=NAME | url=SUBSTR | text=SUBSTR | component=NAME | history=SUBSTR | prop=Comp.name~value[@Within.name~value]")
	fs.Var(&values, "value", "A value the loop may type, as key=text (repeatable). The loop never invents text.")
	fs.Var(&avoid, "avoid", "Drop controls whose name contains this (repeatable), e.g. Delete, Pay")
	pickerFlag := fs.String("picker", "jev", "Who picks each step: jev[:model] (TYPESAFE_API_KEY) or anthropic[:model] (ANTHROPIC_API_KEY)")
	planFlag := fs.Bool("plan", false, "Ask Anthropic once to write the spec from the goal (ANTHROPIC_API_KEY)")
	growFlag := fs.Bool("grow", false, "Grow the corpus while exploring: name unmapped controls on every page visited")
	maxStepsFlag := fs.Int("max-steps", 20, "Stop after this many steps")
	jsonFlag := fs.Bool("json", false, "Print the run as JSON on stdout instead of a summary line")
	benchFlag := fs.String("bench", "", "Run a suite JSON file (see go/explore/bench/) instead of one goal")
	repeatFlag := fs.Int("repeat", 1, "With --bench: run the suite this many times")
	onlyFlag := fs.String("only", "", "With --bench: only goals whose name contains this")
	outFlag := fs.String("out", "", "With --bench: write the full result JSON to this file")
	if err := parseFlagsInterspersed(fs, args); err != nil {
		return err
	}
	if *goalFlag == "" && fs.NArg() > 0 {
		*goalFlag = strings.Join(fs.Args(), " ")
	}
	if *goalFlag == "" && *benchFlag == "" {
		return fmt.Errorf("usage: browser explore --goal \"...\" [--done-when view=Cart] [--value user=alice] [--picker jev|anthropic] [--grow] | --bench suite.json")
	}

	var suite *explore.Suite
	if *benchFlag != "" {
		s, err := explore.LoadSuite(*benchFlag)
		if err != nil {
			return err
		}
		suite = s
		// A suite may name its corpus relative to its own file; an explicit --sightmap-dir wins.
		explicitDir := false
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "sightmap-dir" {
				explicitDir = true
			}
		})
		if !explicitDir && suite.SightmapDir != "" {
			*lf.sightmapDir = filepath.Join(filepath.Dir(*benchFlag), suite.SightmapDir)
		}
	}

	ctx := context.Background()
	conn, cleanup, err := lf.connect(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	_ = browser.BringToFront(ctx, conn)

	if err := lf.navigate(ctx, conn, navOpts{idle: true}); err != nil {
		return err
	}

	corpus, cErr := lf.loadCorpus()
	if cErr != nil {
		return fmt.Errorf("explore: load corpus: %w", cErr)
	}
	if corpus == nil {
		fmt.Fprintf(os.Stderr, "explore: no .sightmap corpus at %q — driving the raw tree; pass --grow to build one as you go\n", *lf.sightmapDir)
	}
	drv := explore.NewCDPDriver(conn, corpus)

	newPicker := func() (explore.Picker, error) { return makePicker(*pickerFlag) }

	var hook explore.PageHook
	if *growFlag {
		if newGrowHook == nil {
			return fmt.Errorf("--grow is not available in this build")
		}
		h, report, err := newGrowHook(*lf.sightmapDir, drv)
		if err != nil {
			return err
		}
		hook = h
		defer func() { fmt.Fprintln(os.Stderr, report()) }()
	}

	if suite != nil {
		return runExploreBench(ctx, drv, suite, explore.SuiteOptions{NewPicker: newPicker, Repeat: *repeatFlag, Only: *onlyFlag, MaxSteps: maxStepsIfSet(fs, *maxStepsFlag), Hook: hook, Out: os.Stderr}, *outFlag)
	}

	spec, err := buildSpec(ctx, *specFlag, doneWhen, values, avoid, *planFlag, *goalFlag, *lf.url)
	if err != nil {
		return err
	}
	picker, err := newPicker()
	if err != nil {
		return err
	}
	run, err := explore.Explore(ctx, drv, explore.Options{
		Goal: *goalFlag, Spec: spec, Picker: picker, MaxSteps: *maxStepsFlag, Hook: hook,
		OnStep: func(s explore.Step) { fmt.Fprintln(os.Stderr, explore.FormatStep(s)) },
	})
	if err != nil {
		if run != nil && *jsonFlag {
			printRunJSON(os.Stdout, run)
		}
		return err
	}
	if *jsonFlag {
		printRunJSON(os.Stdout, run)
	} else {
		status := "FAIL"
		if run.OK {
			status = "OK"
		}
		st := run.Stats
		fmt.Printf("%s  %s  steps=%d  %.1fs  picker=%s calls=%d %dms tokens=%d+%d", status, run.Reason, len(run.Steps), float64(run.Ms)/1000, run.Picker, st.Calls, st.Ms, st.InputTokens, st.OutputTokens)
		if st.USD > 0 {
			fmt.Printf(" $%.3f", st.USD)
		}
		fmt.Println()
	}
	if !run.OK {
		return errExploreNotDone
	}
	return nil
}

// errExploreNotDone makes a goal that was not reached exit non-zero without
// re-printing a reason (the summary line already said why).
var errExploreNotDone = fmt.Errorf("goal not reached")

func maxStepsIfSet(fs *flag.FlagSet, v int) int {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "max-steps" {
			set = true
		}
	})
	if set {
		return v
	}
	return 0
}

func printRunJSON(w io.Writer, run *explore.Run) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	_ = enc.Encode(run)
}

func buildSpec(ctx context.Context, specPath string, doneWhen, values, avoid []string, plan bool, goal, site string) (*explore.Spec, error) {
	spec := &explore.Spec{Values: map[string]string{}}
	if specPath != "" {
		s, err := explore.LoadSpec(specPath)
		if err != nil {
			return nil, err
		}
		spec = s
		if spec.Values == nil {
			spec.Values = map[string]string{}
		}
	} else if plan {
		client, err := explore.NewAnthropicClient("")
		if err != nil {
			return nil, fmt.Errorf("--plan: %w", err)
		}
		host := ""
		if u, err := url.Parse(site); err == nil {
			host = u.Host
		}
		s, ms, err := client.Plan(ctx, goal, host)
		if err != nil {
			return nil, fmt.Errorf("--plan: %w", err)
		}
		b, _ := json.Marshal(s)
		fmt.Fprintf(os.Stderr, "plan (%d ms): %s\n", ms, b)
		spec = s
	}
	if len(doneWhen) > 0 {
		d, err := explore.ParseDoneWhen(doneWhen)
		if err != nil {
			return nil, err
		}
		spec.DoneWhen = d
	}
	for _, kv := range values {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("--value %q: expected key=text", kv)
		}
		spec.Values[k] = v
	}
	spec.Avoid = append(spec.Avoid, avoid...)
	return spec, nil
}

func makePicker(name string) (explore.Picker, error) {
	kind, model, _ := strings.Cut(name, ":")
	switch kind {
	case "jev", "":
		return explore.NewJevPicker(model)
	case "anthropic", "claude":
		return explore.NewAnthropicClient(model)
	}
	return nil, fmt.Errorf("--picker %q: use jev[:model] or anthropic[:model]", name)
}

// newGrowHook builds the --grow page hook and a closure that reports what it
// wrote. It is set by the grow wiring (cmd_browser_explore_grow.go); nil means
// the flag is unavailable.
var newGrowHook func(dir string, drv *explore.CDPDriver) (hook explore.PageHook, report func() string, err error)

func runExploreBench(ctx context.Context, drv *explore.CDPDriver, suite *explore.Suite, opts explore.SuiteOptions, outPath string) error {
	res, err := explore.RunSuite(ctx, drv, suite, opts)
	if err != nil {
		return err
	}
	fmt.Print("\n" + explore.FormatTable(res))
	if outPath == "" {
		outPath = fmt.Sprintf("explore-%s-%s-%s.json", suite.Name, strings.NewReplacer(":", "_", "/", "_").Replace(res.Picker), time.Now().Format("20060102-150405"))
	}
	data, _ := json.MarshalIndent(res, "", " ")
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "saved %s\n", outPath)
	if res.Summary.OK < res.Summary.Goals {
		return fmt.Errorf("%d of %d goals not reached", res.Summary.Goals-res.Summary.OK, res.Summary.Goals)
	}
	return nil
}
