package browser

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const cftVersionsURL = "https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json"

// cftVersionsJSON is the top-level shape of the CfT JSON endpoint.
type cftVersionsJSON struct {
	Channels map[string]struct {
		Version   string `json:"version"`
		Downloads struct {
			Chrome []struct {
				Platform string `json:"platform"`
				URL      string `json:"url"`
			} `json:"chrome"`
		} `json:"downloads"`
	} `json:"channels"`
}

// cftPlatform returns the Chrome for Testing platform string for the current
// OS/arch, or "" if the platform is unsupported.
func cftPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "mac-arm64"
		}
		return "mac-x64"
	case "linux":
		return "linux64"
	case "windows":
		if runtime.GOARCH == "386" {
			return "win32"
		}
		return "win64"
	}
	return ""
}

// cftRelBin returns the path of the Chrome binary relative to the extracted
// zip root (which is also the install directory root).
func cftRelBin(platform string) string {
	switch platform {
	case "mac-arm64", "mac-x64":
		return filepath.Join(
			"chrome-"+platform,
			"Google Chrome for Testing.app",
			"Contents", "MacOS", "Google Chrome for Testing",
		)
	case "linux64":
		return filepath.Join("chrome-linux64", "chrome")
	case "win64":
		return filepath.Join("chrome-win64", "chrome.exe")
	case "win32":
		return filepath.Join("chrome-win32", "chrome.exe")
	}
	return ""
}

// CfTCacheDir returns the directory where sightmap stores its own CfT
// installations (~/.sightmap/browsers).
func CfTCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sightmap", "browsers"), nil
}

// InstalledCfTPath returns the path to the highest-version Chrome for Testing
// binary installed by sightmap, or ("", false) if none is present.
func InstalledCfTPath() (string, bool) {
	platform := cftPlatform()
	if platform == "" {
		return "", false
	}
	relBin := cftRelBin(platform)
	if relBin == "" {
		return "", false
	}
	cacheDir, err := CfTCacheDir()
	if err != nil {
		return "", false
	}
	// filepath.Glob returns sorted results; the last entry is the
	// lexicographically highest version string, e.g. "chrome-149.0.x.y".
	matches, _ := filepath.Glob(filepath.Join(cacheDir, "chrome-*"))
	if len(matches) == 0 {
		return "", false
	}
	binPath := filepath.Join(matches[len(matches)-1], relBin)
	if _, err := os.Stat(binPath); err == nil {
		return binPath, true
	}
	return "", false
}

// ResolveLatestStableCfT queries the CfT JSON API and returns the current
// Stable channel version and the download URL for the current platform.
func ResolveLatestStableCfT(ctx context.Context) (version, downloadURL string, err error) {
	platform := cftPlatform()
	if platform == "" {
		return "", "", fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cftVersionsURL, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetch CfT versions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("fetch CfT versions: HTTP %d", resp.StatusCode)
	}

	var data cftVersionsJSON
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", fmt.Errorf("parse CfT versions: %w", err)
	}

	stable, ok := data.Channels["Stable"]
	if !ok {
		return "", "", fmt.Errorf("no Stable channel in CfT versions response")
	}
	for _, dl := range stable.Downloads.Chrome {
		if dl.Platform == platform {
			return stable.Version, dl.URL, nil
		}
	}
	return "", "", fmt.Errorf("no download URL for platform %q in CfT Stable channel", platform)
}

