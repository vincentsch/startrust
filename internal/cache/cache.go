// Package cache saves finished checks and account lookups on disk.
//
// Layout under the cache root:
//
//	checks/<owner>/<name>/<time>.json   one file per finished check
//	accounts/<owner>/<name>/<day>.json  GH Archive answers per day
//	in-progress/<owner>__<name>.json    an unfinished --deep stargazer list
//
// Names are lowercased because GitHub names are case-insensitive and so are
// the default file systems on macOS and Windows.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store is a cache directory.
type Store struct {
	Root string
}

// DefaultRoot returns the cache directory for this user: on Linux
// ~/.cache/startrust, on macOS ~/Library/Caches/startrust, on Windows
// %LocalAppData%\startrust.
func DefaultRoot() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cannot find your cache directory: %w", err)
	}
	return filepath.Join(base, "startrust"), nil
}

// Open returns the store at root, creating it with owner-only permissions.
func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create cache directory %s: %w", root, err)
	}
	return &Store{Root: root}, nil
}

const timeLayout = "20060102T150405Z"

// Entry is one finished check in the cache.
type Entry struct {
	Repository string    `json:"repository"`
	CheckedAt  time.Time `json:"checked_at"`
	Path       string    `json:"path"`
	Bytes      int64     `json:"bytes"`
}

// SaveCheck writes a finished check and returns its path.
func (s *Store) SaveCheck(repo string, checkedAt time.Time, data []byte) (string, error) {
	rel := filepath.Join("checks", repoDir(repo), checkedAt.UTC().Format(timeLayout)+".json")
	return rel, s.write(rel, data)
}

// Checks lists finished checks, newest first. With repo set, only that repo.
func (s *Store) Checks(repo string) ([]Entry, error) {
	pattern := filepath.Join(s.Root, "checks", "*", "*", "*.json")
	if repo != "" {
		pattern = filepath.Join(s.Root, "checks", repoDir(repo), "*.json")
	}
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		checkedAt, err := time.Parse(timeLayout, strings.TrimSuffix(filepath.Base(path), ".json"))
		if err != nil {
			continue
		}
		dir := filepath.Dir(path)
		entries = append(entries, Entry{
			Repository: filepath.Base(filepath.Dir(dir)) + "/" + filepath.Base(dir),
			CheckedAt:  checkedAt,
			Path:       path,
			Bytes:      info.Size(),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CheckedAt.After(entries[j].CheckedAt) })
	return entries, nil
}

// LatestCheck returns the newest finished check of a repo.
func (s *Store) LatestCheck(repo string) ([]byte, Entry, error) {
	entries, err := s.Checks(repo)
	if err != nil {
		return nil, Entry{}, err
	}
	if len(entries) == 0 {
		return nil, Entry{}, fs.ErrNotExist
	}
	data, err := s.read(entries[0].Path)
	return data, entries[0], err
}

// Plan lists what a prune removes or would remove.
type Plan struct {
	Files  int     `json:"files"`
	Bytes  int64   `json:"bytes"`
	Kept   int     `json:"kept"`
	DryRun bool    `json:"dry_run"`
	Items  []Entry `json:"removed"`
}

// Prune removes checks older than cutoff, but never the newest check of a
// repo. It also removes account lookups and in-progress files older than
// cutoff.
func (s *Store) Prune(cutoff time.Time, dryRun bool) (Plan, error) {
	plan := Plan{DryRun: dryRun, Items: []Entry{}}
	entries, err := s.Checks("")
	if err != nil {
		return plan, err
	}
	newest := map[string]bool{}
	for _, entry := range entries { // newest first
		if !newest[entry.Repository] {
			newest[entry.Repository] = true
			plan.Kept++
			continue
		}
		if entry.CheckedAt.Before(cutoff) {
			plan.Items = append(plan.Items, entry)
		} else {
			plan.Kept++
		}
	}
	for _, dir := range []string{"accounts", "in-progress"} {
		_ = filepath.WalkDir(filepath.Join(s.Root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() {
				return nil
			}
			if info, err := d.Info(); err == nil && info.ModTime().Before(cutoff) {
				plan.Items = append(plan.Items, Entry{Repository: repoOf(dir, path), CheckedAt: info.ModTime().UTC().Truncate(time.Second),
					Path: path, Bytes: info.Size()})
			}
			return nil
		})
	}
	for _, item := range plan.Items {
		plan.Files++
		plan.Bytes += item.Bytes
		if !dryRun {
			if err := os.Remove(item.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return plan, err
			}
		}
	}
	return plan, nil
}

