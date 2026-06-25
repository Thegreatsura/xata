package orbamount

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMicros(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    int64
		wantErr string
	}{
		"zero":             {input: "0.00", want: 0},
		"negative zero":    {input: "-0.00", want: 0},
		"two dp":           {input: "12.34", want: 12340000},
		"negative two dp":  {input: "-12.34", want: -12340000},
		"three dp":         {input: "0.442", want: 442000},
		"six dp":           {input: "1.123456", want: 1123456},
		"one dp":           {input: "1.2", want: 1200000},
		"no decimal":       {input: "5", want: 5000000},
		"trim whitespace":  {input: " 1.23 ", want: 1230000},
		"leading zero":     {input: "01.23", want: 1230000},
		"max int64 micros": {input: "9223372036854.775807", want: 9223372036854775807},
		"min int64 micros": {input: "-9223372036854.775808", want: -9223372036854775808},

		"empty":             {input: "", wantErr: "invalid orb amount"},
		"non numeric":       {input: "abc", wantErr: "strconv.ParseInt"},
		"seven dp":          {input: "1.1234567", wantErr: "more than 6 decimal places"},
		"overflow positive": {input: "9223372036854.775808", wantErr: "strconv.ParseInt"},
		"overflow negative": {input: "-9223372036854.775809", wantErr: "strconv.ParseInt"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMicros(tc.input)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseMicrosTruncated(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    int64
		wantErr string
	}{
		"zero":                {input: "0.00", want: 0},
		"six dp exact":        {input: "1.123456", want: 1123456},
		"seven dp truncates":  {input: "1.1234567", want: 1123456},
		"twelve dp truncates": {input: "0.000410958904", want: 410},
		"truncates not round": {input: "0.0000009", want: 0},
		"negative truncates":  {input: "-1.1234569", want: -1123456},
		"no decimal":          {input: "5", want: 5000000},
		"trim whitespace":     {input: " 0.0000001 ", want: 0},

		"empty":       {input: "", wantErr: "invalid orb amount"},
		"non numeric": {input: "abc", wantErr: "strconv.ParseInt"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMicrosTruncated(tc.input)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseCentsTruncated(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    int64
		wantErr string
	}{
		"zero":                {input: "0.00", want: 0},
		"two dp exact":        {input: "1.23", want: 123},
		"one dp pads":         {input: "1.2", want: 120},
		"no decimal":          {input: "5", want: 500},
		"three dp truncates":  {input: "1.239", want: 123},
		"truncates not round": {input: "0.009", want: 0},
		"negative truncates":  {input: "-1.239", want: -123},
		"trim whitespace":     {input: " 1.23 ", want: 123},

		"empty":       {input: "", wantErr: "invalid orb amount"},
		"non numeric": {input: "abc", wantErr: "strconv.ParseInt"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseCentsTruncated(tc.input)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseDollarsTruncated(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   string
		want    int64
		wantErr string
	}{
		"zero":                {input: "0.00", want: 0},
		"whole dollars":       {input: "12", want: 12},
		"fraction truncates":  {input: "12.99", want: 12},
		"truncates not round": {input: "0.99", want: 0},
		"negative truncates":  {input: "-12.99", want: -12},
		"trim whitespace":     {input: " 12.99 ", want: 12},

		"empty":       {input: "", wantErr: "invalid orb amount"},
		"non numeric": {input: "abc", wantErr: "strconv.ParseInt"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseDollarsTruncated(tc.input)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
