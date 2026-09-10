package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Production imports only: integration tests may act as another process's client.
// Parse every platform's sources so Windows and Linux enforce the same boundaries.
func TestPackageBoundaries(t *testing.T) {
	const module = "github.com/nodelane/nodelane-room/"
	allowed := map[string][]string{
		"cmd/nlroom-cli":      {"internal/localapi", "internal/model", "internal/platform"},
		"cmd/nlroom-service":  {"internal/agent", "internal/model", "internal/platform"},
		"cmd/nlroom-node":     {"internal/agent", "internal/localapi", "internal/model", "internal/nodehost"},
		"cmd/nodelane-server": {"internal/control", "internal/model", "internal/platform"},
		"internal/agent":      {"internal/client", "internal/device", "internal/engine", "internal/game", "internal/localapi", "internal/model", "internal/pki", "internal/platform", "internal/probe"},
		"internal/client":     {"internal/device", "internal/model"},
		"internal/control":    {"internal/device", "internal/model", "internal/pki", "internal/platform"},
		"internal/device":     {"internal/model"},
		"internal/engine":     {"internal/model", "internal/pki"},
		"internal/game":       {"internal/model"},
		"internal/localapi":   {"internal/platform"},
		"internal/model":      {},
		"internal/nodehost":   {"internal/device", "internal/localapi", "internal/model"},
		"internal/pki":        {},
		"internal/platform":   {"internal/device"},
		"internal/probe":      {"internal/model"},
	}
	root := filepath.Join("..", "..")
	seen := map[string]bool{}
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "node_modules" || entry.Name() == "dist" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			pkg := filepath.ToSlash(filepath.Dir(rel))
			deps, ok := allowed[pkg]
			if !ok {
				t.Errorf("%s: define allowed imports here and the package role in docs/files.md", rel)
				return nil
			}
			seen[pkg] = true
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, spec := range file.Imports {
				imp, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return err
				}
				if strings.HasPrefix(imp, module) && !slices.Contains(deps, strings.TrimPrefix(imp, module)) {
					t.Errorf("%s: forbidden dependency on %s", rel, imp)
				}
				if (pkg == "internal/model" || pkg == "internal/device") && !strings.HasPrefix(imp, module) && strings.Contains(strings.Split(imp, "/")[0], ".") {
					t.Errorf("%s: shared data must not depend on third-party implementations: %s", rel, imp)
				}
				if imp == "github.com/jackc/pgx/v5" || strings.HasPrefix(imp, "github.com/jackc/pgx/v5/") {
					if pkg != "internal/control" {
						t.Errorf("%s: PostgreSQL belongs to internal/control", rel)
					}
				}
				if imp == "github.com/slackhq/nebula" || strings.HasPrefix(imp, "github.com/slackhq/nebula/") {
					certificateOnly := imp == "github.com/slackhq/nebula/cert" && (pkg == "internal/agent" || pkg == "internal/pki")
					if pkg != "internal/engine" && !certificateOnly {
						t.Errorf("%s: Nebula runtime belongs to internal/engine", rel)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for pkg := range allowed {
		if !seen[pkg] {
			t.Errorf("%s: remove or update the obsolete package rule", pkg)
		}
	}
}
