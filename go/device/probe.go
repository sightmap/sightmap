package device

import (
	"bufio"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The probe is an optional dylib that `device start --probe` loads into the app
// with DYLD_INSERT_LIBRARIES. It serves the in-process accessibility tree, the
// UIView tree and SwiftUI's view debug graph over a Unix socket, which the
// session merges onto the out-of-process tree. Like the helper, it ships as
// source and is compiled with the local Xcode, then cached by content hash.
//
//go:embed probe/probe.m
var probeSource []byte

// BuildProbe returns the path of the probe dylib, compiling it on first use.
// $SIGHTMAP_DEVICE_PROBE names a prebuilt dylib to use instead.
func BuildProbe(ctx context.Context) (string, error) {
	if path := os.Getenv("SIGHTMAP_DEVICE_PROBE"); path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", errorf("probe_build_failed", "SIGHTMAP_DEVICE_PROBE=%s: %v", path, err)
		}
		return path, nil
	}
	sum := sha256.Sum256(probeSource)
	dir := filepath.Join(CacheDir(), "probe", hex.EncodeToString(sum[:])[:16])
	dylib := filepath.Join(dir, "sightmap-probe.dylib")
	if _, err := os.Stat(dylib); err == nil {
		return dylib, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// Two starts may build at once; each writes its own files and the rename
	// is atomic.
	src := filepath.Join(dir, fmt.Sprintf("probe-%d.m", os.Getpid()))
	tmp := dylib + fmt.Sprintf(".%d.tmp", os.Getpid())
	defer os.Remove(src)
	if err := os.WriteFile(src, probeSource, 0o644); err != nil {
		return "", err
	}
	// The linker's ad-hoc signature is enough for dyld in the simulator; no
	// codesign step, and the app itself is never re-signed.
	out, err := exec.CommandContext(ctx, "xcrun", "-sdk", "iphonesimulator", "clang", "-arch", "arm64",
		"-mios-simulator-version-min=17.0", "-dynamiclib", "-fobjc-arc", "-framework", "UIKit", "-framework", "CoreGraphics", "-framework", "Foundation",
		"-o", tmp, src).CombinedOutput()
	if err != nil {
		os.Remove(tmp)
		return "", withHint("probe_build_failed", "check that Xcode's iOS Simulator SDK is installed (`xcrun -sdk iphonesimulator --show-sdk-path`)",
			"probe build failed: %v\n%s", err, tail(string(out), 15))
	}
	return dylib, os.Rename(tmp, dylib)
}

// ProbeSocketPath is where the probe in udid's app listens.
func ProbeSocketPath(udid string) string {
	sum := sha256.Sum256([]byte("probe\x00" + udid))
	name := "sightmap-probe-" + hex.EncodeToString(sum[:])[:12] + ".sock"
	if p := filepath.Join(os.TempDir(), name); len(p) <= maxSocketPath {
		return p
	}
	return filepath.Join("/tmp", name)
}

// ProbeLaunchEnv is the launch environment that loads the probe. SwiftUI
// builds its view debug graph only when SWIFTUI_VIEW_DEBUG is set at launch.
func ProbeLaunchEnv(dylib, socket string) []string {
	return []string{"DYLD_INSERT_LIBRARIES=" + dylib, "SWIFTUI_VIEW_DEBUG=" + swiftUIDebugMask, "SIGHTMAP_PROBE_SOCKET=" + socket}
}

// SWIFTUI_VIEW_DEBUG is a bit mask of what the graph records. Type and value
// come with any non-zero mask; 8 adds position and 16 size.
const swiftUIDebugMask = "27"

// ProbeFieldsNone is Config.ProbeFields to leave app views' stored fields out.
const ProbeFieldsNone = "none"

// ProbeDump is the probe's dump result.
type ProbeDump struct {
	Meta struct {
		Screen Size               `json:"screen"`
		Timing map[string]float64 `json:"timing"`
		// Skipped counts hosting views left out of an onScreen dump.
		Skipped int          `json:"skipped"`
		AX      []*ProbeAX   `json:"ax"`
		Views   []*ProbeView `json:"views"`
	} `json:"meta"`
	Hosts []ProbeHost `json:"hosts"`
}

// ProbeAX is one in-process accessibility node; F is its screen frame.
type ProbeAX struct {
	Class  string     `json:"cls"`
	El     bool       `json:"el"`
	F      [4]float64 `json:"f"`
	ID     string     `json:"id"`
	Label  string     `json:"label"`
	Value  string     `json:"value"`
	Traits uint64     `json:"traits"`
	Ptr    string     `json:"ptr"`
	Kids   []*ProbeAX `json:"kids"`
}

// ProbeView is one UIView; F is its frame in its window.
type ProbeView struct {
	Class string       `json:"cls"`
	Ptr   string       `json:"ptr"`
	F     [4]float64   `json:"f"`
	ID    string       `json:"id"`
	Kids  []*ProbeView `json:"kids"`
}

// ProbeHost is one SwiftUI hosting view and its debug graph.
type ProbeHost struct {
	Info struct {
		Class string     `json:"cls"`
		Ptr   string     `json:"ptr"`
		F     [4]float64 `json:"f"`
		MS    float64    `json:"ms"`
		Bytes int        `json:"bytes"`
	} `json:"info"`
	Graph json.RawMessage `json:"graph"`
}

// ProbeRequest sends one command to the probe at socket.
func ProbeRequest(ctx context.Context, socket, cmd string, args map[string]any, out any) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socket)
	if err != nil {
		return withHint("probe_unreachable", "start the session with --probe, or relaunch the app", "cannot reach the in-app probe at %s (%v)", socket, err)
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	} else {
		conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	line, _ := json.Marshal(map[string]any{"id": 1, "cmd": cmd, "args": args})
	if _, err := conn.Write(append(line, '\n')); err != nil {
		return errorf("probe_unreachable", "in-app probe: %v", err)
	}
	r := bufio.NewReaderSize(conn, 1<<20)
	data, err := r.ReadBytes('\n')
	if err != nil {
		return errorf("probe_unreachable", "in-app probe closed the connection: %v", err)
	}
	var rep struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  *Error          `json:"error"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		return fmt.Errorf("in-app probe: %w", err)
	}
	if !rep.OK {
		if rep.Error == nil {
			rep.Error = &Error{Code: "probe_failed", Message: strings.TrimSpace(string(data))}
		}
		return rep.Error
	}
	if out != nil {
		return json.Unmarshal(rep.Result, out)
	}
	return nil
}
