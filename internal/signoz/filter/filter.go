// Package filter builds SigNoz filter expressions in a composable, escape-safe way.
package filter

import "strings"

// Expr is a renderable SigNoz filter clause. Render returns the empty string
// when the clause should be omitted.
type Expr interface {
	Render() string
}

type rendered string

func (r rendered) Render() string { return string(r) }

// Eq renders `key = "value"` with the value safely escaped.
func Eq(key, value string) Expr {
	return rendered(key + " = " + escapeString(value))
}

// In renders `key IN ["v1", "v2", ...]`. Returns an empty expression when
// values is empty so callers can skip the clause.
func In(key string, values []string) Expr {
	if len(values) == 0 {
		return rendered("")
	}
	return rendered(key + " IN [" + joinEscaped(values) + "]")
}

// MustIn renders `key IN ["v1", ...]` and always emits. SigNoz rejects an
// empty `IN []` as a syntax error, so callers must guarantee at least one
// value; the failure mode is fail-loud (query 400), never silent match-all.
func MustIn(key string, values []string) Expr {
	return rendered(key + " IN [" + joinEscaped(values) + "]")
}

// Contains renders `key CONTAINS "value"`.
func Contains(key, value string) Expr {
	return rendered(key + " CONTAINS " + escapeString(value))
}

// IContains renders `key ILIKE "%escaped-value%"` with `%`, `_` and `\`
// escaped in the value so the user's input is matched literally.
func IContains(key, value string) Expr {
	return rendered(key + " ILIKE " + escapeString("%"+escapeLikePattern(value)+"%"))
}

// Regexp renders `key REGEXP "value"`. The pattern is the caller's
// responsibility; only `\` and `"` are escaped for the SigNoz string literal.
func Regexp(key, value string) Expr {
	return rendered(key + " REGEXP " + escapeString(value))
}

// And joins non-nil children whose Render returns a non-empty string with
// ` AND `. With zero non-empty children, returns an empty expression.
func And(exprs ...Expr) Expr {
	parts := make([]string, 0, len(exprs))
	for _, e := range exprs {
		if e == nil {
			continue
		}
		s := e.Render()
		if s == "" {
			continue
		}
		parts = append(parts, s)
	}
	return rendered(strings.Join(parts, " AND "))
}

// Or joins non-empty children with ` OR ` and wraps the result in parens so
// it composes safely inside an outer And. With one non-empty child the parens
// are omitted; with zero, returns the empty expression.
func Or(exprs ...Expr) Expr {
	parts := make([]string, 0, len(exprs))
	for _, e := range exprs {
		if e == nil {
			continue
		}
		s := e.Render()
		if s == "" {
			continue
		}
		parts = append(parts, s)
	}
	switch len(parts) {
	case 0:
		return rendered("")
	case 1:
		return rendered(parts[0])
	default:
		return rendered("(" + strings.Join(parts, " OR ") + ")")
	}
}

func joinEscaped(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = escapeString(v)
	}
	return strings.Join(quoted, ", ")
}

func escapeString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
