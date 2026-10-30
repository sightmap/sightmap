package device

import (
	"bufio"
	"context"
	"debug/macho"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Network modes for `device start --network`.
const (
	NetworkAuto = "auto"
	NetworkOff  = "off"
)

// NetworkModes are the values `device start --network` takes.
var NetworkModes = []string{NetworkAuto, SourceHAR, SourceRNCDP, SourceSummary, NetworkOff}

// A source's state in NetStatus.
const (
	stateActive      = "active"
	stateStandby     = "standby"
	stateUnavailable = "unavailable"
	stateOff         = "off"
)

// HAR capture is switched on inside the simulator through private CFNetwork
// preferences, the same ones Instruments' HTTP Traffic instrument writes. The
// capture expires on its own, so a daemon that dies leaves it on for at most
// harLease.
const (
	harLease        = 10 * time.Minute
	harRenew        = 2 * time.Minute
	harBodyLimit    = 1 << 20
	harNotification = "com.apple.CFNetwork.har-capture-update"
)

// summaryHold is how long a Summary line waits for the HAR entry of the same
// task before it is shown on its own.
var summaryHold = 2 * time.Second

// NetSourceStatus is one source's state: active, standby (waiting for
// something, such as Metro), unavailable or off, and why.
type NetSourceStatus struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Detail  string `json:"detail"`
	Entries int    `json:"entries"`
}

// NetStatus is the network part of the status command's result.
type NetStatus struct {
	Mode    string            `json:"mode"`
	Entries int               `json:"entries"`
	Sources []NetSourceStatus `json:"sources"`
}

// NetCollector fills a Network from the sources its mode allows: CFNetwork's
// HAR log, Metro's CDP proxy for a React Native Debug build, and CFNetwork's
// Summary log.
type NetCollector struct {
	UDID, BundleID, Process string
	// AppPath is the installed .app, read for the get-task-allow entitlement.
	AppPath   string
	Mode      string
	MetroPort int
	Network   *Network
	Log       func(string)

	mu          sync.Mutex
	cancel      context.CancelFunc
	streams     sync.WaitGroup
	states      map[string]*NetSourceStatus
	harArmed    bool
	diagnostics bool
	harTasks    map[string]bool
	unclaimed   int
	diag        diagQueue
	// summaryDetail is the summary source's detail while it is active.
	summaryDetail string
}

// ValidNetworkMode reports whether mode is one `--network` accepts.
func ValidNetworkMode(mode string) bool {
	for _, m := range NetworkModes {
		if m == mode {
			return true
		}
	}
	return false
}

func (c *NetCollector) setState(name, state, detail string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.states[name]
	st.State, st.Detail = state, detail
	if name == SourceRNCDP && c.Mode == NetworkAuto && c.summaryDetail != "" {
		summary := c.states[SourceSummary]
		if state == stateActive {
			summary.State, summary.Detail = stateStandby, "paused while rn-cdp is attached: it would list the same JS requests with less detail"
		} else {
			summary.State, summary.Detail = stateActive, c.summaryDetail
		}
	}
}

// Start arms the sources. It must run before the app launches: HAR capture
// misses requests already under way, and the summary source's host names need
// LaunchEnv on the launch.
func (c *NetCollector) Start(ctx context.Context) {
	c.states = map[string]*NetSourceStatus{}
	for _, name := range []string{SourceHAR, SourceRNCDP, SourceSummary} {
		c.states[name] = &NetSourceStatus{Name: name, State: stateOff, Detail: "not selected (--network " + c.Mode + ")"}
	}
	c.harTasks = map[string]bool{}
	if c.Mode == NetworkOff {
		return
	}
	ctx, c.cancel = context.WithCancel(ctx)
	entitled, why := c.entitled(ctx)
	auto := c.Mode == NetworkAuto
	wantHAR := c.Mode == SourceHAR || auto && entitled
	wantCDP := c.Mode == SourceRNCDP || auto && !entitled
	wantSummary := c.Mode == SourceSummary || auto
	c.diagnostics = c.Mode == SourceSummary || auto

	switch {
	case wantHAR:
		if err := c.armHAR(ctx); err != nil {
			c.setState(SourceHAR, stateUnavailable, "cannot switch on CFNetwork HAR capture: "+err.Error())
			wantHAR = false
		} else {
			c.harArmed = true
			detail := "capturing"
			if !entitled {
				detail = "armed, but " + why + ", so CFNetwork will likely log nothing for it"
			}
			c.setState(SourceHAR, stateActive, detail)
			go c.renewHAR(ctx)
		}
	case auto:
		c.setState(SourceHAR, stateUnavailable, why+": CFNetwork logs HAR only for a process with that entitlement (Xcode Debug builds signed for development have it)")
	}
	if wantSummary {
		c.summaryDetail = "status, timing and host only: CFNetwork redacts paths, headers and bodies in this log"
		if c.harArmed {
			c.summaryDetail = "backstop: lists requests HAR did not log (such as ones made before capture began), with method, host, status and timing"
		}
		c.setState(SourceSummary, stateActive, c.summaryDetail)
	}
	if wantHAR || wantSummary {
		c.streams.Add(1)
		go func() {
			defer c.streams.Done()
			c.stream(ctx, wantHAR, wantSummary)
		}()
	}
	switch {
	case wantCDP:
		port := c.metroPort(ctx)
		c.setState(SourceRNCDP, stateStandby, fmt.Sprintf("looking for Metro on port %d", port))
		src := &cdpSource{port: port, bundleID: c.BundleID, push: c.push, log: c.Log,
			setState: func(state, detail string) { c.setState(SourceRNCDP, state, detail) }}
		go src.run(ctx)
	case auto:
		c.setState(SourceRNCDP, stateOff, "not needed: HAR logs a React Native app's JS fetch/XHR traffic too")
	}
}

