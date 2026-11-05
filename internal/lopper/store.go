package lopper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Config comes from the environment; the command line can change Depth.
type Config struct {
	Depth    int    // how deep to look for repositories
	CacheDir string // saved scans and the clean log
	BloomDB  string // Bloom's sqlite database
	BloomDir string // the folder of Bloom's workspaces
}

// ConfigFromEnv reads LOPPER_DEPTH, LOPPER_CACHE_DIR, XDG_CACHE_HOME,
// LOPPER_BLOOM_DB and LOPPER_BLOOM_DIR.
func ConfigFromEnv() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("cannot find the home folder: %w", err)
	}
	c := Config{
		Depth:    3,
		CacheDir: env("LOPPER_CACHE_DIR", filepath.Join(env("XDG_CACHE_HOME", filepath.Join(home, ".cache")), "git-lopper")),
		BloomDB:  env("LOPPER_BLOOM_DB", filepath.Join(home, "Library", "Application Support", "Bloom", "bloom.sqlite")),
		BloomDir: filepath.Clean(env("LOPPER_BLOOM_DIR", filepath.Join(home, "bloom", "workspaces.noindex"))),
	}
	if d := os.Getenv("LOPPER_DEPTH"); d != "" {
		if c.Depth, err = ParseDepth(d); err != nil {
			return Config{}, fmt.Errorf("LOPPER_DEPTH: %w", err)
		}
	}
	return c, nil
}

// ParseDepth accepts a whole number of 1 or more.
func ParseDepth(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%q is not a number of 1 or more", s)
	}
	return n, nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// LogPath is the file that holds git's output of the last clean.
func (c Config) LogPath() string { return filepath.Join(c.CacheDir, "last-clean.log") }

// Saved is the saved scan of one root folder.
type Saved struct {
	Root      string    `json:"root"`
	ScannedAt time.Time `json:"scanned_at"`
	Rows      []Row     `json:"worktrees"`
}

// ErrNoScan means that root has no saved scan yet.
var ErrNoScan = errors.New("no saved scan")

// ScanPath is where the scan of root is saved.
func (c Config) ScanPath(root string) string {
	sum := sha256.Sum256([]byte(root))
	return filepath.Join(c.CacheDir, hex.EncodeToString(sum[:])+".json")
}

// Load reads a saved scan. It returns ErrNoScan when there is none.
func Load(path string) (Saved, error) {
	var s Saved
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, ErrNoScan
	}
	if err != nil {
		return s, fmt.Errorf("cannot read the saved scan: %w", err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("the saved scan %s is damaged, scan again: %w", path, err)
	}
	return s, nil
}

// Save writes a scan; a reader never sees half a file.
func Save(path string, s Saved) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode the scan: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("cannot create %s: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".scan-*")
	if err != nil {
		return fmt.Errorf("cannot save the scan: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot save the scan: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cannot save the scan: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("cannot save the scan: %w", err)
	}
	return nil
}

// Without returns the scan minus the rows of the removed paths.
func (s Saved) Without(removed []string) Saved {
	out := Saved{Root: s.Root, ScannedAt: s.ScannedAt}
	for _, r := range s.Rows {
		if !slices.Contains(removed, r.Path) {
			out.Rows = append(out.Rows, r)
		}
	}
	return out
}

// Of returns the rows of the wanted classes, in scan order.
func Of(rows []Row, wanted []Class) []Row {
	var out []Row
	for _, r := range rows {
		if slices.Contains(wanted, r.Class) {
			out = append(out, r)
		}
	}
	return out
}

// TSVHeader names the columns of WriteTSV.
const TSVHeader = "class\treason\trepo\tpath\tbranch\tbase\tpr\tpr_head\tupstream\tlast_commit"

// WriteTSV writes a header line and one tab-separated line per row.
func WriteTSV(w io.Writer, rows []Row) error {
	if _, err := fmt.Fprintln(w, TSVHeader); err != nil {
		return err
	}
	for _, r := range rows {
		f := []string{string(r.Class), r.Reason, r.Repo, r.Path, r.Branch, r.Base, r.PR, r.PRHead, r.Upstream, r.LastCommit}
		if _, err := fmt.Fprintln(w, strings.Join(f, "\t")); err != nil {
			return err
		}
	}
	return nil
}

// ScanAndSave scans root and saves the result. warn comes from Scan.
func (c Config) ScanAndSave(root string, fetch bool, report Progress) (Saved, string, error) {
	rows, warn := Scan(root, c.Depth, fetch, report)
	s := Saved{Root: root, ScannedAt: time.Now(), Rows: rows}
	return s, warn, Save(c.ScanPath(root), s)
}
