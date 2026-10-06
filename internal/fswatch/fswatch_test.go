package fswatch

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestWatchReportsRelativePaths(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("FSEvents only")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []string
	w, err := Watch(root, 50*time.Millisecond, func(c Change) {
		mu.Lock()
		got = append(got, c.Paths...)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // let the stream settle
	if err := os.WriteFile(filepath.Join(root, "src", "deep", "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "src", "deep", "a.txt"), filepath.Join(root, "b.txt")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := slices.Contains(got, "src/deep/a.txt") && slices.Contains(got, "b.txt")
		mu.Unlock()
		if ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	w.Close()
	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(got, "src/deep/a.txt") || !slices.Contains(got, "b.txt") {
		t.Fatalf("got %v", got)
	}
	// Nothing arrives after Close.
	n := len(got)
	mu.Unlock()
	_ = os.WriteFile(filepath.Join(root, "c.txt"), []byte("x"), 0o644)
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	if len(got) != n {
		t.Fatalf("events after Close: %v", got[n:])
	}
}

func TestWatchDirsReportsFolders(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("FSEvents only")
	}
	var roots []string
	for range 2 {
		r, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		roots = append(roots, r)
	}
	if err := os.MkdirAll(filepath.Join(roots[1], "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	got := map[string]bool{}
	w, err := WatchDirs(roots, 30*time.Millisecond, func(dirs []string, rescan bool) {
		mu.Lock()
		for _, d := range dirs {
			got[d] = true
		}
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	time.Sleep(200 * time.Millisecond) // let the stream settle
	if err := os.WriteFile(filepath.Join(roots[0], "HEAD"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roots[1], "sub", "a"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := []string{roots[0], filepath.Join(roots[1], "sub")}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := got[want[0]] && got[want[1]]
		mu.Unlock()
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("want %v reported, got %v", want, got)
}