// LaunchEnv is the environment the app must launch with.
func (c *NetCollector) LaunchEnv() []string {
	if c == nil || !c.diagnostics {
		return nil
	}
	return []string{"CFNETWORK_DIAGNOSTICS=1"}
}

func (c *NetCollector) push(e NetworkEntry) { c.Network.Push(e) }

var getTaskAllow = regexp.MustCompile(`<key>(?:com\.apple\.security\.)?get-task-allow</key>\s*<true\s*/>`)

// entitled reports whether the app's executable carries get-task-allow, in
// the simulated entitlements Xcode embeds for a simulator build or in its
// code signature.
func (c *NetCollector) entitled(ctx context.Context) (bool, string) {
	if c.AppPath == "" || c.Process == "" {
		return false, "the app's bundle was not found to check for the get-task-allow entitlement"
	}
	if getTaskAllow.Match(entitlementsOf(ctx, filepath.Join(c.AppPath, c.Process))) {
		return true, ""
	}
	return false, c.Process + " lacks the get-task-allow entitlement"
}

// entitlementsOf returns exe's entitlements: the simulated ones Xcode embeds
// for a simulator build, else those in its code signature.
var entitlementsOf = func(ctx context.Context, exe string) []byte {
	if data := simulatedEntitlements(exe); data != nil {
		return data
	}
	out, _ := exec.CommandContext(ctx, "codesign", "-d", "--entitlements", "-", "--xml", exe).Output()
	return out
}

func simulatedEntitlements(exe string) []byte {
	files := []*macho.File{}
	if f, err := macho.Open(exe); err == nil {
		defer f.Close()
		files = append(files, f)
	} else if fat, err := macho.OpenFat(exe); err == nil {
		defer fat.Close()
		for _, a := range fat.Arches {
			files = append(files, a.File)
		}
	}
	for _, f := range files {
		if s := f.Section("__entitlements"); s != nil && s.Seg == "__TEXT" {
			if data, err := s.Data(); err == nil {
				return data
			}
		}
	}
	return nil
}

// metroPort is MetroPort, else the port the app's RCT_jsLocation default
// names, else Metro's default 8081.
func (c *NetCollector) metroPort(ctx context.Context) int {
	if c.MetroPort > 0 {
		return c.MetroPort
	}
	out, err := xcrun(ctx, 15*time.Second, nil, "simctl", "spawn", c.UDID, "defaults", "read", c.BundleID, "RCT_jsLocation")
	if err == nil {
		if _, port, ok := strings.Cut(strings.TrimSpace(out), ":"); ok {
			if n, err := strconv.Atoi(port); err == nil {
				return n
			}
		}
	}
	return 8081
}

func (c *NetCollector) spawn(ctx context.Context, args ...string) error {
	return simSpawn(ctx, c.UDID, args...)
}

// simSpawn runs a command in the simulator and waits for it.
var simSpawn = func(ctx context.Context, udid string, args ...string) error {
	_, err := xcrun(ctx, 30*time.Second, nil, append([]string{"simctl", "spawn", udid}, args...)...)
	return err
}

// logStream is `log stream` in the simulator, in ndjson, for predicate.
var logStream = func(ctx context.Context, udid, predicate string) *exec.Cmd {
	return exec.CommandContext(ctx, "xcrun", "simctl", "spawn", udid, "log", "stream", "--style", "ndjson", "--level", "info", "--predicate", predicate)
}

func (c *NetCollector) armHAR(ctx context.Context) error {
	if err := c.spawn(ctx, "log", "config", "--mode", "level:default", "--subsystem", "com.apple.CFNetwork", "--category", "HAR"); err != nil {
		return err
	}
	if err := c.spawn(ctx, "defaults", "write", "com.apple.CFNetwork", "har-body-size-limit", "-int", strconv.Itoa(harBodyLimit)); err != nil {
		return err
	}
	return c.leaseHAR(ctx)
}

func (c *NetCollector) leaseHAR(ctx context.Context) error {
	until := time.Now().Add(harLease).UTC().Format("2006-01-02T15:04:05Z")
	if err := c.spawn(ctx, "defaults", "write", "com.apple.CFNetwork", "har-capture-global", "-date", until); err != nil {
		return err
	}
	return c.spawn(ctx, "notifyutil", "-p", harNotification)
}

