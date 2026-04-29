package filter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEq(t *testing.T) {
	tests := map[string]struct {
		key   string
		value string
		want  string
	}{
		"simple":           {key: "k", value: "v", want: `k = "v"`},
		"empty value":      {key: "k", value: "", want: `k = ""`},
		"embedded quote":   {key: "k", value: `a"b`, want: `k = "a\"b"`},
		"embedded slash":   {key: "k", value: `a\b`, want: `k = "a\\b"`},
		"slash then quote": {key: "k", value: `a\"b`, want: `k = "a\\\"b"`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, Eq(tc.key, tc.value).Render())
		})
	}
}

func TestIn(t *testing.T) {
	tests := map[string]struct {
		key    string
		values []string
		want   string
	}{
		"empty values skips":     {key: "k", values: nil, want: ""},
		"empty slice skips":      {key: "k", values: []string{}, want: ""},
		"single value":           {key: "k", values: []string{"a"}, want: `k IN ["a"]`},
		"multiple values":        {key: "k", values: []string{"a", "b", "c"}, want: `k IN ["a", "b", "c"]`},
		"escapes embedded quote": {key: "k", values: []string{`a"b`}, want: `k IN ["a\"b"]`},
		"escapes embedded slash": {key: "k", values: []string{`a\b`}, want: `k IN ["a\\b"]`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, In(tc.key, tc.values).Render())
		})
	}
}

func TestMustIn(t *testing.T) {
	tests := map[string]struct {
		key    string
		values []string
		want   string
	}{
		"empty values still emits": {key: "k8s.pod.name", values: nil, want: `k8s.pod.name IN []`},
		"empty slice still emits":  {key: "k", values: []string{}, want: `k IN []`},
		"single value":             {key: "k", values: []string{"a"}, want: `k IN ["a"]`},
		"multiple values":          {key: "k", values: []string{"a", "b"}, want: `k IN ["a", "b"]`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, MustIn(tc.key, tc.values).Render())
		})
	}
}

func TestContains(t *testing.T) {
	tests := map[string]struct {
		key   string
		value string
		want  string
	}{
		"simple":         {key: "body", value: "needle", want: `body CONTAINS "needle"`},
		"embedded quote": {key: "body", value: `a"b`, want: `body CONTAINS "a\"b"`},
		"embedded slash": {key: "body", value: `a\b`, want: `body CONTAINS "a\\b"`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, Contains(tc.key, tc.value).Render())
		})
	}
}

func TestIContains(t *testing.T) {
	tests := map[string]struct {
		key   string
		value string
		want  string
	}{
		"simple":              {key: "body", value: "needle", want: `body ILIKE "%needle%"`},
		"escapes percent":     {key: "body", value: "50%", want: `body ILIKE "%50\\%%"`},
		"escapes underscore":  {key: "body", value: "a_b", want: `body ILIKE "%a\\_b%"`},
		"escapes backslash":   {key: "body", value: `a\b`, want: `body ILIKE "%a\\\\b%"`},
		"escapes embedded \"": {key: "body", value: `a"b`, want: `body ILIKE "%a\"b%"`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, IContains(tc.key, tc.value).Render())
		})
	}
}

func TestRegexp(t *testing.T) {
	tests := map[string]struct {
		key   string
		value string
		want  string
	}{
		"simple":              {key: "body", value: "^err.*", want: `body REGEXP "^err.*"`},
		"caller-owned escape": {key: "body", value: `\d+`, want: `body REGEXP "\\d+"`},
		"embedded quote":      {key: "body", value: `a"b`, want: `body REGEXP "a\"b"`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, Regexp(tc.key, tc.value).Render())
		})
	}
}

func TestOr(t *testing.T) {
	tests := map[string]struct {
		exprs []Expr
		want  string
	}{
		"all empty": {
			exprs: []Expr{},
			want:  "",
		},
		"only nils and empty children skipped": {
			exprs: []Expr{nil, In("k", nil), nil},
			want:  "",
		},
		"single non-empty unwrapped": {
			exprs: []Expr{Eq("k", "v")},
			want:  `k = "v"`,
		},
		"two non-empty wrapped in parens": {
			exprs: []Expr{Eq("a", "1"), Eq("b", "2")},
			want:  `(a = "1" OR b = "2")`,
		},
		"three non-empty": {
			exprs: []Expr{Eq("a", "1"), Eq("b", "2"), Eq("c", "3")},
			want:  `(a = "1" OR b = "2" OR c = "3")`,
		},
		"skips empty child": {
			exprs: []Expr{Eq("a", "1"), In("b", nil), Eq("c", "3")},
			want:  `(a = "1" OR c = "3")`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, Or(tc.exprs...).Render())
		})
	}
}

func TestAnd(t *testing.T) {
	tests := map[string]struct {
		exprs []Expr
		want  string
	}{
		"all empty": {
			exprs: []Expr{},
			want:  "",
		},
		"only nils and empty children skipped": {
			exprs: []Expr{nil, In("k", nil), nil},
			want:  "",
		},
		"single non-empty": {
			exprs: []Expr{Eq("k", "v")},
			want:  `k = "v"`,
		},
		"two non-empty": {
			exprs: []Expr{Eq("a", "1"), Eq("b", "2")},
			want:  `a = "1" AND b = "2"`,
		},
		"skips nil child": {
			exprs: []Expr{Eq("a", "1"), nil, Eq("b", "2")},
			want:  `a = "1" AND b = "2"`,
		},
		"skips empty child": {
			exprs: []Expr{Eq("a", "1"), In("b", nil), Eq("c", "3")},
			want:  `a = "1" AND c = "3"`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, And(tc.exprs...).Render())
		})
	}
}
