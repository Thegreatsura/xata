package serverless

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"

	"xata/services/gateway/serverless/spec"
	"xata/services/gateway/session"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestPgErrorClassification(t *testing.T) {
	tests := map[string]struct {
		codes string
		want  string
	}{
		"upstream protocol":     {"08P01", errTypePgErrorUpstream},
		"unknown outcome":       {"08007 40003", errTypePgErrorInternal},
		"privileges":            {"42501", errTypePgErrorAccess},
		"request overrides":     {"23502 23514 57014 P0001 P0002 P0003", errTypePgErrorRequest},
		"unavailable overrides": {"57P01 57P02 57P03 57P04 57P05", errTypePgErrorAvailable},
		"availability classes":  {"08000 08001 08006 53000 53100 53200 53300 53400", errTypePgErrorAvailable},
		"authorization class":   {"28000 28P01", errTypePgErrorAccess},
		"conflict classes":      {"23000 23503 23505 23P01 40000 40001 40P01 55000 55P03", errTypePgErrorConflict},
		"request classes": {
			"03000 09000 0A000 0B000 0F000 0L000 0P000 0Z000 10000 20000 " +
				"21000 22000 24000 25000 26000 27000 2B000 2D000 2F000 34000 " +
				"38000 39000 3B000 3D000 3F000 42000 44000 54000",
			errTypePgErrorRequest,
		},
		"server and unknown classes":               {"57000 58000 F0000 HV000 XX000 XX001 P0000 P0004 ZZ000", errTypePgErrorInternal},
		"non-error classes passed to error helper": {"00000 01000 02000", errTypePgErrorInternal},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			for code := range strings.FieldsSeq(tc.codes) {
				t.Run(code, func(t *testing.T) {
					got := pgErrorClassification(code)
					require.Equal(t, tc.want, got)
				})
			}
		})
	}
	for name, code := range map[string]string{
		"empty": "", "short": "0800", "long": "080000", "lowercase": "08p01",
		"punctuation": "08!01", "space": "08 01", "unicode": "08é1",
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, errTypePgErrorInternal, pgErrorClassification(code))
		})
	}
}

func TestHandlePgErrorPreservesFields(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code: "23505", Message: "duplicate key", Severity: "ERROR", Detail: "key already exists",
		Hint: "choose another key", Position: 7, InternalPosition: 2, InternalQuery: "INSERT",
		Where: "function", SchemaName: "public", TableName: "items", ColumnName: "id",
		DataTypeName: "int4", ConstraintName: "items_pkey", File: "index.c", Line: 42, Routine: "insert",
	}
	for name, err := range map[string]error{
		"direct":            pgErr,
		"wrapped":           fmt.Errorf("query: %w", pgErr),
		"during connection": &connectionEstablishmentError{err: pgErr},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/sql", nil), rec)
			require.NoError(t, handlePgError(c, err))
			require.Equal(t, http.StatusBadRequest, rec.Code)
			var got spec.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			want := pgErrorResponse(pgErr)
			require.Equal(t, *want, got)
			require.Equal(t, "23505", *got.Code)
			require.Empty(t, rec.Header().Get("Retry-After"))
		})
	}
}

func TestConnectionEstablishmentErrorPrecedence(t *testing.T) {
	tests := map[string]struct {
		err        error
		wantType   string
		wantStatus int
		wantCode   string
	}{
		"refused":        {syscall.ECONNREFUSED, errTypeConnection, http.StatusServiceUnavailable, "08001"},
		"deadline":       {context.DeadlineExceeded, errTypeTimeout, http.StatusGatewayTimeout, "GATEWAY_TIMEOUT"},
		"canceled":       {context.Canceled, errTypeCanceled, http.StatusOK, ""},
		"query timeout":  {ErrQueryExecTimeout, errTypeQueryTimeout, http.StatusBadRequest, "QUERY_TIMEOUT"},
		"hibernated":     {session.ErrBranchHibernated, errTypeHibernated, http.StatusConflict, "BRANCH_HIBERNATED"},
		"missing branch": {session.ErrBranchNotFound, errTypeBranchNotFound, http.StatusNotFound, "BRANCH_NOT_FOUND"},
		"authentication": {&pgconn.PgError{Code: "28P01"}, errTypePgErrorAccess, http.StatusBadRequest, "28P01"},
		"credentials":    {errors.New("postgres://user:secret@localhost/db"), errTypeConnection, http.StatusServiceUnavailable, "08001"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := fmt.Errorf("request: %w", &connectionEstablishmentError{err: tc.err})
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.wantType, classifyError(err))
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/sql", nil), rec)
			require.NoError(t, handlePgError(c, err))
			require.Equal(t, tc.wantStatus, rec.Code)
			if tc.wantCode == "" {
				require.Empty(t, rec.Body.String())
				return
			}
			var got spec.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.NotNil(t, got.Code)
			require.Equal(t, tc.wantCode, *got.Code)
			require.NotContains(t, got.Message, "secret")
		})
	}
}

func TestConnectTracedLeavesAddressErrorsUnwrapped(t *testing.T) {
	h := &handler{tracer: tracenoop.NewTracerProvider().Tracer("test")}
	_, err := h.connectTraced(context.Background(), &session.Branch{Address: "missing-port"}, &connectionInfo{})
	require.Error(t, err)
	var got *connectionEstablishmentError
	require.False(t, errors.As(err, &got))
	require.ErrorContains(t, err, "split branch address")
	require.Equal(t, errTypeOther, classifyError(err))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/sql", nil), rec)
	require.NoError(t, handlePgError(c, err))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
