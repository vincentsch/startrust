package cache

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSaveListAndLatest(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	older := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	newer := older.Add(48 * time.Hour)
	for _, at := range []time.Time{older, newer} {
		if _, err := store.SaveCheck("Owner/Repo", at, []byte(`{"at":"`+at.String()+`"}`)); err != nil {
			t.Fatal(err)
		}
	}
	store.SaveCheck("other/repo", older, []byte(`{}`))

	all, err := store.Checks("")
	if err != nil || len(all) != 3 {
		t.Fatalf("checks = %+v, %v", all, err)
	}
	mine, _ := store.Checks("owner/REPO") // names are case-insensitive
	if len(mine) != 2 || !mine[0].CheckedAt.Equal(newer) || mine[0].Repository != "owner/repo" {
		t.Fatalf("checks = %+v", mine)
	}
	data, entry, err := store.LatestCheck("owner/repo")
	if err != nil || !strings.Contains(string(data), "2026-10-03") || !entry.CheckedAt.Equal(newer) {
		t.Fatalf("latest = %s %+v %v", data, entry, err)
	}
}

func TestPruneKeepsNewestOfEachRepo(t *testing.T) {
	store, _ := Open(t.TempDir())
	now := time.Now().UTC()
	store.SaveCheck("a/b", now.AddDate(0, -3, 0), []byte(`{}`))
	store.SaveCheck("a/b", now.AddDate(0, -2, 0), []byte(`{}`))
	store.SaveCheck("c/d", now.AddDate(0, -3, 0), []byte(`{}`))

	plan, err := store.Prune(now.AddDate(0, -1, 0), true)
	if err != nil || plan.Files != 1 || plan.Kept != 2 {
		t.Fatalf("dry run plan = %+v, %v", plan, err)
	}
	if left, _ := store.Checks(""); len(left) != 3 {
		t.Fatalf("dry run deleted files")
	}
	if _, err := store.Prune(now.AddDate(0, -1, 0), false); err != nil {
		t.Fatal(err)
	}
	left, _ := store.Checks("")
	if len(left) != 2 {
		t.Fatalf("left = %+v", left)
	}
}

func TestPruneNamesTheRepoOfEveryFile(t *testing.T) {
	// The JSON of a prune lists every file with its repo, also the GH
	// Archive answers and unfinished stargazer lists, in UTC like the checks.
	store, _ := Open(t.TempDir())
	store.Save("accounts", "Owner/Some_Repo", "2025-02-10", struct{}{})
	store.SaveProgress("owner/some__repo", struct{}{})
	plan, err := store.Prune(time.Now().Add(time.Hour), true)
	if err != nil || len(plan.Items) != 2 {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	got := map[string]bool{}
	for _, item := range plan.Items {
		got[item.Repository] = true
		if item.CheckedAt.Location() != time.UTC || item.CheckedAt.Nanosecond() != 0 {
			t.Errorf("time %v should be UTC whole seconds", item.CheckedAt)
		}
	}
	if !got["owner/some_repo"] || !got["owner/some__repo"] {
		t.Errorf("repos = %v", got)
	}
}

func TestValuesAndProgress(t *testing.T) {
	store, _ := Open(t.TempDir())
	type value struct{ N int }
	if store.Load("accounts", "a/b", "2025-02-10", &value{}) {
		t.Fatal("found a value that was never saved")
	}
	store.Save("accounts", "a/b", "2025-02-10", value{N: 7})
	var got value
	if !store.Load("accounts", "A/B", "2025-02-10", &got) || got.N != 7 {
		t.Fatalf("got %+v", got)
	}
	store.SaveProgress("a/b", value{N: 1})
	store.SaveProgress("a/b", value{N: 2}) // overwritten, not added
	if !store.LoadProgress("a/b", &got) || got.N != 2 {
		t.Fatalf("progress = %+v", got)
	}
	store.ClearProgress("a/b")
	if store.LoadProgress("a/b", &got) {
		t.Fatal("progress not cleared")
	}
}

func TestSymlinkInsideCacheIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra rights on Windows")
	}
	root := t.TempDir()
	store, _ := Open(root)
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(root, "checks")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveCheck("a/b", time.Now(), []byte(`{}`)); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("error = %v, want a refusal", err)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatal("wrote through the symlink")
	}
}

func TestCacheRootMayBeASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra rights on Windows")
	}
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "linked-cache")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	store, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveCheck("a/b", time.Now(), []byte(`{}`)); err != nil {
		t.Fatalf("a linked cache root (common with linked home directories) failed: %v", err)
	}
}

func TestWriteFileFollowsUserSymlinksAndIsShareable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.md")
	path := target
	if runtime.GOOS != "windows" {
		path = filepath.Join(dir, "link.md")
		os.WriteFile(target, []byte("old"), 0o644)
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteFile(path, []byte("new report")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "new report" {
		t.Fatalf("target holds %q", data)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(target)
		if info.Mode().Perm() != 0o644 {
			t.Errorf("mode %v, want 0644 so the report can be shared", info.Mode().Perm())
		}
		if fi, _ := os.Lstat(path); fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("the user's symlink was replaced")
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".*.tmp"))
	if len(leftovers) > 0 {
		t.Errorf("temporary files left: %v", leftovers)
	}
}

func TestOpenCreatesPrivateDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permissions")
	}
	root := filepath.Join(t.TempDir(), "new", "cache")
	if _, err := Open(root); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(root)
	if info.Mode().Perm() != 0o700 {
		t.Errorf("mode %v, want 0700", info.Mode().Perm())
	}
}

func TestWriteFileCreatesMissingFolders(t *testing.T) {
	// The Action writes badge/stars.json in a fresh checkout, where the
	// badge folder does not exist yet.
	path := filepath.Join(t.TempDir(), "badge", "stars.json")
	if err := WriteFile(path, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "{}" {
		t.Errorf("read %q, %v", data, err)
	}
}
