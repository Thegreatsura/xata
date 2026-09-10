// Package roles assigns each member of an organization exactly one role, carried
// by a reserved Keycloak organization group that nothing outside here speaks of.
package roles

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xata/services/auth/keycloak"
)

// Role is the identifier used on the wire.
type Role string

const (
	Admin  Role = "admin"
	Editor Role = "editor"
	Viewer Role = "viewer"
)

// Default is what a member with no reserved group resolves to.
const Default = Editor

// Definition describes a role for the API.
type Definition struct {
	Role        Role
	Name        string
	Description string
}

// All is ordered from most to least privileged.
var All = []Definition{
	{Admin, "Admin", "Full access, including billing, members, roles and deleting the organization"},
	{Editor, "Editor", "Create and change projects, branches and their data"},
	{Viewer, "Viewer", "Read-only access to projects and branches"},
}

func (r Role) groupName() string {
	for _, d := range All {
		if d.Role == r {
			return d.Name
		}
	}
	return ""
}

func (r Role) Valid() bool {
	return r.groupName() != ""
}

// Case-insensitive, so a group differing only in case cannot shadow a reserved one.
func roleOfGroup(name string) (Role, bool) {
	for _, d := range All {
		if strings.EqualFold(name, d.Name) {
			return d.Role, true
		}
	}
	return "", false
}

// SeedMembers is resolved only when the Admin role needs seeding.
type SeedMembers func(ctx context.Context) ([]string, error)

// Roles enforces the Xata role rules on top of Keycloak organization groups.
type Roles interface {
	// Members returns the role held by every member of the organization.
	Members(ctx context.Context, organizationID string) (map[string]Role, error)
	// SetMember replaces the role of one member.
	SetMember(ctx context.Context, organizationID, userID string, role Role, callerID string) error
	// EnsureRoles creates any missing reserved group, seeding Admin when empty.
	EnsureRoles(ctx context.Context, organizationID string, seed SeedMembers) error
	// CheckOrganizationMemberRemovable refuses to strand an organization with no Admin.
	CheckOrganizationMemberRemovable(ctx context.Context, organizationID, userID string) error
	// RemoveMemberFromAllRoles clears a member's role when they leave.
	RemoveMemberFromAllRoles(ctx context.Context, organizationID, userID string) error
}

type rolesService struct {
	realm  string
	kcRest keycloak.KeyCloak
}

func NewRoles(realm string, kcRest keycloak.KeyCloak) Roles {
	return &rolesService{realm: realm, kcRest: kcRest}
}

func (s *rolesService) Members(ctx context.Context, organizationID string) (map[string]Role, error) {
	orgMembers, err := s.kcRest.ListMembers(ctx, s.realm, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list organization members: %w", err)
	}

	byUser := make(map[string]Role, len(orgMembers))
	for _, m := range orgMembers {
		byUser[m.ID] = Default
	}

	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	for _, g := range groups {
		role, ok := roleOfGroup(g.Name)
		if !ok {
			continue
		}
		members, err := s.kcRest.ListGroupMembers(ctx, s.realm, organizationID, g.ID)
		if err != nil {
			return nil, fmt.Errorf("list role members: %w", err)
		}
		for _, m := range members {
			// Keycloak keeps membership after a user leaves the organization.
			if _, current := byUser[m.ID]; current {
				byUser[m.ID] = role
			}
		}
	}
	return byUser, nil
}

