package orbamount

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	dollarScale = 0
	centScale   = 2
	microScale  = 6
)

// The data warehouse uses microdollars everywhere

// ParseMicros converts an Orb monetary amount string to microdollars
// (1 dollar = 1,000,000 micros). Up to 6 decimal places are accepted;
// fewer are zero-padded on the right.
func ParseMicros(amount string) (int64, error) {
	if _, frac, _ := strings.Cut(strings.TrimSpace(amount), "."); len(frac) > microScale {
		return 0, fmt.Errorf("invalid orb amount %q: more than 6 decimal places", amount)
	}
	return ParseMicrosTruncated(amount)
}

// ParseMicrosTruncated truncates Orb monetary amounts to 6 d.p. when converting to micros.
func ParseMicrosTruncated(amount string) (int64, error) {
	return orbDecimalToInt64Truncated(amount, microScale)
}

func ParseCentsTruncated(amount string) (int64, error) {
	return orbDecimalToInt64Truncated(amount, centScale)
}

func ParseDollarsTruncated(amount string) (int64, error) {
	return orbDecimalToInt64Truncated(amount, dollarScale)
}

func orbDecimalToInt64Truncated(amount string, scale int) (int64, error) {
	v := strings.TrimSpace(amount)
	if v == "" {
		return 0, fmt.Errorf("invalid orb amount %q: empty string", amount)
	}

	whole, frac, hasDot := strings.Cut(v, ".")
	if !hasDot {
		frac = ""
	}
	frac += strings.Repeat("0", scale)
	frac = frac[:scale]

	return strconv.ParseInt(whole+frac, 10, 64)
}