// Load reads a JSON value stored under kind/repo/key. It reports false when
// there is none or it cannot be read.
func (s *Store) Load(kind, repo, key string, v any) bool {
	data, err := s.read(filepath.Join(s.Root, kind, repoDir(repo), key+".json"))
	return err == nil && json.Unmarshal(data, v) == nil
}

// Save stores a JSON value under kind/repo/key.
func (s *Store) Save(kind, repo, key string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.write(filepath.Join(kind, repoDir(repo), key+".json"), data)
}

// SaveProgress overwrites the in-progress file of a repo.
func (s *Store) SaveProgress(repo string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.write(progressPath(repo), data)
}

// LoadProgress reads the in-progress file of a repo, if any.
func (s *Store) LoadProgress(repo string, v any) bool {
	data, err := s.read(filepath.Join(s.Root, progressPath(repo)))
	return err == nil && json.Unmarshal(data, v) == nil
}

// ClearProgress removes the in-progress file of a repo.
func (s *Store) ClearProgress(repo string) {
	_ = os.Remove(filepath.Join(s.Root, progressPath(repo)))
}

func progressPath(repo string) string {
	return filepath.Join("in-progress", strings.ReplaceAll(strings.ToLower(repo), "/", "__")+".json")
}

// repoOf reads the repo name back from the path of an account lookup
// (accounts/owner/name/day.json) or an unfinished stargazer list
// (in-progress/owner__name.json). Owner names cannot contain "_", so the
// first "__" separates owner and name.
func repoOf(kind, path string) string {
	if kind == "in-progress" {
		return strings.Replace(strings.TrimSuffix(filepath.Base(path), ".json"), "__", "/", 1)
	}
	dir := filepath.Dir(path)
	return filepath.Base(filepath.Dir(dir)) + "/" + filepath.Base(dir)
}

func repoDir(repo string) string {
	owner, name, _ := strings.Cut(strings.ToLower(repo), "/")
	return filepath.Join(owner, name)
}

// write stores data at a path relative to the root. It refuses to follow a
// symlink inside the cache, so a planted link cannot redirect a write.
func (s *Store) write(rel string, data []byte) error {
	if err := s.checkInside(rel); err != nil {
		return err
	}
	path := filepath.Join(s.Root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFile(path, data, 0o600)
}

func (s *Store) read(path string) ([]byte, error) {
	rel, err := filepath.Rel(s.Root, path)
	if err != nil {
		return nil, err
	}
	if err := s.checkInside(rel); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// checkInside makes sure no part of rel below the root is a symlink. The
// root itself and its parents may be links (a linked home directory is
// common); only the cache's own contents are checked.
func (s *Store) checkInside(rel string) error {
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("cache path %q is outside the cache", rel)
	}
	current := s.Root
	for _, part := range strings.Split(filepath.Clean(rel), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to use %s: it is a symbolic link inside the cache", current)
		}
	}
	return nil
}

// WriteFile writes data to a user-chosen path in one step: it writes a
// temporary file next to the target and renames it into place, so readers
// never see half a file. Missing parent folders are created, so
// --out badge/stars.json works in a fresh checkout. If the path is a
// symlink, the file it points to is replaced.
func WriteFile(path string, data []byte) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFile(path, data, 0o644)
}

func writeFile(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
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
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
