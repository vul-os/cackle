package tickets

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/vul-os/cackle"

// TestGatePathCarriesNoServerSecrets is a structural guard, not a behavioural
// test: it walks the real import graph of the two packages an offline gate is
// built from and fails if either can reach code that handles server-side
// secrets.
//
// Why this is worth a test rather than a comment. Event signing keys are now
// encrypted at rest under an operator passphrase, and the single most damaging
// way to "adopt" that would be to let any of it drift into the gate:
//
//   - a gate device holds PUBLIC keys only, and must keep verifying tickets
//     with no network and no operator input. A passphrase prompt at a venue
//     door — or a gate binary that merely contains the vault and one day gets
//     asked for a key — would break the property the whole product rests on;
//   - a scan bundle is copied onto devices that get lost, borrowed and handed
//     to volunteers. Nothing that can decrypt a signing key may be reachable
//     from the code that reads one.
//
// So: internal/tickets and internal/scan must depend on nothing inside this
// module except each other. That is a stronger statement than "no keyvault
// import", and it is the reason the check is spelled as an allowlist — a new
// dependency has to be considered here deliberately, which is exactly when
// somebody should be asking whether a gate can carry it.
//
// The walk is hermetic: it parses import declarations from source with go/ast
// and never shells out to the toolchain.
func TestGatePathCarriesNoServerSecrets(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	// The complete set of in-module packages an offline gate may contain.
	allowed := map[string]bool{
		modulePath + "/internal/tickets": true,
		modulePath + "/internal/scan":    true,
	}

	roots := []string{
		modulePath + "/internal/tickets",
		modulePath + "/internal/scan",
	}

	seen := map[string]bool{}
	var queue []string
	queue = append(queue, roots...)

	// reachedFrom records how a package was reached, so a failure names the
	// actual edge instead of just the offending package.
	reachedFrom := map[string]string{}

	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		if seen[pkg] {
			continue
		}
		seen[pkg] = true

		imports, err := inModuleImports(repoRoot, pkg)
		if err != nil {
			t.Fatalf("read imports of %s: %v", pkg, err)
		}
		for _, imp := range imports {
			if !allowed[imp] {
				t.Errorf("offline gate path leak: %s imports %s, which is not in the gate's allowlist.\n"+
					"A gate holds PUBLIC keys only and must work with no network and no operator input. "+
					"If this dependency is genuinely gate-safe, add it to `allowed` in this test and say why; "+
					"if it reaches server-side key material (internal/store, internal/store/keyvault, internal/config), it must not be there at all.",
					pkg, imp)
				continue
			}
			if !seen[imp] {
				reachedFrom[imp] = pkg
				queue = append(queue, imp)
			}
		}
	}

	// Belt and braces: name the packages that must never appear, so the test
	// fails with an unmistakable message if the allowlist above is ever
	// loosened to include one of them.
	forbidden := []string{
		modulePath + "/internal/store",
		modulePath + "/internal/store/keyvault",
		modulePath + "/internal/config",
		modulePath + "/internal/scan/substrate",
	}
	for _, f := range forbidden {
		if allowed[f] {
			t.Errorf("%s is in the gate allowlist; it must never be. A gate binary must carry no vault, no database and no engine.", f)
		}
		if seen[f] {
			t.Errorf("the gate import graph reaches %s (via %s)", f, reachedFrom[f])
		}
	}

	// A walk that silently matched nothing would pass vacuously.
	if len(seen) != len(roots) {
		var got []string
		for p := range seen {
			got = append(got, p)
		}
		sort.Strings(got)
		t.Errorf("gate import graph = %v, want exactly the two gate packages %v", got, roots)
	}
}

// inModuleImports returns the in-module packages imported by pkg's non-test
// Go files. Imports outside this module (stdlib, third-party) are ignored: the
// property under test is about this repo's own layering.
func inModuleImports(repoRoot, pkg string) ([]string, error) {
	rel := strings.TrimPrefix(pkg, modulePath+"/")
	dir := filepath.Join(repoRoot, rel)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	set := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		for _, spec := range f.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return nil, err
			}
			if path == modulePath || strings.HasPrefix(path, modulePath+"/") {
				set[path] = true
			}
		}
	}

	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}
