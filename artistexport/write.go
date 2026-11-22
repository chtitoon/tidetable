package artistexport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Marshal renders the export as indented JSON with a trailing newline.
func Marshal(e Export) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(e); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CheckShrink guards against a broken API response silently wiping the
// library: it fails if the previous file at path had more than twice as many
// artists as the new export. A missing previous file is fine.
func CheckShrink(path string, e Export) error {
	old, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading previous %s: %w", path, err)
	}
	var prev struct {
		MyArtists []json.RawMessage `json:"my_artists"`
		Artists   []json.RawMessage `json:"artists"` // pre-2026-11-22 format
	}
	if err := json.Unmarshal(old, &prev); err != nil {
		return nil // unreadable previous file: nothing to compare against
	}
	prevCount := max(len(prev.MyArtists), len(prev.Artists))
	if len(e.MyArtists)*2 < prevCount {
		return fmt.Errorf("artist count dropped from %d to %d; refusing to overwrite (use -force if this is real)",
			prevCount, len(e.MyArtists))
	}
	return nil
}

// WriteAtomic writes data to path via a temp file in the same directory and a
// rename, so readers never see a partial file.
func WriteAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".artists-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