// InstallCfT downloads and installs Chrome for Testing into the sightmap
// cache directory (~/.sightmap/browsers/chrome-<version>/).
//
// It is idempotent: if the current Stable version is already installed the
// binary path is returned immediately without a download.
//
// Progress messages are written to w (pass os.Stderr for interactive use,
// io.Discard to suppress).
func InstallCfT(ctx context.Context, w io.Writer) (binaryPath string, err error) {
	platform := cftPlatform()
	if platform == "" {
		return "", fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	relBin := cftRelBin(platform)

	fmt.Fprintf(w, "Resolving latest Chrome for Testing (Stable)...\n")
	version, downloadURL, err := ResolveLatestStableCfT(ctx)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(w, "  version: %s  platform: %s\n", version, platform)

	cacheDir, err := CfTCacheDir()
	if err != nil {
		return "", err
	}
	installDir := filepath.Join(cacheDir, "chrome-"+version)
	binPath := filepath.Join(installDir, relBin)

	// Already installed?
	if _, statErr := os.Stat(binPath); statErr == nil {
		fmt.Fprintf(w, "Already installed: %s\n", binPath)
		return binPath, nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	// Download to a temp file in the cache dir (same filesystem → atomic rename).
	tmp, err := os.CreateTemp(cacheDir, "cft-download-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	fmt.Fprintf(w, "Downloading %s\n", downloadURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}

	pr := &cftProgressReader{r: resp.Body, w: w, total: resp.ContentLength}
	if _, err := io.Copy(tmp, pr); err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	tmp.Close()
	fmt.Fprintf(w, "\n")

	fmt.Fprintf(w, "Extracting...\n")
	if err := cftExtractZip(tmpPath, installDir); err != nil {
		os.RemoveAll(installDir)
		return "", fmt.Errorf("extract: %w", err)
	}

	if _, err := os.Stat(binPath); err != nil {
		return "", fmt.Errorf("extraction succeeded but binary not found at expected path:\n  %s", binPath)
	}
	fmt.Fprintf(w, "Installed: %s\n", binPath)
	return binPath, nil
}

// ── progress ──────────────────────────────────────────────────────────────────

type cftProgressReader struct {
	r       io.Reader
	w       io.Writer
	total   int64
	written int64
	lastPct int64
}

func (pr *cftProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.r.Read(p)
	pr.written += int64(n)
	if pr.total > 0 {
		pct := pr.written * 100 / pr.total
		if pct/5 != pr.lastPct/5 { // print every 5%
			fmt.Fprintf(pr.w, "\r  %3d%%  %d / %d MB  ",
				pct, pr.written>>20, pr.total>>20)
			pr.lastPct = pct
		}
	} else {
		mb := pr.written >> 20
		prev := (pr.written - int64(n)) >> 20
		if mb != prev {
			fmt.Fprintf(pr.w, "\r  %d MB  ", mb)
		}
	}
	return
}

// ── zip extraction ────────────────────────────────────────────────────────────

// cftExtractZip extracts a zip file to destDir, preserving Unix permissions
// and correctly handling symlinks that appear in macOS .app bundles.
func cftExtractZip(zipPath, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if err := cftExtractEntry(f, destDir); err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
	}
	return nil
}

// Unix file-type bits used in zip ExternalAttrs.
const (
	unixSymlink  = 0o120000 // symlink type
	unixTypeMask = 0o170000 // file-type mask
)

func cftExtractEntry(f *zip.File, destDir string) error {
	// Zip-slip guard: reject any path that would escape destDir.
	cleanDest := filepath.Clean(destDir)
	dest := filepath.Clean(filepath.Join(cleanDest, filepath.FromSlash(f.Name)))
	if dest != cleanDest && !strings.HasPrefix(dest, cleanDest+string(os.PathSeparator)) {
		return fmt.Errorf("path escapes destination directory")
	}

	// Unix mode from upper 16 bits of ExternalAttrs.
	unixMode := os.FileMode(f.ExternalAttrs >> 16)
	perm := unixMode & 0o777

	// ── symlink ──
	if unixMode&unixTypeMask == unixSymlink {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		target, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		os.Remove(dest) // remove stale link if re-extracting
		return os.Symlink(string(target), dest)
	}

	// ── directory ──
	if f.FileInfo().IsDir() {
		if perm == 0 {
			perm = 0o755
		}
		return os.MkdirAll(dest, perm)
	}

	// ── regular file ──
	if perm == 0 {
		perm = 0o644
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
