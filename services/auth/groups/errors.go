package groups

import (
	"fmt"
	"net/http"
)

type ErrGroupNameReserved struct {
	Name string
}

func (e ErrGroupNameReserved) Error() string {
	return fmt.Sprintf("%q is a reserved group name", e.Name)
}

func (e ErrGroupNameReserved) StatusCode() int {
	return http.StatusBadRequest
}

type ErrOwnerGroupImmutable struct{}

func (e ErrOwnerGroupImmutable) Error() string {
	return fmt.Sprintf("the %q group cannot be modified or deleted", OwnerGroupName)
}

func (e ErrOwnerGroupImmutable) StatusCode() int {
	return http.StatusForbidden
}

// ErrOwnerGroupLastMember means the organization would be left with no owner.
type ErrOwnerGroupLastMember struct{}

func (e ErrOwnerGroupLastMember) Error() string {
	return fmt.Sprintf("the %q group must always have at least one member", OwnerGroupName)
}

func (e ErrOwnerGroupLastMember) StatusCode() int {
	return http.StatusConflict
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
