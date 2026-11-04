package device

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionFile is the session record inside the sightmap directory, the
// device counterpart of the browser's `.session`.
const SessionFile = ".device-session"

// SessionInfo is what the session file records: how to reach the daemon and
// what it drives.
type SessionInfo struct {
	Socket     string `json:"socket"`
	PID        int    `json:"pid"`
	UDID       string `json:"udid"`
	DeviceName string `json:"deviceName,omitempty"`
	BundleID   string `json:"bundleId"`
	AppPath    string `json:"appPath,omitempty"`
	StartedAt  string `json:"startedAt"`
	Run        string `json:"run,omitempty"`
	LogFile    string `json:"logFile,omitempty"`

	File string `json:"-"`
}

// SessionPath returns the session file path for the corpus at sightmapDir.
func SessionPath(sightmapDir string) string { return filepath.Join(sightmapDir, SessionFile) }

// ReadSession reads the session file, or returns nil when there is none.
func ReadSession(sightmapDir string) *SessionInfo {
	data, err := os.ReadFile(SessionPath(sightmapDir))
	if err != nil {
		return nil
	}
	var info SessionInfo
	if json.Unmarshal(data, &info) != nil || info.Socket == "" {
		return nil
	}
	info.File = SessionPath(sightmapDir)
	return &info
}

// LiveSession returns the running session for sightmapDir, removing a stale
// session file whose daemon is gone or no longer answers.
func LiveSession(sightmapDir string) *SessionInfo {
	info := ReadSession(sightmapDir)
	if info == nil {
		return nil
	}
	if pidAlive(info.PID) {
		if err := Call(info.Socket, "ping", nil, nil, 5*time.Second); err == nil {
			return info
		}
	}
	os.Remove(info.File)
	return nil
}

// RequireSession is LiveSession, erroring when there is none.
func RequireSession(sightmapDir string) (*SessionInfo, error) {
	if info := LiveSession(sightmapDir); info != nil {
		return info, nil
	}
	return nil, errorf("no_session", "no running device session (no live %s)", SessionPath(sightmapDir))
}

// maxSocketPath is the longest Unix socket path macOS accepts: sun_path holds
// 104 bytes including the terminating NUL.
const maxSocketPath = 103

// SocketPath returns the daemon socket for sightmapDir and udid. It lives in
// the temp dir because a Unix socket path is limited in length, and in /tmp
// when the temp dir is itself too long (as in some agent sandboxes).
func SocketPath(sightmapDir, udid string) string {
	abs, _ := filepath.Abs(sightmapDir)
	sum := sha256.Sum256([]byte(abs + "\x00" + udid))
	name := "sightmap-device-" + hex.EncodeToString(sum[:])[:12] + ".sock"
	if p := filepath.Join(os.TempDir(), name); len(p) <= maxSocketPath {
		return p
	}
	return filepath.Join("/tmp", name)
}

type request struct {
	Cmd  string          `json:"cmd"`
	Args json.RawMessage `json:"args,omitempty"`
}

