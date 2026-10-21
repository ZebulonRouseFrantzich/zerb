package git

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type fixture struct {
	repoDir      string
	commit       string
	rootTree     string
	baseTree     string
	selectedTree string
	gitlinks     []fixtureGitlink
	modules      string
	manifest     string
	mu           sync.Mutex
	requests     []string
	bytes        int64
}

type fixtureGitlink struct{ path, commit, locator, kind string }
type treeItem struct{ mode, name, oid string }

func buildFixture(root string) (*fixture, error) {
	repo := filepath.Join(root, "fixture", "repo.git")
	if err := os.MkdirAll(filepath.Join(repo, "objects"), 0o700); err != nil {
		return nil, err
	}
	f := &fixture{repoDir: repo}
	baseReadme, err := f.object("blob", []byte("synthetic base profile\n"))
	if err != nil {
		return nil, err
	}
	baseScript, err := f.object("blob", []byte("#!/bin/sh\nprintf '%s\\n' synthetic-base\n"))
	if err != nil {
		return nil, err
	}
	baseBin, err := f.tree([]treeItem{{"100755", "base-tool", baseScript}})
	if err != nil {
		return nil, err
	}
	f.baseTree, err = f.tree([]treeItem{{"100644", "README.txt", baseReadme}, {"040000", "bin", baseBin}})
	if err != nil {
		return nil, err
	}
	selectedFile, err := f.object("blob", []byte("synthetic selected profile\n"))
	if err != nil {
		return nil, err
	}
	f.selectedTree, err = f.tree([]treeItem{{"100644", "profile.txt", selectedFile}})
	if err != nil {
		return nil, err
	}
	f.gitlinks = []fixtureGitlink{
		{path: "profiles/unselected-https", commit: strings.Repeat("1", 40), locator: "https://gitlinks.invalid/public-unselected.git", kind: "https"},
		{path: "profiles/unselected-ssh", commit: strings.Repeat("2", 40), locator: "ssh://gitlinks.invalid/public-unselected.git", kind: "ssh"},
	}
	f.modules = "[submodule \"unselected-https\"]\n\tpath = profiles/unselected-https\n\turl = " + f.gitlinks[0].locator + "\n" +
		"[submodule \"unselected-ssh\"]\n\tpath = profiles/unselected-ssh\n\turl = " + f.gitlinks[1].locator + "\n"
	f.manifest = "schema = 1\nbase = \"profiles/base\"\nselected = \"profiles/selected\"\n"
	modulesBlob, err := f.object("blob", []byte(f.modules))
	if err != nil {
		return nil, err
	}
	manifestBlob, err := f.object("blob", []byte(f.manifest))
	if err != nil {
		return nil, err
	}
	profiles, err := f.tree([]treeItem{
		{"040000", "base", f.baseTree}, {"040000", "selected", f.selectedTree},
		{"160000", "unselected-https", f.gitlinks[0].commit}, {"160000", "unselected-ssh", f.gitlinks[1].commit},
	})
	if err != nil {
		return nil, err
	}
	f.rootTree, err = f.tree([]treeItem{{"100644", ".gitmodules", modulesBlob}, {"040000", "profiles", profiles}, {"100644", "zerb.toml", manifestBlob}})
	if err != nil {
		return nil, err
	}
	commitBody := []byte("tree " + f.rootTree + "\nauthor ZERB Proof <proof@invalid> 0 +0000\ncommitter ZERB Proof <proof@invalid> 0 +0000\n\nsynthetic deterministic proof fixture\n")
	f.commit, err = f.object("commit", commitBody)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(repo, "refs", "heads"), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(repo, "refs", "heads", "main"), []byte(f.commit+"\n"), 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(repo, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(repo, "info", "refs"), []byte(f.commit+"\trefs/heads/main\n"), 0o600); err != nil {
		if err := os.MkdirAll(filepath.Join(repo, "info"), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(repo, "info", "refs"), []byte(f.commit+"\trefs/heads/main\n"), 0o600); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func (f *fixture) tree(items []treeItem) (string, error) {
	sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })
	var b bytes.Buffer
	for _, item := range items {
		oid, err := hex.DecodeString(item.oid)
		if err != nil || len(oid) != sha1.Size {
			return "", fmt.Errorf("invalid fixture object %q", item.name)
		}
		fmt.Fprintf(&b, "%s %s%c", item.mode, item.name, byte(0))
		b.Write(oid)
	}
	return f.object("tree", b.Bytes())
}

func (f *fixture) object(kind string, body []byte) (string, error) {
	raw := append([]byte(fmt.Sprintf("%s %d%c", kind, len(body), byte(0))), body...)
	sum := sha1.Sum(raw)
	oid := hex.EncodeToString(sum[:])
	dir := filepath.Join(f.repoDir, "objects", oid[:2])
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	file, err := os.OpenFile(filepath.Join(dir, oid[2:]), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	zw := zlib.NewWriter(file)
	_, writeErr := zw.Write(raw)
	closeErr := zw.Close()
	fileErr := file.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if fileErr != nil {
		return "", fileErr
	}
	return oid, nil
}

func (f *fixture) serve(ctx context.Context) (string, func() error, error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/stall.git/") {
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
			return
		}
		if strings.HasPrefix(r.URL.Path, "/high-rate.git/") {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			chunk := bytes.Repeat([]byte{'x'}, 64<<10)
			for range 64 {
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
			return
		}
		if strings.HasPrefix(r.URL.Path, "/redirect.git/") {
			w.Header().Set("Location", "/info/refs")
			w.WriteHeader(http.StatusFound)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/auth.git/") {
			w.Header().Set("WWW-Authenticate", `Basic realm="proof"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if clean == "." || strings.HasPrefix(clean, "..") || strings.Contains(clean, "\\") {
			http.Error(w, "invalid", http.StatusBadRequest)
			return
		}
		p := filepath.Join(f.repoDir, filepath.FromSlash(clean))
		if !within(f.repoDir, p) {
			http.NotFound(w, r)
			return
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		f.requests = append(f.requests, "/"+clean)
		f.bytes += int64(len(data))
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	})
	srv := &http.Server{Handler: h, ReadHeaderTimeout: MaxWallTime}
	go func() { <-ctx.Done(); _ = srv.Close() }()
	go func() { _ = srv.Serve(ln) }()
	return "http://" + ln.Addr().String(), srv.Close, nil
}

func (f *fixture) transferObservation() (int64, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bytes, append([]string(nil), f.requests...)
}
