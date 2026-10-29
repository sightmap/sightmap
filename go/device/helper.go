package device

import (
	"bufio"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// The helper is a small Swift program on Meta's FBSimulatorControl (idb's
// prebuilt xcframeworks) that reads the simulator's accessibility tree and sends
// touch, key and button input. Its sources ship inside this binary and are built
// with the local Xcode on first use, so `go install`, the npm package and a
// release binary all get the same helper without a signed, per-Xcode prebuilt.
//
//go:embed helper/Package.swift helper/Sources
var helperSources embed.FS

const helperBinary = "sightmap-device-helper"

// CacheDir is where the helper is built and session files are kept:
// $SIGHTMAP_DEVICE_CACHE, else ~/Library/Caches/sightmap/device.
func CacheDir() string {
	if dir := os.Getenv("SIGHTMAP_DEVICE_CACHE"); dir != "" {
		return dir
	}
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "sightmap", "device")
	}
	return filepath.Join(os.TempDir(), "sightmap-device")
}

// helperHash identifies the embedded sources, so a changed helper rebuilds.
func helperHash() (string, error) {
	h := sha256.New()
	var paths []string
	err := fs.WalkDir(helperSources, "helper", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, p := range paths {
		data, err := helperSources.ReadFile(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// HelperPreflight returns why this machine cannot build or run the helper, or
// nil. Doctor reports the same checks one by one.
func HelperPreflight() error {
	if runtime.GOOS != "darwin" {
		return errorf("unsupported", "sightmap device needs macOS; this is %s", runtime.GOOS)
	}
	if runtime.GOARCH != "arm64" {
		if out, _ := exec.Command("sysctl", "-n", "hw.optional.arm64").Output(); strings.TrimSpace(string(out)) != "1" {
			return errorf("unsupported", "sightmap device needs an Apple Silicon Mac (idb's prebuilt frameworks are arm64 only)")
		}
	}
	dev, _ := exec.Command("xcode-select", "-p").Output()
	if !strings.HasSuffix(strings.TrimSpace(string(dev)), ".app/Contents/Developer") {
		return withHint("no_xcode", "install Xcode, open it once, then run `sudo xcode-select -s /Applications/Xcode.app/Contents/Developer`",
			"full Xcode is required, not just the Command Line Tools (xcode-select -p: %q)", strings.TrimSpace(string(dev)))
	}
	if _, err := os.Stat("/Library/Developer/PrivateFrameworks/CoreSimulator.framework"); err != nil {
		return withHint("no_xcode", "open Xcode once so it installs its components, or run `xcodebuild -runFirstLaunch`", "CoreSimulator is missing")
	}
	return nil
}

// BuildHelper returns the path of a helper binary for this machine, building it
// from the embedded sources on first use (a minute or two, then cached).
// $SIGHTMAP_DEVICE_HELPER names a prebuilt binary to use instead. progress, when
// set, receives a line before a build starts and the build's own output.
func BuildHelper(ctx context.Context, progress io.Writer) (string, error) {
	if path := os.Getenv("SIGHTMAP_DEVICE_HELPER"); path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", errorf("helper_build_failed", "SIGHTMAP_DEVICE_HELPER=%s: %v", path, err)
		}
		return path, nil
	}
	if err := HelperPreflight(); err != nil {
		return "", err
	}
	hash, err := helperHash()
	if err != nil {
		return "", err
	}
	root := filepath.Join(CacheDir(), "helper")
	binary := filepath.Join(root, hash, helperBinary)
	if _, err := os.Stat(binary); err == nil {
		return binary, nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	unlock, err := lockDir(ctx, filepath.Join(root, ".build.lock"))
	if err != nil {
		return "", err
	}
	defer unlock()
	// Another process may have finished the same build while this one waited.
	if _, err := os.Stat(binary); err == nil {
		return binary, nil
	}

	src := filepath.Join(root, "src-"+hash)
	if err := extractHelper(src); err != nil {
		return "", err
	}
	if progress == nil {
		progress = io.Discard
	}
	fmt.Fprintf(progress, "building the simulator helper (first use; downloads idb's frameworks, ~10 MB, and takes a minute or two)…\n")
	scratch := filepath.Join(root, ".build")
	// --disable-keychain/--disable-netrc keep SwiftPM from offering a saved
	// github.com credential (a keychain prompt) while it downloads public files.
	args := []string{"swift", "build", "-c", "release", "--package-path", src, "--scratch-path", scratch,
		"--cache-path", filepath.Join(root, ".swiftpm"), "--disable-keychain", "--disable-netrc"}
	var log strings.Builder
	cmd := exec.CommandContext(ctx, "xcrun", args...)
	cmd.Stdout = io.MultiWriter(&log, progress)
	cmd.Stderr = io.MultiWriter(&log, progress)
	started := time.Now()
	if err := cmd.Run(); err != nil {
		return "", withHint("helper_build_failed", "fix the errors above; a new Xcode release may need a sightmap update",
			"simulator helper build failed: %v\n%s", err, tail(log.String(), 15))
	}
	built := filepath.Join(scratch, "release", helperBinary)
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		return "", err
	}
	if err := copyFile(built, binary+".tmp", 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(binary+".tmp", binary); err != nil {
		return "", err
	}
	fmt.Fprintf(progress, "built the simulator helper in %.0f s\n", time.Since(started).Seconds())
	return binary, nil
}

func extractHelper(dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return fs.WalkDir(helperSources, "helper", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out := filepath.Join(dst, strings.TrimPrefix(path, "helper"))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := helperSources.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// lockDir takes a directory lock, waiting up to ten minutes for another build.
func lockDir(ctx context.Context, path string) (func(), error) {
	deadline := time.Now().Add(10 * time.Minute)
	for {
		if err := os.Mkdir(path, 0o755); err == nil {
			return func() { os.Remove(path) }, nil
		}
		if time.Now().After(deadline) {
			return nil, errorf("helper_build_failed", "timed out waiting for another helper build to release %s (remove it if no build is running)", path)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// Helper sends requests to the simulator helper.
type Helper interface {
	// Request sends cmd and decodes the result into out (when non-nil).
	Request(ctx context.Context, cmd string, args map[string]any, out any) error
	Alive() bool
	Restarts() int
	Close()
}

// HelperClient runs the helper as a long-lived process speaking JSON lines on
// stdin/stdout. When the process dies, pending requests fail with
// helper_crashed and the next request starts a fresh one.
type HelperClient struct {
	path    string
	log     func(string)
	verbose bool

	mu       sync.Mutex
	proc     *exec.Cmd
	stdin    io.WriteCloser
	nextID   int
	pending  map[int]chan helperReply
	restarts int
	closed   bool
}

type helperReply struct {
	OK     bool            `json:"ok"`
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *Error          `json:"error"`
}

// NewHelperClient returns a client for the helper at path; log receives the
// helper's failures (and with SIGHTMAP_DEVICE_DEBUG, all its stderr).
func NewHelperClient(path string, log func(string)) *HelperClient {
	if log == nil {
		log = func(string) {}
	}
	return &HelperClient{path: path, log: log, verbose: os.Getenv("SIGHTMAP_DEVICE_DEBUG") != "", pending: map[int]chan helperReply{}}
}

func (c *HelperClient) start() error {
	cmd := exec.Command(c.path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return errorf("helper_crashed", "simulator helper failed to start: %v", err)
	}
	c.proc, c.stdin = cmd, stdin
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			line := sc.Text()
			if c.verbose || strings.Contains(strings.ToLower(line), "fail") || strings.Contains(strings.ToLower(line), "error") {
				c.log("helper: " + line)
			}
		}
	}()
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var r helperReply
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				c.log("helper: unparseable line " + truncateRunes(sc.Text(), 200))
				continue
			}
			c.mu.Lock()
			ch := c.pending[r.ID]
			delete(c.pending, r.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- r
			}
		}
		err := cmd.Wait()
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.proc != cmd {
			return
		}
		c.proc = nil
		if !c.closed {
			c.restarts++
			c.log(fmt.Sprintf("helper exited (%v); restarting on next request", err))
		}
		for id, ch := range c.pending {
			ch <- helperReply{Error: errorf("helper_crashed", "simulator helper exited (%v)", err)}
			delete(c.pending, id)
		}
	}()
	return nil
}

// Request implements Helper.
func (c *HelperClient) Request(ctx context.Context, cmd string, args map[string]any, out any) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errorf("helper_closed", "the helper is shut down")
	}
	if c.proc == nil {
		if err := c.start(); err != nil {
			c.mu.Unlock()
			return err
		}
	}
	c.nextID++
	id := c.nextID
	ch := make(chan helperReply, 1)
	c.pending[id] = ch
	line, _ := json.Marshal(map[string]any{"id": id, "cmd": cmd, "args": args})
	_, werr := c.stdin.Write(append(line, '\n'))
	c.mu.Unlock()
	if werr != nil {
		c.forget(id)
		return errorf("helper_crashed", "simulator helper: %v", werr)
	}
	select {
	case r := <-ch:
		if r.Error != nil {
			if r.Error.Hint == "" {
				r.Error.Hint = hints[r.Error.Code]
			}
			return r.Error
		}
		if out != nil && len(r.Result) > 0 {
			return json.Unmarshal(r.Result, out)
		}
		return nil
	case <-ctx.Done():
		c.forget(id)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return withHint("helper_timeout", "retry; if it keeps timing out, run `sightmap device stop` then `sightmap device start`", "helper %s timed out", cmd)
		}
		return ctx.Err()
	}
}

func (c *HelperClient) forget(id int) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// Alive implements Helper.
func (c *HelperClient) Alive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.proc != nil
}

// Restarts implements Helper.
func (c *HelperClient) Restarts() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.restarts
}

// Close implements Helper.
func (c *HelperClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.proc != nil {
		c.stdin.Close()
		c.proc.Process.Kill()
	}
}