func (c *NetCollector) renewHAR(ctx context.Context) {
	ticker := time.NewTicker(harRenew)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.leaseHAR(ctx); err != nil && ctx.Err() == nil {
				c.Log("network: renewing HAR capture failed: " + err.Error())
			}
		}
	}
}

// disarmHAR switches HAR capture off and puts the HAR log category back to its
// shipped default (off).
func (c *NetCollector) disarmHAR() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, key := range []string{"har-capture-global", "har-body-size-limit"} {
		c.spawn(ctx, "defaults", "delete", "com.apple.CFNetwork", key)
	}
	c.spawn(ctx, "notifyutil", "-p", harNotification)
	c.spawn(ctx, "log", "config", "--reset", "--subsystem", "com.apple.CFNetwork", "--category", "HAR")
}

func (c *NetCollector) stream(ctx context.Context, har, summary bool) {
	var categories []string
	if har {
		categories = append(categories, `category == "HAR"`)
	}
	if summary {
		categories = append(categories, `category == "Summary"`)
	}
	if c.diagnostics {
		categories = append(categories, `category == "Diagnostics"`)
	}
	predicate := fmt.Sprintf(`subsystem == "com.apple.CFNetwork" AND process == %q AND (%s)`, c.Process, strings.Join(categories, " OR "))
	for ctx.Err() == nil {
		cmd := logStream(ctx, c.UDID, predicate)
		out, err := cmd.StdoutPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err == nil {
			sc := bufio.NewScanner(out)
			sc.Buffer(make([]byte, 1<<20), 64<<20)
			for sc.Scan() {
				if l, ok := parseCFLine(sc.Text()); ok {
					c.handleLine(l)
				}
			}
			err = cmd.Wait()
		}
		if ctx.Err() != nil {
			return
		}
		c.Log(fmt.Sprintf("network: log stream exited (%v); restarting in 1 s", err))
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

func (c *NetCollector) handleLine(l cfLine) {
	switch l.Category {
	case "HAR":
		if e, ok := parseHAR(l); ok {
			c.mu.Lock()
			if len(c.harTasks) > 10000 {
				c.harTasks = map[string]bool{}
			}
			c.harTasks[e.TaskID] = true
			c.mu.Unlock()
			c.push(e)
		}
	case "Diagnostics":
		if d, ok := parseDiagnostics(l); ok {
			c.mu.Lock()
			c.diag.add(d)
			c.mu.Unlock()
		}
	case "Summary":
		if e, ok := parseSummary(l); ok {
			c.onSummary(e)
		}
	}
}

// onSummary shows a Summary line unless HAR logged the same task, or rn-cdp
// is attached and lists JS requests with more detail.
func (c *NetCollector) onSummary(e NetworkEntry) {
	c.mu.Lock()
	c.diag.fill(&e)
	paused := c.states[SourceSummary].State != stateActive
	armed := c.harArmed
	c.mu.Unlock()
	if paused {
		return
	}
	if !armed {
		c.push(e)
		return
	}
	time.AfterFunc(summaryHold, func() {
		c.mu.Lock()
		claimed := c.harTasks[e.TaskID]
		if !claimed {
			c.unclaimed++
		}
		c.mu.Unlock()
		if !claimed {
			c.push(e)
		}
	})
}

// Status reports each source's state and entry count.
func (c *NetCollector) Status() NetStatus {
	st := NetStatus{Mode: c.Mode, Entries: c.Network.Len()}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, name := range []string{SourceHAR, SourceRNCDP, SourceSummary} {
		s := *c.states[name]
		s.Entries = c.Network.CountSource(name)
		if name == SourceHAR && s.State == stateActive {
			switch {
			case s.Entries == 0 && c.unclaimed > 0:
				s.Detail = fmt.Sprintf("armed, but CFNetwork logged no HAR entry for the %d request(s) the summary log saw; the app may lack the get-task-allow entitlement", c.unclaimed)
			case s.Entries == 0:
				s.Detail += "; no requests yet"
			}
		}
		st.Sources = append(st.Sources, s)
	}
	return st
}

// Close stops the sources, waits for `log stream` to exit, and switches HAR
// capture off. It must run while the simulator is still booted.
func (c *NetCollector) Close() {
	if c.cancel != nil {
		c.cancel()
	}
	// The daemon exits right after Close; a log stream not yet reaped would
	// outlive it.
	c.streams.Wait()
	if c.harArmed {
		c.disarmHAR()
		c.harArmed = false
	}
}

// newNetCollector starts the session's network sources.
func (s *Session) newNetCollector(ctx context.Context) *NetCollector {
	appPath := s.AppPath
	if appPath == "" {
		appPath, _ = AppContainer(ctx, s.UDID, s.BundleID)
	}
	c := &NetCollector{UDID: s.UDID, BundleID: s.BundleID, Process: s.Process, AppPath: appPath,
		Mode: cmpOr(s.Config.Network, NetworkAuto), MetroPort: s.MetroPort, Network: s.Network, Log: s.Log}
	c.Start(ctx)
	for _, st := range c.Status().Sources {
		s.Log(fmt.Sprintf("network: %s %s: %s", st.Name, st.State, st.Detail))
	}
	return c
}
