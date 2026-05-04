package api

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/utils/ptr"

	"xata/internal/signoz/filter"
	"xata/services/projects/api/spec"
	"xata/services/projects/metrics"
)

const (
	DefaultLogLimit = 100
	MaxLogLimit     = 1000

	maxLogFilters             = 16
	maxLogFilterValueLen      = 1024
	maxLogFilterValuesPerList = 100
)

var validLogLevels = map[spec.LogLevel]struct{}{
	spec.Debug:   {},
	spec.Info:    {},
	spec.Warning: {},
	spec.Error:   {},
}

func validateLogsLimit(branchID string, limit *int) error {
	if limit != nil && (*limit < 1 || *limit > MaxLogLimit) {
		return ErrorInvalidParam{BranchName: branchID, Param: "limit", Message: fmt.Sprintf("limit must be between 1 and %d", MaxLogLimit)}
	}
	return nil
}

// Without the branch scope, an authenticated user could omit their instance filter and read other branches' logs.
func compileLogsFilters(branchID string, filters []spec.LogFilter) ([]filter.Expr, error) {
	if len(filters) > maxLogFilters {
		return nil, ErrorInvalidParam{BranchName: branchID, Param: "filters", Message: fmt.Sprintf("at most %d filters are allowed", maxLogFilters)}
	}

	exprs := make([]filter.Expr, 0, len(filters)+1)
	exprs = append(exprs, branchScopeExpr(branchID))
	for i, f := range filters {
		expr, err := compileFilter(branchID, i, f)
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, expr)
	}
	return exprs, nil
}

// CNPG names cluster pods `{branchID}-{ordinal}`, so an anchored prefix match covers exactly those pods.
func branchScopeExpr(branchID string) filter.Expr {
	return filter.Regexp("k8s.pod.name", "^"+regexp.QuoteMeta(branchID)+"-")
}

func compileFilter(branchID string, idx int, f spec.LogFilter) (filter.Expr, error) {
	switch f.Field {
	case spec.Instance:
		values, err := requireInValues(branchID, idx, f)
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			if !strings.HasPrefix(v, branchID+"-") {
				return nil, ErrorInvalidParam{BranchName: branchID, Param: valuesParam(idx), Message: fmt.Sprintf("invalid instance [%s]", v)}
			}
		}
		return filter.MustIn("k8s.pod.name", values), nil

	case spec.Level:
		values, err := requireInValues(branchID, idx, f)
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			if _, ok := validLogLevels[spec.LogLevel(v)]; !ok {
				return nil, ErrorInvalidParam{BranchName: branchID, Param: valuesParam(idx), Message: fmt.Sprintf("invalid log level [%s]", v)}
			}
		}
		return filter.In("severity_text", metrics.ExpandLevels(values)), nil

	case spec.Process:
		values, err := requireInValues(branchID, idx, f)
		if err != nil {
			return nil, err
		}
		return filter.In("backend_type", values), nil

	case spec.Body:
		value, err := requireValue(branchID, idx, f)
		if err != nil {
			return nil, err
		}
		switch f.Op {
		case spec.Contains:
			return filter.Contains("body", value), nil
		case spec.Icontains:
			return filter.IContains("body", value), nil
		case spec.Regex, spec.Iregex:
			if _, err := regexp.Compile(value); err != nil {
				return nil, ErrorInvalidParam{BranchName: branchID, Param: valueParam(idx), Message: fmt.Sprintf("invalid regex: %s", err)}
			}
			if f.Op == spec.Iregex {
				return filter.Regexp("body", "(?i)"+value), nil
			}
			return filter.Regexp("body", value), nil
		default:
			return nil, ErrorInvalidParam{BranchName: branchID, Param: fmt.Sprintf("filters[%d].op", idx), Message: fmt.Sprintf("op [%s] not allowed for field [%s]", f.Op, f.Field)}
		}
	}
	return nil, ErrorInvalidParam{BranchName: branchID, Param: fmt.Sprintf("filters[%d].field", idx), Message: fmt.Sprintf("unknown field [%s]", f.Field)}
}

func requireInValues(branchID string, idx int, f spec.LogFilter) ([]string, error) {
	if f.Op != spec.In {
		return nil, ErrorInvalidParam{BranchName: branchID, Param: fmt.Sprintf("filters[%d].op", idx), Message: fmt.Sprintf("op [%s] not allowed for field [%s]", f.Op, f.Field)}
	}
	values := ptr.Deref(f.Values, nil)
	if len(values) == 0 {
		return nil, ErrorInvalidParam{BranchName: branchID, Param: valuesParam(idx), Message: "values must be non-empty for op [in]"}
	}
	if len(values) > maxLogFilterValuesPerList {
		return nil, ErrorInvalidParam{BranchName: branchID, Param: valuesParam(idx), Message: fmt.Sprintf("at most %d values are allowed", maxLogFilterValuesPerList)}
	}
	if f.Value != nil {
		return nil, ErrorInvalidParam{BranchName: branchID, Param: valueParam(idx), Message: "value must be unset for op [in]"}
	}
	return values, nil
}

func requireValue(branchID string, idx int, f spec.LogFilter) (string, error) {
	value := ptr.Deref(f.Value, "")
	if value == "" {
		return "", ErrorInvalidParam{BranchName: branchID, Param: valueParam(idx), Message: fmt.Sprintf("value must be non-empty for op [%s]", f.Op)}
	}
	if len(value) > maxLogFilterValueLen {
		return "", ErrorInvalidParam{BranchName: branchID, Param: valueParam(idx), Message: fmt.Sprintf("value must be at most %d characters", maxLogFilterValueLen)}
	}
	if len(ptr.Deref(f.Values, nil)) > 0 {
		return "", ErrorInvalidParam{BranchName: branchID, Param: valuesParam(idx), Message: fmt.Sprintf("values must be unset for op [%s]", f.Op)}
	}
	return value, nil
}

func valueParam(idx int) string  { return fmt.Sprintf("filters[%d].value", idx) }
func valuesParam(idx int) string { return fmt.Sprintf("filters[%d].values", idx) }
