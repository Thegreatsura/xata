// Package roles assigns each member of an organization exactly one role, carried
// by a reserved Keycloak organization group that nothing outside here speaks of.
package roles

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"xata/services/auth/keycloak"

	"github.com/rs/zerolog/log"
)

// Role is the identifier used on the wire.
type Role string

const (
	Admin  Role = "admin"
	Editor Role = "editor"
	Viewer Role = "viewer"
)

// Unassigned is what a member holds until a role is granted them. It is the least
// of the offered roles on purpose: a membership the backfill missed, or one created
// while nothing was watching, costs its holder access rather than handing it to them.
const Unassigned = Editor

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

// Grantable reports whether the role can be granted. Viewer keeps its reserved group
// but is granted only where the viewer role is enabled.
func (r Role) Grantable(viewer bool) bool {
	return r.Valid() && (viewer || r != Viewer)
}

// Reported is the role shown for a holder of r: without the viewer role, a Viewer can do what an Editor can.
func (r Role) Reported(viewer bool) Role {
	if r == Viewer && !viewer {
		return Editor
	}
	return r
}

// Offered lists the grantable roles, from most to least privileged.
func Offered(viewer bool) []Definition {
	return slices.DeleteFunc(slices.Clone(All), func(d Definition) bool { return !d.Role.Grantable(viewer) })
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

// Roles enforces the Xata role rules on top of Keycloak organization groups.
//
// The unassigned parameter is what a member in no reserved group counts as: Admin while roles are hidden.
type Roles interface {
	// Members returns the role held by every member of the organization.
	Members(ctx context.Context, organizationID string, unassigned Role) (map[string]Role, error)
	// IsAdmin reports whether the user holds Admin.
	IsAdmin(ctx context.Context, organizationID, userID string, unassigned Role) (bool, error)
	// SetMember replaces the role of one member.
	SetMember(ctx context.Context, organizationID, userID string, role Role, callerID string) error
	// AddAdmins creates any missing reserved group and grants Admin to each user.
	AddAdmins(ctx context.Context, organizationID string, userIDs ...string) error
	// Audit reads an organization's role state without writing.
	Audit(ctx context.Context, organizationID string) (Audit, error)
	// CheckOrganizationMemberRemovable refuses to strand an organization with no Admin.
	CheckOrganizationMemberRemovable(ctx context.Context, organizationID, userID string, unassigned Role) error
	// RemoveMemberFromAllRoles clears a member's role when they leave.
	RemoveMemberFromAllRoles(ctx context.Context, organizationID, userID string) error
	// SetInvitation records the role an invitee receives on joining.
	SetInvitation(ctx context.Context, organizationID, email string, role Role) error
	// ClearInvitation forgets the role recorded for an invitee.
	ClearInvitation(ctx context.Context, organizationID, email string) error
	// Invitations returns the role recorded for each invited address.
	Invitations(ctx context.Context, organizationID string) (map[string]Role, error)
}

type rolesService struct {
	realm  string
	kcRest keycloak.KeyCloak
	viewer func(context.Context) bool
}

// Option configures the service NewRoles returns.
type Option func(*rolesService)

// WithViewer decides per request whether Viewer may be granted; without it Viewer never is.
func WithViewer(enabled func(context.Context) bool) Option {
	return func(s *rolesService) { s.viewer = enabled }
}

func NewRoles(realm string, kcRest keycloak.KeyCloak, opts ...Option) Roles {
	s := &rolesService{realm: realm, kcRest: kcRest, viewer: func(context.Context) bool { return false }}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Audit describes an organization's reserved groups and who holds them.
type Audit struct {
	Members        int
	MissingRoles   []Role
	DuplicateRoles []Role
	Admins         int
	Unassigned     []string
}

type holding struct {
	role    Role
	userIDs []string
}

type snapshot struct {
	members  []keycloak.OrganizationMember
	holdings []holding
}

func (s *rolesService) read(ctx context.Context, organizationID string) (snapshot, error) {
	orgMembers, err := s.kcRest.ListMembers(ctx, s.realm, organizationID)
	if err != nil {
		return snapshot{}, fmt.Errorf("list organization members: %w", err)
	}
	current := make(map[string]struct{}, len(orgMembers))
	for _, m := range orgMembers {
		current[m.ID] = struct{}{}
	}

	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return snapshot{}, fmt.Errorf("list groups: %w", err)
	}
	snap := snapshot{members: orgMembers}
	for _, g := range groups {
		role, ok := roleOfGroup(g.Name)
		if !ok {
			continue
		}
		members, err := s.kcRest.ListGroupMembers(ctx, s.realm, organizationID, g.ID)
		if err != nil {
			return snapshot{}, fmt.Errorf("list role members: %w", err)
		}
		h := holding{role: role}
		for _, m := range members {
			// Keycloak keeps membership after a user leaves the organization.
			if _, ok := current[m.ID]; ok {
				h.userIDs = append(h.userIDs, m.ID)
			}
		}
		snap.holdings = append(snap.holdings, h)
	}
	return snap, nil
}

func (s *rolesService) Members(ctx context.Context, organizationID string, unassigned Role) (map[string]Role, error) {
	snap, err := s.read(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	byUser := make(map[string]Role, len(snap.members))
	for _, m := range snap.members {
		byUser[m.ID] = unassigned
	}
	for _, h := range snap.holdings {
		for _, id := range h.userIDs {
			byUser[id] = h.role
		}
	}
	return byUser, nil
}

func (s *rolesService) Audit(ctx context.Context, organizationID string) (Audit, error) {
	snap, err := s.read(ctx, organizationID)
	if err != nil {
		return Audit{}, err
	}

	groupsOf := make(map[Role]int, len(All))
	assigned := make(map[string]struct{}, len(snap.members))
	admins := make(map[string]struct{})
	for _, h := range snap.holdings {
		groupsOf[h.role]++
		for _, id := range h.userIDs {
			assigned[id] = struct{}{}
			if h.role == Admin {
				admins[id] = struct{}{}
			}
		}
	}

	audit := Audit{Members: len(snap.members), Admins: len(admins)}
	for _, d := range All {
		switch n := groupsOf[d.Role]; {
		case n == 0:
			audit.MissingRoles = append(audit.MissingRoles, d.Role)
		case n > 1:
			audit.DuplicateRoles = append(audit.DuplicateRoles, d.Role)
		}
	}
	for _, m := range snap.members {
		if _, ok := assigned[m.ID]; !ok {
			audit.Unassigned = append(audit.Unassigned, m.ID)
		}
	}
	return audit, nil
}

func (s *rolesService) IsAdmin(ctx context.Context, organizationID, userID string, unassigned Role) (bool, error) {
	if userID == "" {
		return false, nil
	}
	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return false, fmt.Errorf("list groups: %w", err)
	}
	admin, err := s.holds(ctx, organizationID, groups, Admin, userID)
	if admin || err != nil || unassigned != Admin {
		return admin, err
	}
	// The user counts as Admin unless a lesser reserved group holds them.
	for _, d := range All[1:] {
		held, err := s.holds(ctx, organizationID, groups, d.Role, userID)
		if err != nil {
			return false, err
		}
		if held {
			return false, nil
		}
	}
	return true, nil
}

func (s *rolesService) holds(ctx context.Context, organizationID string, groups []keycloak.Group, role Role, userID string) (bool, error) {
	for _, g := range groups {
		if found, ok := roleOfGroup(g.Name); !ok || found != role {
			continue
		}
		holders, err := s.kcRest.ListGroupMembers(ctx, s.realm, organizationID, g.ID)
		if err != nil {
			return false, fmt.Errorf("list role members: %w", err)
		}
		for _, m := range holders {
			if m.ID == userID {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *rolesService) SetMember(ctx context.Context, organizationID, userID string, role Role, callerID string) error {
	if !role.Valid() {
		return ErrUnknownRole{Role: string(role)}
	}

	current, err := s.Members(ctx, organizationID, Unassigned)
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
	if !role.Grantable(s.viewer(ctx)) {
		return ErrRoleNotGrantable{Role: string(role)}
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
	if role == Admin || current[userID] != Admin {
		return nil
	}
	err = s.keepAnAdmin(ctx, organizationID, groups, userID, target)
	if err != nil && !errors.Is(err, ErrLastAdmin{}) {
		log.Ctx(ctx).Err(err).Str("org_id", organizationID).Bool("roles_last_admin_restore_failed", true).
			Msgf("restore an Admin for organization [%s]", organizationID)
	}
	return err
}

// keepAnAdmin restores Admin to a member just demoted from it when a concurrent demotion left no Admin.
func (s *rolesService) keepAnAdmin(ctx context.Context, organizationID string, groups []keycloak.Group, userID, demotedTo string) error {
	after, err := s.Members(ctx, organizationID, Unassigned)
	if err != nil {
		return err
	}
	if countRole(after, Admin) > 0 {
		return nil
	}
	admin, err := s.groupFor(ctx, organizationID, groups, Admin)
	if err != nil {
		return err
	}
	if err := s.kcRest.AddGroupMember(ctx, s.realm, organizationID, admin, userID); err != nil {
		return fmt.Errorf("restore role %s: %w", Admin, err)
	}
	if err := s.kcRest.RemoveGroupMember(ctx, s.realm, organizationID, demotedTo, userID); err != nil {
		return fmt.Errorf("undo role change: %w", err)
	}
	return ErrLastAdmin{}
}

func (s *rolesService) AddAdmins(ctx context.Context, organizationID string, userIDs ...string) error {
	groups, err := s.ensureGroups(ctx, organizationID)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range userIDs {
		if id == "" {
			continue
		}
		if err := s.kcRest.AddGroupMember(ctx, s.realm, organizationID, groups[Admin], id); err != nil {
			errs = append(errs, fmt.Errorf("grant admin %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func (s *rolesService) CheckOrganizationMemberRemovable(ctx context.Context, organizationID, userID string, unassigned Role) error {
	current, err := s.Members(ctx, organizationID, unassigned)
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

func (s *rolesService) ensureGroups(ctx context.Context, organizationID string) (map[Role]string, error) {
	groups, err := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	ids := make(map[Role]string, len(All))
	for _, d := range All {
		id, err := s.groupFor(ctx, organizationID, groups, d.Role)
		if err != nil {
			return nil, err
		}
		ids[d.Role] = id
	}
	return ids, nil
}

// groupFor returns the reserved group backing a role, creating it if absent.
func (s *rolesService) groupFor(ctx context.Context, organizationID string, groups []keycloak.Group, role Role) (string, error) {
	if id, ok := findRoleGroup(groups, role); ok {
		return id, nil
	}
	created, err := s.kcRest.CreateGroup(ctx, s.realm, organizationID, role.groupName())
	if err == nil {
		return created.ID, nil
	}
	if !errors.As(err, &keycloak.ErrGroupAlreadyExists{}) {
		return "", fmt.Errorf("create role %s: %w", role, err)
	}

	current, listErr := s.kcRest.ListGroups(ctx, s.realm, organizationID)
	if listErr != nil {
		return "", fmt.Errorf("list groups: %w", listErr)
	}
	if id, ok := findRoleGroup(current, role); ok {
		return id, nil
	}
	return "", fmt.Errorf("create role %s: %w", role, err)
}

func findRoleGroup(groups []keycloak.Group, role Role) (string, bool) {
	for _, g := range groups {
		if found, ok := roleOfGroup(g.Name); ok && found == role {
			return g.ID, true
		}
	}
	return "", false
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

// CheckGrantable refuses a role that is unknown or not granted yet.
func CheckGrantable(role Role, viewer bool) error {
	if !role.Valid() {
		return ErrUnknownRole{Role: string(role)}
	}
	if !role.Grantable(viewer) {
		return ErrRoleNotGrantable{Role: string(role)}
	}
	return nil
}
