package groups

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xata/services/auth/keycloak"
)

// OwnerGroupName is reserved: it cannot be renamed, deleted or left empty.
const OwnerGroupName = "Owner"

// SeedMembers is resolved only when the Owner group needs seeding.
type SeedMembers func(ctx context.Context) ([]string, error)

// Groups enforces the Xata Owner-group rules on top of Keycloak groups.
type Groups interface {
	List(ctx context.Context, organizationID string) ([]keycloak.Group, error)
	Get(ctx context.Context, organizationID, groupID string) (keycloak.Group, error)
	Create(ctx context.Context, organizationID, name string) (keycloak.Group, error)
	Update(ctx context.Context, organizationID, groupID, name string) (keycloak.Group, error)
	Delete(ctx context.Context, organizationID, groupID string) error
	ListMembers(ctx context.Context, organizationID, groupID string) ([]keycloak.OrganizationMember, error)
	AddMember(ctx context.Context, organizationID, groupID, userID, callerID string) error
	RemoveMember(ctx context.Context, organizationID, groupID, userID, callerID string) error
	EnsureOwnerGroup(ctx context.Context, organizationID string, seed SeedMembers) (keycloak.Group, error)
	CheckOrganizationMemberRemovable(ctx context.Context, organizationID, userID string) error
	RemoveMemberFromAllGroups(ctx context.Context, organizationID, userID string) error
}

type groupsService struct {
	realm  string
	kcRest keycloak.KeyCloak
}

func NewGroups(realm string, kcRest keycloak.KeyCloak) Groups {
	return &groupsService{realm: realm, kcRest: kcRest}
}

func isOwnerGroup(g keycloak.Group) bool {
	return isReservedName(g.Name)
}

// Case-insensitive: "Owner" and "owner" must not both exist.
func isReservedName(name string) bool {
	return strings.EqualFold(name, OwnerGroupName)
}

func (s *groupsService) List(ctx context.Context, organizationID string) ([]keycloak.Group, error) {
	return s.kcRest.ListGroups(ctx, s.realm, organizationID)
}

func (s *groupsService) Get(ctx context.Context, organizationID, groupID string) (keycloak.Group, error) {
	return s.kcRest.GetGroup(ctx, s.realm, organizationID, groupID)
}

func (s *groupsService) Create(ctx context.Context, organizationID, name string) (keycloak.Group, error) {
	if isReservedName(name) {
		return keycloak.Group{}, ErrGroupNameReserved{Name: name}
	}
	return s.kcRest.CreateGroup(ctx, s.realm, organizationID, name)
}

func (s *groupsService) Update(ctx context.Context, organizationID, groupID, name string) (keycloak.Group, error) {
	group, err := s.kcRest.GetGroup(ctx, s.realm, organizationID, groupID)
	if err != nil {
		return keycloak.Group{}, err
	}
	if isOwnerGroup(group) {
		return keycloak.Group{}, ErrOwnerGroupImmutable{}
	}
	if isReservedName(name) {
		return keycloak.Group{}, ErrGroupNameReserved{Name: name}
	}
	return s.kcRest.UpdateGroup(ctx, s.realm, organizationID, groupID, name)
}

func (s *groupsService) Delete(ctx context.Context, organizationID, groupID string) error {
	group, err := s.kcRest.GetGroup(ctx, s.realm, organizationID, groupID)
	if err != nil {
		return err
	}
	if isOwnerGroup(group) {
		return ErrOwnerGroupImmutable{}
	}
	return s.kcRest.DeleteGroup(ctx, s.realm, organizationID, groupID)
}

func (s *groupsService) ListMembers(ctx context.Context, organizationID, groupID string) ([]keycloak.OrganizationMember, error) {
	return s.kcRest.ListGroupMembers(ctx, s.realm, organizationID, groupID)
}

func (s *groupsService) AddMember(ctx context.Context, organizationID, groupID, userID, callerID string) error {
	group, err := s.kcRest.GetGroup(ctx, s.realm, organizationID, groupID)
	if err != nil {
		return err
	}

	orgMembers, err := s.kcRest.ListMembers(ctx, s.realm, organizationID)
	if err != nil {
		return fmt.Errorf("list organization members: %w", err)
	}

	// Authorize first, so a caller who may not manage owners learns nothing about
	// who belongs to the organization.
	if isOwnerGroup(group) {
		owners, err := s.ownersAmong(ctx, organizationID, group.ID, orgMembers)
		if err != nil {
			return err
		}
		if !containsMember(owners, callerID) {
			return ErrNotOwner{}
		}
	}

	if !containsMember(orgMembers, userID) {
		return ErrUserNotOrganizationMember{UserID: userID}
	}
	return s.kcRest.AddGroupMember(ctx, s.realm, organizationID, groupID, userID)
}

