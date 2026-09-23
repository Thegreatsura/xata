package roles

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Temporary until an invitation can carry a role (https://github.com/keycloak/keycloak/issues/45238),
// e.g. through the invitation attributes of https://github.com/keycloak/keycloak/pull/48842.
// invitedRolesAttribute, its values and the group names must match OrgInvitationRole in keycloak-extensions.
const invitedRolesAttribute = "invitedRoles"

// Keycloak stores each attribute value in an NVARCHAR(255) column.
const maxInvitationLength = 255

// CheckInvitation refuses a role that could not be recorded for the address.
func CheckInvitation(email string, role Role) error {
	if !role.Valid() {
		return ErrUnknownRole{Role: string(role)}
	}
	if utf8.RuneCountInString(invitationEntry(NormalizeEmail(email), role)) > maxInvitationLength {
		return ErrEmailTooLong{}
	}
	return nil
}

func (s *rolesService) SetInvitation(ctx context.Context, organizationID, email string, role Role) error {
	if err := CheckGrantable(role, s.viewer(ctx)); err != nil {
		return err
	}
	if err := CheckInvitation(email, role); err != nil {
		return err
	}
	email = NormalizeEmail(email)
	return s.updateInvitations(ctx, organizationID, func(entries []string) []string {
		return append(forgetInvitation(entries, email), invitationEntry(email, role))
	})
}

func (s *rolesService) ClearInvitation(ctx context.Context, organizationID, email string) error {
	email = NormalizeEmail(email)
	return s.updateInvitations(ctx, organizationID, func(entries []string) []string {
		return forgetInvitation(entries, email)
	})
}

func (s *rolesService) Invitations(ctx context.Context, organizationID string) (map[string]Role, error) {
	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}

	byEmail := map[string]Role{}
	for _, g := range groups {
		if role, ok := roleOfGroup(g.Name); !ok || role != Viewer {
			continue
		}
		for _, entry := range g.Attributes[invitedRolesAttribute] {
			if email, invited, ok := parseInvitation(entry); ok {
				byEmail[email] = invited
			}
		}
		// Only the first match, the group findRoleGroup writes to.
		break
	}
	return byEmail, nil
}

func (s *rolesService) updateInvitations(ctx context.Context, organizationID string, update func([]string) []string) error {
	groups, err := s.ensureGroups(ctx, organizationID)
	if err != nil {
		return err
	}
	if err := s.kcRest.UpdateGroupAttribute(ctx, s.realm, organizationID, groups[Viewer], invitedRolesAttribute, update); err != nil {
		return fmt.Errorf("update invitation roles: %w", err)
	}
	return nil
}

func forgetInvitation(entries []string, email string) []string {
	return slices.DeleteFunc(entries, func(entry string) bool {
		invited, _, _ := parseInvitation(entry)
		return invited == email
	})
}

func invitationEntry(email string, role Role) string {
	return email + "=" + string(role)
}

func parseInvitation(entry string) (string, Role, bool) {
	i := strings.LastIndexByte(entry, '=')
	if i < 0 {
		return "", "", false
	}
	email, role := NormalizeEmail(entry[:i]), Role(entry[i+1:])
	return email, role, email != "" && role.Valid()
}

// NormalizeEmail lowercases an address, as Keycloak stores invitations.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
