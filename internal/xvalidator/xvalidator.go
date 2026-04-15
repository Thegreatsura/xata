package xvalidator

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"
	"unicode"
)

var validEmailRegex = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

var validDurationRegex = regexp.MustCompile(`^(\d+)(d|h|m|s|ms)$`)

var validTimezoneRegex = regexp.MustCompile(`^[+-][01]\d:[0-5]\d$`)

var errEmptyIdentifier = errors.New("empty")

type ErrorMaxLength struct{ Limit int }

func (e ErrorMaxLength) Error() string {
	return fmt.Sprintf("max length is %d", e.Limit)
}

func (e ErrorMaxLength) StatusCode() int {
	return http.StatusBadRequest
}

type ErrorInvalidName struct {
	message string
	name    string
}

func (e ErrorInvalidName) Error() string {
	return fmt.Sprintf("name %s invalid: %s", e.name, e.message)
}

func (e ErrorInvalidName) StatusCode() int {
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

// IsEmailValid checks if the email provided passes the required structure and length.
func IsEmailValid(e string) bool {
	return validEmailRegex.MatchString(e)
}

// IsDurationValid checks if the duration provided seems valid.
func IsDurationValid(e string) bool {
	return validDurationRegex.MatchString(e)
}

// IsTimezoneValid checks if the provided timezone seems valid.
func IsTimezoneValid(e string) bool {
	return validTimezoneRegex.MatchString(e)
}

func IsValidJSON(e string) bool {
	return json.Valid([]byte(e))
}

func DurationValidator(e string) error {
	if !IsDurationValid(e) {
		return errors.New("invalid duration")
	}
	return nil
}

func DateRFC3339Validator(e string) error {
	_, err := time.Parse(time.RFC3339, e)
	return err
}