func (s *groupsService) RemoveMember(ctx context.Context, organizationID, groupID, userID, callerID string) error {
	group, err := s.kcRest.GetGroup(ctx, s.realm, organizationID, groupID)
	if err != nil {
		return err
	}
	if isOwnerGroup(group) {
		owners, err := s.activeOwnerMembers(ctx, organizationID, group.ID)
		if err != nil {
			return err
		}
		if !containsMember(owners, callerID) {
			return ErrNotOwner{}
		}
		if len(owners) <= 1 && containsMember(owners, userID) {
			return ErrOwnerGroupLastMember{}
		}
	}
	return s.kcRest.RemoveGroupMember(ctx, s.realm, organizationID, groupID, userID)
}

func (s *groupsService) EnsureOwnerGroup(ctx context.Context, organizationID string, seed SeedMembers) (keycloak.Group, error) {
	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return keycloak.Group{}, fmt.Errorf("list groups: %w", err)
	}

	owner, found := findOwnerGroup(groups)
	if !found {
		owner, err = s.kcRest.CreateGroup(ctx, s.realm, organizationID, OwnerGroupName)
		if err != nil {
			return keycloak.Group{}, fmt.Errorf("create owner group: %w", err)
		}
		return owner, s.seedGroup(ctx, organizationID, owner.ID, seed)
	}

	if seed == nil {
		return owner, nil
	}

	members, err := s.activeOwnerMembers(ctx, organizationID, owner.ID)
	if err != nil {
		return keycloak.Group{}, err
	}
	if len(members) == 0 {
		return owner, s.seedGroup(ctx, organizationID, owner.ID, seed)
	}

	return owner, nil
}

func (s *groupsService) CheckOrganizationMemberRemovable(ctx context.Context, organizationID, userID string) error {
	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}
	owner, found := findOwnerGroup(groups)
	if !found {
		return nil
	}
	members, err := s.activeOwnerMembers(ctx, organizationID, owner.ID)
	if err != nil {
		return err
	}
	if len(members) <= 1 && containsMember(members, userID) {
		return ErrOwnerGroupLastMember{}
	}
	return nil
}

func (s *groupsService) RemoveMemberFromAllGroups(ctx context.Context, organizationID, userID string) error {
	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}
	// Best-effort: one failing group must not strand the user in the rest.
	var errs []error
	for _, g := range groups {
		if err := s.kcRest.RemoveGroupMember(ctx, s.realm, organizationID, g.ID, userID); err != nil {
			errs = append(errs, fmt.Errorf("remove member from group %s: %w", g.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (s *groupsService) seedGroup(ctx context.Context, organizationID, groupID string, seed SeedMembers) error {
	if seed == nil {
		return nil
	}
	memberIDs, err := seed(ctx)
	if err != nil {
		return fmt.Errorf("resolve owner group seed: %w", err)
	}
	for _, id := range memberIDs {
		if id == "" {
			continue
		}
		if err := s.kcRest.AddGroupMember(ctx, s.realm, organizationID, groupID, id); err != nil {
			return fmt.Errorf("seed owner group member %s: %w", id, err)
		}
	}
	return nil
}

// Keycloak keeps group membership after a user leaves the organization, so the
// raw group list can claim owners who can no longer act.
// ownersAmong filters out group entries Keycloak keeps after a user has left the
// organization, which would otherwise count as owners who can no longer act.
func (s *groupsService) ownersAmong(ctx context.Context, organizationID, ownerGroupID string, orgMembers []keycloak.OrganizationMember) ([]keycloak.OrganizationMember, error) {
	groupMembers, err := s.kcRest.ListGroupMembers(ctx, s.realm, organizationID, ownerGroupID)
	if err != nil {
		return nil, fmt.Errorf("list owner group members: %w", err)
	}
	active := make([]keycloak.OrganizationMember, 0, len(groupMembers))
	for _, m := range groupMembers {
		if containsMember(orgMembers, m.ID) {
			active = append(active, m)
		}
	}
	return active, nil
}

func (s *groupsService) activeOwnerMembers(ctx context.Context, organizationID, ownerGroupID string) ([]keycloak.OrganizationMember, error) {
	orgMembers, err := s.kcRest.ListMembers(ctx, s.realm, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list organization members: %w", err)
	}
	return s.ownersAmong(ctx, organizationID, ownerGroupID, orgMembers)
}

func findOwnerGroup(groups []keycloak.Group) (keycloak.Group, bool) {
	for _, g := range groups {
		if isOwnerGroup(g) {
			return g, true
		}
	}
	return keycloak.Group{}, false
}

func containsMember(members []keycloak.OrganizationMember, userID string) bool {
	for _, m := range members {
		if m.ID == userID {
			return true
		}
	}
	return false
}
