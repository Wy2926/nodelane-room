package architecture

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// Build images contain only selected sources, so check the complete inventory in
// Git worktrees (including unstaged additions/deletions), not partial exports.
func TestFileIndex(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, ".git")); os.IsNotExist(err) {
		t.Skip("file inventory requires a Git worktree")
	} else if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("list maintained files: %v", err)
	}
	files := map[string]bool{}
	for name := range strings.SplitSeq(strings.TrimSuffix(string(output), "\x00"), "\x00") {
		if name == "" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name))); os.IsNotExist(err) {
			continue // git ls-files still lists tracked, unstaged deletions.
		} else if err != nil {
			t.Fatal(err)
		}
		files[name] = true
	}
	data, err := os.ReadFile(filepath.Join(root, "docs", "files.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, tree, ok := strings.Cut(strings.ReplaceAll(string(data), "\r\n", "\n"), "```text\n")
	if !ok {
		t.Fatal("docs/files.md: missing text tree")
	}
	tree, _, ok = strings.Cut(tree, "\n```")
	if !ok {
		t.Fatal("docs/files.md: unclosed text tree")
	}
	entries := map[string]bool{}
	var parents, collapsed []string
	for line := range strings.SplitSeq(tree, "\n") {
		fields := strings.Fields(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if len(fields) < 2 || indent%2 != 0 || indent/2 > len(parents) {
			t.Fatalf("docs/files.md: invalid entry %q", line)
		}
		parents = parents[:indent/2]
		directory := strings.HasSuffix(fields[0], "/")
		name := strings.TrimSuffix(fields[0], "/")
		if name == "." || name == ".." || name == "" || strings.ContainsAny(name, "/\\") {
			t.Fatalf("docs/files.md: invalid path %q", fields[0])
		}
		full := path.Join(strings.Join(parents, "/"), name)
		if _, exists := entries[full]; exists {
			t.Errorf("docs/files.md: duplicate %s", full)
		}
		entries[full] = directory
		if strings.Contains(line, "（不展开）") {
			if !directory {
				t.Errorf("docs/files.md: only directories may be collapsed: %s", full)
			}
			collapsed = append(collapsed, full)
		}
		if directory {
			parents = append(parents, name)
		}
	}
	covered := func(name string) bool {
		for _, dir := range collapsed {
			if name == dir || strings.HasPrefix(name, dir+"/") {
				return true
			}
		}
		return false
	}
	for name := range files {
		if covered(name) {
			continue
		}
		if directory, exists := entries[name]; !exists || directory {
			t.Errorf("docs/files.md: missing file %s", name)
		}
	}
	for name, directory := range entries {
		if covered(name) {
			continue
		}
		found := files[name]
		if directory {
			for file := range files {
				found = found || strings.HasPrefix(file, name+"/")
			}
		}
		if !found {
			t.Errorf("docs/files.md: obsolete entry %s", name)
		}
	}
}
