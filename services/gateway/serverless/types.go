package serverless

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// PostgreSQL type OIDs
const (
	oidBool   = 16
	oidInt2   = 21
	oidInt4   = 23
	oidInt8   = 20
	oidFloat4 = 700
	oidFloat8 = 701
	oidJSON   = 114
	oidJSONB  = 3802
)

// arrayElemOID maps PostgreSQL array type OIDs to their element type OIDs.
var arrayElemOID = map[uint32]uint32{
	1000: oidBool,   // _bool
	1005: oidInt2,   // _int2
	1007: oidInt4,   // _int4
	1016: oidInt8,   // _int8
	1021: oidFloat4, // _float4
	1022: oidFloat8, // _float8
	199:  oidJSON,   // _json
	3807: oidJSONB,  // _jsonb
	1009: 0,         // _text  (element OID 0 = default string)
	1015: 0,         // _varchar
	1231: 0,         // _numeric
}

// convertTextValue converts a raw text-format PostgreSQL value to the appropriate
// Go type for JSON serialization.
func convertTextValue(raw []byte, typeOID uint32) any {
	if raw == nil {
		return nil
	}

	text := string(raw)

	switch typeOID {
	case oidBool:
		return text == "t"

	case oidInt2, oidInt4:
		if n, err := strconv.Atoi(text); err == nil {
			return n
		}
		return text

	case oidInt8:
		return text

	case oidFloat4, oidFloat8:
		return convertFloat(text)

	case oidJSON, oidJSONB:
		cp := make([]byte, len(raw))
		copy(cp, raw)
		return json.RawMessage(cp)

	default:
		if elemOID, ok := arrayElemOID[typeOID]; ok {
			arr, err := parsePgArray(raw, elemOID)
			if err != nil {
				return text
			}
			return arr
		}
		return text
	}
}

// convertFloat converts a PostgreSQL text-format float to a Go value.
// NaN and Infinity are returned as strings since JSON does not support them.
func convertFloat(text string) any {
	switch text {
	case "NaN":
		return "NaN"
	case "Infinity":
		return "Infinity"
	case "-Infinity":
		return "-Infinity"
	}
	if f, err := strconv.ParseFloat(text, 64); err == nil {
		return f
	}
	return text
}

// parsePgArray parses a PostgreSQL text-format array literal into a Go slice.
// It handles nested arrays, quoted strings with escapes, NULL elements,
// and optional bounds decoration (e.g., [1:3]={1,2,3}).
func parsePgArray(raw []byte, elemTypeOID uint32) ([]any, error) {
	s := string(raw)

	// Strip optional bounds decoration: [1:3]={1,2,3} → {1,2,3}
	if len(s) > 0 && s[0] == '[' {
		idx := strings.Index(s, "=")
		if idx < 0 {
			return nil, fmt.Errorf("invalid array bounds: %s", s)
		}
		s = s[idx+1:]
	}

	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return nil, fmt.Errorf("invalid array literal: %s", string(raw))
	}

	result, _, err := parsePgArrayInner(s, 0, elemTypeOID)
	return result, err
}

func parsePgArrayInner(s string, pos int, elemTypeOID uint32) ([]any, int, error) {
	if pos >= len(s) || s[pos] != '{' {
		return nil, pos, fmt.Errorf("expected '{' at position %d", pos)
	}
	pos++ // skip '{'

	var result []any

	for pos < len(s) {
		if s[pos] == '}' {
			return result, pos + 1, nil
		}

		if len(result) > 0 {
			if pos >= len(s) || s[pos] != ',' {
				return nil, pos, fmt.Errorf("expected ',' at position %d", pos)
			}
			pos++ // skip ','
		}

		if pos >= len(s) {
			return nil, pos, fmt.Errorf("unexpected end of array")
		}

		// Nested array
		if s[pos] == '{' {
			nested, newPos, err := parsePgArrayInner(s, pos, elemTypeOID)
			if err != nil {
				return nil, newPos, err
			}
			result = append(result, nested)
			pos = newPos
			continue
		}

		// Quoted element
		if s[pos] == '"' {
			pos++ // skip opening quote
			var buf strings.Builder
			for pos < len(s) {
				if s[pos] == '\\' && pos+1 < len(s) {
					buf.WriteByte(s[pos+1])
					pos += 2
					continue
				}
				if s[pos] == '"' {
					pos++ // skip closing quote
					break
				}
				buf.WriteByte(s[pos])
				pos++
			}
			result = append(result, convertTextValue([]byte(buf.String()), elemTypeOID))
			continue
		}

		// Unquoted element (NULL or plain value)
		end := pos
		for end < len(s) && s[end] != ',' && s[end] != '}' {
			end++
		}
		token := s[pos:end]
		pos = end

		if token == "NULL" {
			result = append(result, nil)
		} else {
			result = append(result, convertTextValue([]byte(token), elemTypeOID))
		}
	}

	return nil, pos, fmt.Errorf("unterminated array")
}

// convertParams preprocesses query parameters before sending them to PostgreSQL.
// Slices are converted to PG array literals and maps are converted to JSON strings.
func convertParams(params []any) []any {
	if len(params) == 0 {
		return params
	}

	out := make([]any, len(params))
	for i, p := range params {
		switch v := p.(type) {
		case []any:
			out[i] = formatPgArray(v)
		case map[string]any:
			b, err := json.Marshal(v)
			if err != nil {
				out[i] = v
			} else {
				out[i] = string(b)
			}
		default:
			out[i] = v
		}
	}
	return out
}

// formatPgArray formats a Go slice as a PostgreSQL array literal string, e.g. {1,2,3}.
func formatPgArray(arr []any) string {
	var buf strings.Builder
	buf.WriteByte('{')
	for i, elem := range arr {
		if i > 0 {
			buf.WriteByte(',')
		}
		switch v := elem.(type) {
		case nil:
			buf.WriteString("NULL")
		case []any:
			buf.WriteString(formatPgArray(v))
		case bool:
			if v {
				buf.WriteString("t")
			} else {
				buf.WriteString("f")
			}
		case string:
			buf.WriteByte('"')
			for _, c := range []byte(v) {
				if c == '"' || c == '\\' {
					buf.WriteByte('\\')
				}
				buf.WriteByte(c)
			}
			buf.WriteByte('"')
		case float64:
			buf.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		case json.Number:
			buf.WriteString(string(v))
		default:
			fmt.Fprint(&buf, v)
		}
	}
	buf.WriteByte('}')
	return buf.String()
}
