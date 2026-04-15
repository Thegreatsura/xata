package serverless

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertTextValue(t *testing.T) {
	tests := map[string]struct {
		raw     []byte
		typeOID uint32
		want    any
	}{
		"nil": {
			raw:     nil,
			typeOID: 0,
			want:    nil,
		},
		"bool true": {
			raw:     []byte("t"),
			typeOID: oidBool,
			want:    true,
		},
		"bool false": {
			raw:     []byte("f"),
			typeOID: oidBool,
			want:    false,
		},
		"int2": {
			raw:     []byte("42"),
			typeOID: oidInt2,
			want:    42,
		},
		"int4": {
			raw:     []byte("12345"),
			typeOID: oidInt4,
			want:    12345,
		},
		"int4 negative": {
			raw:     []byte("-99"),
			typeOID: oidInt4,
			want:    -99,
		},
		"int8 large": {
			raw:     []byte("9007199254740993"),
			typeOID: oidInt8,
			want:    "9007199254740993",
		},
		"int8 zero": {
			raw:     []byte("0"),
			typeOID: oidInt8,
			want:    "0",
		},
		"int8 negative": {
			raw:     []byte("-9007199254740993"),
			typeOID: oidInt8,
			want:    "-9007199254740993",
		},
		"float4 normal": {
			raw:     []byte("3.14"),
			typeOID: oidFloat4,
			want:    3.14,
		},
		"float8 normal": {
			raw:     []byte("2.718281828"),
			typeOID: oidFloat8,
			want:    2.718281828,
		},
		"float8 NaN": {
			raw:     []byte("NaN"),
			typeOID: oidFloat8,
			want:    "NaN",
		},
		"float4 Infinity": {
			raw:     []byte("Infinity"),
			typeOID: oidFloat4,
			want:    "Infinity",
		},
		"float8 -Infinity": {
			raw:     []byte("-Infinity"),
			typeOID: oidFloat8,
			want:    "-Infinity",
		},
		"json": {
			raw:     []byte(`{"key":"value"}`),
			typeOID: oidJSON,
			want:    json.RawMessage(`{"key":"value"}`),
		},
		"jsonb": {
			raw:     []byte(`[1,2,3]`),
			typeOID: oidJSONB,
			want:    json.RawMessage(`[1,2,3]`),
		},
		"text": {
			raw:     []byte("hello world"),
			typeOID: 25, // TEXT
			want:    "hello world",
		},
		"timestamp": {
			raw:     []byte("2024-01-15 10:30:00"),
			typeOID: 1114, // TIMESTAMP
			want:    "2024-01-15 10:30:00",
		},
		"uuid": {
			raw:     []byte("550e8400-e29b-41d4-a716-446655440000"),
			typeOID: 2950, // UUID
			want:    "550e8400-e29b-41d4-a716-446655440000",
		},
		"numeric": {
			raw:     []byte("123456.789"),
			typeOID: 1700, // NUMERIC
			want:    "123456.789",
		},
		"bool array via array type OID": {
			raw:     []byte("{t,f,t}"),
			typeOID: 1000, // _bool
			want:    []any{true, false, true},
		},
		"int4 array via array type OID": {
			raw:     []byte("{1,2,3}"),
			typeOID: 1007, // _int4
			want:    []any{1, 2, 3},
		},
		"text array via array type OID": {
			raw:     []byte("{hello,world}"),
			typeOID: 1009, // _text
			want:    []any{"hello", "world"},
		},
		"jsonb array via array type OID": {
			raw:     []byte(`{"{\"a\":1}","{\"b\":2}"}`),
			typeOID: 3807, // _jsonb
			want:    []any{json.RawMessage(`{"a":1}`), json.RawMessage(`{"b":2}`)},
		},
		"invalid array falls back to string": {
			raw:     []byte("not-an-array"),
			typeOID: 1007, // _int4
			want:    "not-an-array",
		},
		"int2 invalid falls back to string": {
			raw:     []byte("abc"),
			typeOID: oidInt2,
			want:    "abc",
		},
		"float4 unparseable falls back to string": {
			raw:     []byte("not-a-float"),
			typeOID: oidFloat4,
			want:    "not-a-float",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := convertTextValue(tc.raw, tc.typeOID)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParsePgArray(t *testing.T) {
	tests := map[string]struct {
		raw        []byte
		elemOID    uint32
		want       []any
		wantErrStr string
	}{
		"empty array": {
			raw:     []byte("{}"),
			elemOID: oidInt4,
			want:    nil,
		},
		"int array": {
			raw:     []byte("{1,2,3}"),
			elemOID: oidInt4,
			want:    []any{1, 2, 3},
		},
		"text array": {
			raw:     []byte("{hello,world}"),
			elemOID: 0,
			want:    []any{"hello", "world"},
		},
		"NULL elements": {
			raw:     []byte("{1,NULL,3}"),
			elemOID: oidInt4,
			want:    []any{1, nil, 3},
		},
		"quoted strings": {
			raw:     []byte(`{"hello world","foo bar"}`),
			elemOID: 0,
			want:    []any{"hello world", "foo bar"},
		},
		"escaped quotes": {
			raw:     []byte(`{"say \"hi\"","back\\slash"}`),
			elemOID: 0,
			want:    []any{`say "hi"`, `back\slash`},
		},
		"nested int arrays": {
			raw:     []byte("{{1,2},{3,4}}"),
			elemOID: oidInt4,
			want:    []any{[]any{1, 2}, []any{3, 4}},
		},
		"bool array": {
			raw:     []byte("{t,f,t}"),
			elemOID: oidBool,
			want:    []any{true, false, true},
		},
		"int8 array": {
			raw:     []byte("{9007199254740993,42}"),
			elemOID: oidInt8,
			want:    []any{"9007199254740993", "42"},
		},
		"float array": {
			raw:     []byte("{1.5,NaN,Infinity}"),
			elemOID: oidFloat8,
			want:    []any{1.5, "NaN", "Infinity"},
		},
		"with bounds decoration": {
			raw:     []byte("[1:3]={10,20,30}"),
			elemOID: oidInt4,
			want:    []any{10, 20, 30},
		},
		"invalid literal": {
			raw:        []byte("not-an-array"),
			elemOID:    oidInt4,
			wantErrStr: "invalid array literal",
		},
		"invalid bounds decoration": {
			raw:        []byte("[1:3{10,20,30}"),
			elemOID:    oidInt4,
			wantErrStr: "invalid array bounds",
		},
		"json array": {
			raw:     []byte(`{"{\"a\":1}","{\"b\":2}"}`),
			elemOID: oidJSONB,
			want:    []any{json.RawMessage(`{"a":1}`), json.RawMessage(`{"b":2}`)},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parsePgArray(tc.raw, tc.elemOID)
			if tc.wantErrStr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErrStr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestConvertParams(t *testing.T) {
	tests := map[string]struct {
		input []any
		want  []any
	}{
		"nil params": {
			input: nil,
			want:  nil,
		},
		"empty params": {
			input: []any{},
			want:  []any{},
		},
		"scalars unchanged": {
			input: []any{nil, "hello", float64(42), true},
			want:  []any{nil, "hello", float64(42), true},
		},
		"array converted": {
			input: []any{[]any{float64(1), float64(2), float64(3)}},
			want:  []any{"{1,2,3}"},
		},
		"object converted to JSON": {
			input: []any{map[string]any{"key": "value"}},
			want:  []any{`{"key":"value"}`},
		},
		"mixed params": {
			input: []any{"text", []any{float64(1), float64(2)}, map[string]any{"a": float64(1)}},
			want:  []any{"text", "{1,2}", `{"a":1}`},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := convertParams(tc.input)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestFormatPgArray(t *testing.T) {
	tests := map[string]struct {
		input []any
		want  string
	}{
		"simple ints": {
			input: []any{float64(1), float64(2), float64(3)},
			want:  "{1,2,3}",
		},
		"nested": {
			input: []any{[]any{float64(1), float64(2)}, []any{float64(3), float64(4)}},
			want:  "{{1,2},{3,4}}",
		},
		"with strings": {
			input: []any{"hello", "world"},
			want:  `{"hello","world"}`,
		},
		"with NULL": {
			input: []any{float64(1), nil, float64(3)},
			want:  "{1,NULL,3}",
		},
		"with booleans": {
			input: []any{true, false, true},
			want:  "{t,f,t}",
		},
		"string with quotes": {
			input: []any{`say "hi"`},
			want:  `{"say \"hi\""}`,
		},
		"empty": {
			input: []any{},
			want:  "{}",
		},
		"json.Number": {
			input: []any{json.Number("42"), json.Number("3.14")},
			want:  "{42,3.14}",
		},
		"string with backslash": {
			input: []any{`back\slash`},
			want:  `{"back\\slash"}`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := formatPgArray(tc.input)
			require.Equal(t, tc.want, got)
		})
	}
}