type reply struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Call sends one command to the daemon at socket and decodes its result into
// out (when non-nil).
func Call(socket, cmd string, args, out any, timeout time.Duration) error {
	conn, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		return errorf("daemon_unreachable", "cannot reach the device daemon at %s (%v)", socket, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))
	raw, _ := json.Marshal(args)
	line, _ := json.Marshal(request{Cmd: cmd, Args: raw})
	if _, err := conn.Write(append(line, '\n')); err != nil {
		return errorf("daemon_unreachable", "cannot reach the device daemon: %v", err)
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 1<<20), 256<<20)
	if !sc.Scan() {
		if ne, ok := sc.Err().(net.Error); ok && ne.Timeout() {
			return withHint("daemon_timeout", "check `sightmap device status`; if it is stuck, `sightmap device stop` then `sightmap device start --detach`",
				"the daemon did not answer %s within %s", cmd, timeout)
		}
		return errorf("daemon_unreachable", "the daemon closed the connection without answering")
	}
	var r reply
	if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
		return err
	}
	if !r.OK {
		if r.Error == nil {
			r.Error = &Error{Code: "internal", Message: "the daemon failed without an error"}
		}
		return r.Error
	}
	if out != nil && len(r.Result) > 0 {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

// Daemon serves a Session over a Unix socket and records it in the session
// file until Stop.
type Daemon struct {
	Session  *Session
	Info     SessionInfo
	listener net.Listener
	stopOnce sync.Once
	stopped  chan struct{}
}

// Serve launches the session's app (when launch is set), starts the console
// collector, begins a run, listens, and writes the session file. It returns
// once the daemon is serving; Wait blocks until it stops.
func Serve(s *Session, launch bool, logFile string) (_ *Daemon, err error) {
	ctx := context.Background()
	if err := os.MkdirAll(s.SightmapDir, 0o755); err != nil {
		return nil, err
	}
	s.Timing = s.Timing.withDefaults()
	if s.RunDir != "" {
		if err := os.MkdirAll(s.RunDir, 0o755); err != nil {
			return nil, err
		}
		for _, p := range []string{s.stdoutPath(), s.stderrPath()} {
			os.WriteFile(p, nil, 0o644)
		}
	}
	// A daemon that fails to start exits; its log streams and HAR capture must
	// not outlive it.
	defer func() {
		if err != nil {
			s.closeCollectors()
		}
	}()
	if s.Process != "" && s.Logs == nil {
		s.Logs = &LogCollector{UDID: s.UDID, Process: s.Process, Stdout: s.stdoutPath(), Stderr: s.stderrPath(), Console: s.Console, Log: s.Log}
		s.Logs.Start()
		// log stream misses what happens before it attaches.
		time.Sleep(500 * time.Millisecond)
	}
	if s.Process != "" && s.Net == nil {
		s.Net = s.newNetCollector(ctx)
	}
	var hello map[string]any
	if err := s.Helper.Request(ctx, "hello", nil, &hello); err != nil {
		return nil, err
	}
	s.Log(fmt.Sprintf("helper %v ready", hello["version"]))
	if _, err := s.RunLog.Begin(s.RunLabel); err != nil {
		return nil, err
	}
	if launch {
		if _, err := s.LaunchApp(ctx, true); err != nil {
			return nil, err
		}
		s.Log(fmt.Sprintf("launched %s (pid %d)", s.BundleID, s.AppPID))
	}
	s.RunLog.Append(RunEntry{Action: "start", OK: true, Args: map[string]any{"udid": s.UDID, "deviceName": s.DeviceName, "bundleId": s.BundleID, "appPath": s.AppPath}})

	socket := SocketPath(s.SightmapDir, s.UDID)
	os.Remove(socket)
	ln, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	d := &Daemon{Session: s, listener: ln, stopped: make(chan struct{})}
	d.Info = SessionInfo{
		Socket: socket, PID: os.Getpid(), UDID: s.UDID, DeviceName: s.DeviceName, BundleID: s.BundleID, AppPath: s.AppPath,
		StartedAt: s.StartedAt.UTC().Format(time.RFC3339), Run: s.RunLog.Current().Run, LogFile: logFile,
	}
	go d.accept()
	data, _ := json.MarshalIndent(d.Info, "", "  ")
	if err := os.WriteFile(SessionPath(s.SightmapDir), append(data, '\n'), 0o644); err != nil {
		ln.Close()
		return nil, err
	}
	d.Info.File = SessionPath(s.SightmapDir)
	s.Log("serving on " + socket)
	return d, nil
}

func (d *Daemon) accept() {
	for {
		conn, err := d.listener.Accept()
		if err != nil {
			return
		}
		go d.serve(conn)
	}
}

// StopArgs are the stop command's arguments.
type StopArgs struct {
	Shutdown bool `json:"shutdown,omitempty"`
}

func (d *Daemon) serve(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	if !sc.Scan() {
		return
	}
	write := func(r reply) {
		data, _ := json.Marshal(r)
		conn.Write(append(data, '\n'))
	}
	var req request
	if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
		write(reply{Error: errorf("bad_request", "request is not JSON")})
		return
	}
	if req.Cmd == "stop" {
		a, _ := decode[StopArgs](req.Args)
		result, _ := json.Marshal(map[string]bool{"stopped": true, "shutdown": a.Shutdown})
		write(reply{OK: true, Result: result})
		conn.Close()
		d.Stop(a.Shutdown)
		return
	}
	result, err := d.Session.Handle(context.Background(), req.Cmd, req.Args)
	if err != nil {
		e := AsError(err)
		if e.Code == "internal" {
			d.Session.Log(fmt.Sprintf("internal error in %s: %v", req.Cmd, err))
		}
		write(reply{Error: e})
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		write(reply{Error: AsError(err)})
		return
	}
	write(reply{OK: true, Result: data})
}

// Stop terminates the app (and with shutdown, the simulator), closes the
// session and removes the session file and socket. It is safe to call twice.
func (d *Daemon) Stop(shutdown bool) {
	d.stopOnce.Do(func() { d.stop(shutdown) })
}

func (d *Daemon) stop(shutdown bool) {
	s := d.Session
	ctx := context.Background()
	s.Log("stopping")
	s.Sim.Terminate(ctx, s.UDID, s.BundleID)
	// Closing switches HAR capture off inside the simulator, so it must come
	// before a shutdown.
	s.Close()
	if shutdown {
		s.Sim.Shutdown(ctx, s.UDID)
	}
	d.listener.Close()
	os.Remove(d.Info.Socket)
	if cur := ReadSession(s.SightmapDir); cur != nil && cur.PID == d.Info.PID {
		os.Remove(SessionPath(s.SightmapDir))
	}
	close(d.stopped)
}

// Wait blocks until the daemon stops.
func (d *Daemon) Wait() { <-d.stopped }