func (s *rolesService) SetMember(ctx context.Context, organizationID, userID string, role Role, callerID string) error {
	if !role.Valid() {
		return ErrUnknownRole{Role: string(role)}
	}

	current, err := s.Members(ctx, organizationID)
	if err != nil {
		return err
	}

	// Authorize first, so a caller who may not manage roles learns nothing.
	if current[callerID] != Admin {
		return ErrNotAdmin{}
	}
	if _, member := current[userID]; !member {
		return ErrUserNotOrganizationMember{UserID: userID}
	}
	if role != Admin && current[userID] == Admin && countRole(current, Admin) <= 1 {
		return ErrLastAdmin{}
	}

	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}

	// Join first, so a failure part-way grants too much rather than nothing.
	target, err := s.groupFor(ctx, organizationID, groups, role)
	if err != nil {
		return err
	}
	if err := s.kcRest.AddGroupMember(ctx, s.realm, organizationID, target, userID); err != nil {
		return fmt.Errorf("assign role %s: %w", role, err)
	}
	for _, g := range groups {
		other, ok := roleOfGroup(g.Name)
		if !ok || other == role {
			continue
		}
		if err := s.kcRest.RemoveGroupMember(ctx, s.realm, organizationID, g.ID, userID); err != nil {
			return fmt.Errorf("clear role %s: %w", other, err)
		}
	}
	return nil
}

func (s *rolesService) EnsureRoles(ctx context.Context, organizationID string, seed SeedMembers) error {
	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}

	for _, d := range All {
		if _, err := s.groupFor(ctx, organizationID, groups, d.Role); err != nil {
			return err
		}
	}

	if seed == nil {
		return nil
	}
	admins, err := s.membersOf(ctx, organizationID, Admin)
	if err != nil {
		return err
	}
	if len(admins) > 0 {
		return nil
	}
	return s.seedAdmins(ctx, organizationID, seed)
}

func (s *rolesService) CheckOrganizationMemberRemovable(ctx context.Context, organizationID, userID string) error {
	current, err := s.Members(ctx, organizationID)
	if err != nil {
		return err
	}
	if current[userID] == Admin && countRole(current, Admin) <= 1 {
		return ErrLastAdmin{}
	}
	return nil
}

func (s *rolesService) RemoveMemberFromAllRoles(ctx context.Context, organizationID, userID string) error {
	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}
	// Best-effort: one failing role must not strand the member in the rest.
	var errs []error
	for _, g := range groups {
		if _, ok := roleOfGroup(g.Name); !ok {
			continue
		}
		if err := s.kcRest.RemoveGroupMember(ctx, s.realm, organizationID, g.ID, userID); err != nil {
			errs = append(errs, fmt.Errorf("clear role %s: %w", g.Name, err))
		}
	}
	return errors.Join(errs...)
}

// groupFor returns the reserved group backing a role, creating it if absent.
func (s *rolesService) groupFor(ctx context.Context, organizationID string, groups []keycloak.Group, role Role) (string, error) {
	for _, g := range groups {
		if found, ok := roleOfGroup(g.Name); ok && found == role {
			return g.ID, nil
		}
	}
	created, err := s.kcRest.CreateGroup(ctx, s.realm, organizationID, role.groupName())
	if err != nil {
		return "", fmt.Errorf("create role %s: %w", role, err)
	}
	return created.ID, nil
}

func (s *rolesService) membersOf(ctx context.Context, organizationID string, role Role) ([]string, error) {
	current, err := s.Members(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for userID, held := range current {
		if held == role {
			ids = append(ids, userID)
		}
	}
	return ids, nil
}

// seedAdmins preserves the prior behaviour, where every member could do anything.
func (s *rolesService) seedAdmins(ctx context.Context, organizationID string, seed SeedMembers) error {
	memberIDs, err := seed(ctx)
	if err != nil {
		return fmt.Errorf("resolve admin seed: %w", err)
	}
	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}
	target, err := s.groupFor(ctx, organizationID, groups, Admin)
	if err != nil {
		return err
	}
	for _, id := range memberIDs {
		if id == "" {
			continue
		}
		if err := s.kcRest.AddGroupMember(ctx, s.realm, organizationID, target, id); err != nil {
			return fmt.Errorf("seed admin %s: %w", id, err)
		}
	}
	return nil
}

func countRole(byUser map[string]Role, role Role) int {
	n := 0
	for _, held := range byUser {
		if held == role {
			n++
		}
	}
	return n
}
