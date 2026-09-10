package api_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// pathsNotRoutedYet: every entry is a bug waiting to be reported.
var pathsNotRoutedYet = map[string]string{}

// charts/auth/values.yaml is an allow-list: a path missing from it 404s at the
// gateway, with no application log and nothing in traces.
func TestEveryAuthPathIsRouted(t *testing.T) {
	root := repoRoot(t)

	matchers := routeMatchers(t, filepath.Join(root, "charts", "auth", "values.yaml"))
	require.NotEmpty(t, matchers, "the auth chart declares no route matches")

	for _, path := range authSpecPaths(t, filepath.Join(root, "openapi", "gen", "bundled", "auth.yaml")) {
		if reason, known := pathsNotRoutedYet[path]; known {
			t.Logf("skipping %s: %s", path, reason)
			continue
		}
		t.Run(path, func(t *testing.T) {
			concrete := templateParams.ReplaceAllString(path, "placeholder")
			require.Truef(t, anyMatch(matchers, concrete),
				"%s is served by auth but no entry in charts/auth/values.yaml httpRoute.matches accepts %s, "+
					"so the gateway will 404 it with no application log", path, concrete)
		})
	}
}

var templateParams = regexp.MustCompile(`\{[^}]+\}`)

type routeMatcher func(string) bool

func routeMatchers(t *testing.T, valuesPath string) []routeMatcher {
	t.Helper()

	raw, err := os.ReadFile(valuesPath)
	require.NoError(t, err)

	var values struct {
		HTTPRoute struct {
			Matches []struct {
				Path string `yaml:"path"`
				Type string `yaml:"type"`
			} `yaml:"matches"`
		} `yaml:"httpRoute"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &values))

	matchers := make([]routeMatcher, 0, len(values.HTTPRoute.Matches))
	for _, match := range values.HTTPRoute.Matches {
		switch match.Type {
		case "RegularExpression":
			// Envoy's safe_regex is RE2 and matches the whole path, so a
			// partial match here would be a false pass.
			re, err := regexp.Compile(match.Path)
			require.NoErrorf(t, err, "route regex %q does not compile", match.Path)
			matchers = append(matchers, func(p string) bool { return re.FindString(p) == p })
		case "PathPrefix":
			prefix := match.Path
			matchers = append(matchers, func(p string) bool {
				return p == prefix || strings.HasPrefix(p, strings.TrimSuffix(prefix, "/")+"/")
			})
		default:
			exact := match.Path
			matchers = append(matchers, func(p string) bool { return p == exact })
		}
	}
	return matchers
}

func anyMatch(matchers []routeMatcher, path string) bool {
	for _, match := range matchers {
		if match(path) {
			return true
		}
	}
	return false
}

// authSpecPaths reads auth's own bundled spec, not the joined public one, which
// also carries the projects service's /organizations/... paths.
func authSpecPaths(t *testing.T, specPath string) []string {
	t.Helper()

	raw, err := os.ReadFile(specPath)
	require.NoError(t, err)

	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &doc))

	methods := []string{"get", "put", "post", "delete", "patch"}
	authPaths := make([]string, 0)
	for path, item := range doc.Paths {
		if !strings.HasPrefix(path, "/organizations") {
			continue
		}
		for _, method := range methods {
			if _, ok := item[method]; ok {
				authPaths = append(authPaths, path)
				break
			}
		}
	}
	require.NotEmpty(t, authPaths, "no organization paths found in %s", specPath)
	return authPaths
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "could not find the repository root")
		dir = parent
	}
}
