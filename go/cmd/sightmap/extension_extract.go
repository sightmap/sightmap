package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ensureExtension extracts the embedded Chrome extension to
// ~/.sightmap/extension/ if it is missing or out of date, and returns the path
// to the extracted directory. A fresh `browser start` always launches a new
// Chrome process with --load-extension pointing here, so Chrome picks up a
// newly-extracted version directly — no service-worker hot-reload is needed.
func ensureExtension() (string, error) {
	// 1. Read embedded manifest to get the bundled version.
	embeddedData, err := extensionFS.ReadFile("extension/manifest.json")
	if err != nil {
		return "", fmt.Errorf("ensureExtension: read embedded manifest: %w", err)
	}
	var embeddedManifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(embeddedData, &embeddedManifest); err != nil {
		return "", fmt.Errorf("ensureExtension: parse embedded manifest: %w", err)
	}
	embeddedVersion := embeddedManifest.Version

	// 2. Compute target directory.
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ensureExtension: home dir: %w", err)
	}
	targetDir := filepath.Join(home, ".sightmap", "extension")

	// 3. Check on-disk version.
	diskManifestPath := filepath.Join(targetDir, "manifest.json")
	if diskData, readErr := os.ReadFile(diskManifestPath); readErr == nil {
		var diskManifest struct {
			Version string `json:"version"`
		}
		if jsonErr := json.Unmarshal(diskData, &diskManifest); jsonErr == nil {
			// 4. Versions match — nothing to do.
			if diskManifest.Version == embeddedVersion {
				return targetDir, nil
			}
		}
	}

	// 5. Version missing or differs — (re)install.
	// a. Remove old directory.
	if rmErr := os.RemoveAll(targetDir); rmErr != nil {
		return "", fmt.Errorf("ensureExtension: remove old extension: %w", rmErr)
	}

	// b–c. Walk embedded FS and write every file.
	if walkErr := fs.WalkDir(extensionFS, "extension", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Strip leading "extension/" prefix to get relative path inside targetDir.
		rel, relErr := filepath.Rel("extension", path)
		if relErr != nil {
			return relErr
		}
		dest := filepath.Join(targetDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}

		data, readErr := extensionFS.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read embedded %s: %w", path, readErr)
		}
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0o755); mkErr != nil {
			return mkErr
		}
		return os.WriteFile(dest, data, 0o644)
	}); walkErr != nil {
		return "", fmt.Errorf("ensureExtension: extract files: %w", walkErr)
	}

	// d. Log installation.
	fmt.Fprintf(os.Stderr, "[browser] installed extension v%s → %s\n", embeddedVersion, targetDir)

	return targetDir, nil
}
