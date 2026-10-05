package xvalidator

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"unicode"
)

var validDurationRegex = regexp.MustCompile(`^(\d+)(d|h|m|s|ms)$`)

var errEmptyIdentifier = errors.New("empty")

type ErrorMaxLength struct{ Limit int }

func (e ErrorMaxLength) Error() string {
	return fmt.Sprintf("max length is %d", e.Limit)
}

func (e ErrorMaxLength) StatusCode() int {
	return http.StatusBadRequest
}

// IsValidIdentifier checks if a string is a valid identifier.
// Identifiers can include alphanumerics and the special characters -, _, or ~.
func IsValidIdentifier(id string) error {
	if len(id) == 0 {
		return errEmptyIdentifier
	}

	for i, r := range id {
		if err := checkSpecial(i, r); err != nil {
			return err
		}

		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' && r != '~' {
			return fmt.Errorf(
				"offset %d: invalid symbol [%c], only alphanumerics and '_', or '~' are allowed",
				i, r)
		}
	}

	return nil
}

func checkSpecial(i int, r rune) error {
	if unicode.IsControl(r) { // \n, \r, \t or other control symbols?
		return fmt.Errorf("offset %d: control characters are not allowed", i)
	}

	if !unicode.IsPrint(r) { // unicode non printable symbols? e.g. BOM, invalid UTF-8 char, ...
		return fmt.Errorf("offset %d: unprintable symbols are not allowed", i)
	}

	if r > unicode.MaxASCII { // we want ASCII
		return fmt.Errorf("offset %d: unicode characters are not allowed", i)
	}
	return nil
}

// IsDurationValid checks if the duration provided seems valid.
func IsDurationValid(e string) bool {
	return validDurationRegex.MatchString(e)
}
