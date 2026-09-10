package roles

import (
	"fmt"
	"net/http"
)

type ErrUnknownRole struct {
	Role string
}

func (e ErrUnknownRole) Error() string {
	return fmt.Sprintf("%q is not a role", e.Role)
}

func (e ErrUnknownRole) StatusCode() int {
	return http.StatusBadRequest
}

// ErrLastAdmin means the organization would be left with no Admin.
type ErrLastAdmin struct{}

func (e ErrLastAdmin) Error() string {
	return "an organization must always have at least one Admin"
}

func (e ErrLastAdmin) StatusCode() int {
	return http.StatusConflict
}

type ErrNotAdmin struct{}

func (e ErrNotAdmin) Error() string {
	return "you must be an Admin of the organization to do this"
}

func (e ErrNotAdmin) StatusCode() int {
	return http.StatusForbidden
}

type ErrUserNotOrganizationMember struct {
	UserID string
}

func (e ErrUserNotOrganizationMember) Error() string {
	return fmt.Sprintf("user %s is not a member of the organization", e.UserID)
}

func (e ErrUserNotOrganizationMember) StatusCode() int {
	return http.StatusBadRequest
}
