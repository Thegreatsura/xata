package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type httpError interface {
	error
	StatusCode() int
}

// userError wraps an error with a custom error message.
// The userError is used to create more user friendly error messages
// from runtime internal errors for API users.
//
// The http StatusCode for user errors is chosen based on the wrapped type.
type userError struct {
	inner   error
	message string
}

func (e *userError) StatusCode() int { return GetErrorStatusCode(e.inner) }
func (e *userError) Unwrap() error   { return e.inner }
func (e *userError) Error() string {
	if e.message != "" {
		return e.message
	}
	return e.inner.Error()
}

// badRequestError wraps an error and ensures that the status code is always
// reported as StatusBadRequest (400).
//
// The error message is taken from the wrapped error.
type badRequestError struct{ inner error }

func (e badRequestError) StatusCode() int { return http.StatusBadRequest }
func (e badRequestError) Unwrap() error   { return e.inner }
func (e badRequestError) Error() string   { return e.inner.Error() }

// BadRequestErr creates a new badRequestError using the format string given.
func BadRequestErr(str string) error {
	return badRequestError{inner: errors.New(str)}
}

// BadRequestErrf creates a new badRequestError using the format string given.
func BadRequestErrf(str string, args ...any) error {
	return badRequestError{inner: fmt.Errorf(str, args...)}
}

type ErrorAuthorizationFailed struct {
	Reason string
}

func (e ErrorAuthorizationFailed) StatusCode() int { return http.StatusUnauthorized }
func (e ErrorAuthorizationFailed) Error() string   { return e.Reason }

// makeHTTPErrorHandler creates the error handler used with the echo Framework.
//
// The handler tries to guess http status codes and produce user-friendly messages
// from internal or go stdlib errors.
//
// Internal errors with status code 5xx will be logged, but the error string
// is not shown to the user. Instead, we are reporting 'Internal Error (request ID: <x>)'.
// The request ID can be used to find logs/traces that belong to the failed request.
func makeHTTPErrorHandler() echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		ctx := c.Request().Context()
		logger := log.Ctx(ctx)

		if err := ctx.Err(); errors.Is(err, context.Canceled) {
			// client disconnected, do not reply
			logger.Warn().Msg(err.Error())
			return
		}

		code := GetErrorStatusCode(err)
		if c.Request().Method == http.MethodHead {
			if err := c.NoContent(code); err != nil {
				logger.Err(err).Msg("Write no-contents error response")
			}
			return
		}

		simpleError := struct {
			ID      string `json:"id,omitempty"`
			Message string `json:"message,omitempty"`
		}{
			ID: c.Request().Header.Get(echo.HeaderXRequestID),
		}

		if code >= 500 {
			// 500 errors should be logged at error level
			logger.Error().Msg(err.Error())

			if code == 503 {
				c.Response().Header().Set("Retry-After", "30")
				simpleError.Message = "Service temporarily unavailable. Please try again later."
			} else {
				simpleError.Message = "Internal Error"
			}
			if requestID := c.Response().Header().Get(echo.HeaderXRequestID); requestID != "" {
				simpleError.Message += " (Request ID: " + requestID + ")"
			}
		} else {
			// Anything else should be logged at debug level
			logger.Debug().Msg(err.Error())
			simpleError.Message = ErrorMessage(err)
		}

		if err = c.JSON(code, simpleError); err != nil {
			logger.Err(err).Msg("write JSON error response")
		}
	}
}

// GetErrorStatusCode tries to find the appropriate status code for internal or
// go runtime errors.
//
// We get the status code from the error type itself, if the error type
// implements the `StatusCode()` method.
//
// We default to 500 if the error type is unknown.
//
// When errors are wrapped the outermost errors will take preference.
//
//nolint:errorlint // the function is already recursive using Unwrap.
func GetErrorStatusCode(err error) int {
	switch err := err.(type) {

	case httpError:
		return err.StatusCode()

	case *json.SyntaxError,
		*json.UnmarshalTypeError,
		*json.UnsupportedValueError:
		return http.StatusBadRequest

	case *echo.HTTPError:
		return err.Code

	default:
		if child := errors.Unwrap(err); child != nil {
			return GetErrorStatusCode(child)
		}
		return http.StatusInternalServerError
	}
}

// UserError optionally wraps internal errors with more user friendly
// messages that will not expose internal Go runtime/types details.
//
// UserError is normally used to wrap parse/unmarshal or validation errors
// in the api handlers.
//
//nolint:errorlint // the function is already recursive using Unwrap.
func UserError(err error) error {
	if err == nil {
		return nil
	}

	switch err := err.(type) {
	case *json.UnmarshalTypeError:
		return &userError{inner: err, message: ErrorMessage(err)}

	case validator.ValidationErrors:
		for _, e := range err {
			if e.Tag() == "required" {
				return BadRequestErrf("missing value for [%s]", e.Field())
			}
		}
		return err
	default:
		if _, ok := err.(httpError); !ok {
			// ensure BadRequest Status code
			return badRequestError{err}
		}
		return err
	}
}

//nolint:errorlint // wrapping error might be ignored, so we might lose context
func ErrorMessage(err error) string {
	switch err := err.(type) {
	case *echo.HTTPError:
		if str, ok := err.Message.(string); ok {
			return str
		}
		return fmt.Sprintf("%v", err.Message)

	case *json.UnsupportedTypeError:
		// TODO: Log, this error indicates an internal error due to us using
		//       a go type that one can not unmarshal to. E.g. we try to unmarshal into a channel
		return err.Error()
	case *json.UnmarshalTypeError:
		// User error (most likely). The type in the JSON document can not be parsed
		// into the expected Go type. Let's warn the user, but do not mention go internals.
		if err.Field != "" {
			return fmt.Sprintf("type mismatch: type [%s] is not valid for field [%s]", err.Value, err.Field)
		}
		return fmt.Sprintf("type mismatch: type [%s] is invalid at offset %v", err.Value, err.Offset)

	default:
		return err.Error()
	}
}

// ErrJSONInvalidField parses invalid field errors from json.Unmarshal. These only return an error string, without
// easily-accessible metadata. In order to give users a better error message, we parse the error string and
// produce a better message for them to act upon.
//
// For the future: it'd be wise to look deeper here. Perhaps another library could better support our needs.
type ErrJSONInvalidField struct {
	field string
}

func (e ErrJSONInvalidField) Error() string {
	return fmt.Sprintf("invalid key [%s] in request", e.field)
}

func NewErrJSONInvalidField(field string) ErrJSONInvalidField {
	return ErrJSONInvalidField{field}
}

func ErrAsJSONInvalidFieldError(err error) (error, bool) {
	if after, ok := strings.CutPrefix(err.Error(), "json: unknown field \""); ok {
		fieldName := after
		fieldName = strings.TrimSuffix(fieldName, "\"")

		return NewErrJSONInvalidField(fieldName), true
	}

	return err, false
}
