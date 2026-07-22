package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
)

type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// groupPayload is the minimal GroupRepresentation Keycloak accepts for a group.
type groupPayload struct {
	Name string `json:"name"`
}

func (r *restKC) ListGroups(ctx context.Context, realm, organizationID string) ([]Group, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	return r.listOrgGroups(ctx, realm, organization.ID)
}

func (r *restKC) GetGroup(ctx context.Context, realm, organizationID, groupID string) (Group, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return Group{}, fmt.Errorf("failed to get organization: %w", err)
	}
	return r.getOrgGroup(ctx, realm, organization.ID, groupID)
}

func (r *restKC) CreateGroup(ctx context.Context, realm, organizationID, name string) (Group, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return Group{}, fmt.Errorf("failed to get organization: %w", err)
	}

	groupsURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "groups")
	if err != nil {
		return Group{}, fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "POST", groupsURL, nil, groupPayload{Name: name})
	if err != nil {
		return Group{}, fmt.Errorf("failed to create group: %w", err)
	}
	if resp.StatusCode() == http.StatusConflict {
		return Group{}, ErrGroupAlreadyExists{Name: name}
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusCreated, http.StatusOK, http.StatusNoContent) {
		return Group{}, fmt.Errorf("failed to create group: status code: %d", resp.StatusCode())
	}

	// Keycloak returns the new group's id as the last segment of the Location
	// header on the 201 response (".../groups/{groupId}").
	location := resp.Header().Get("Location")
	if location == "" {
		return Group{}, fmt.Errorf("create group %q: no id in Location header", name)
	}
	return Group{ID: path.Base(location), Name: name}, nil
}

func (r *restKC) UpdateGroup(ctx context.Context, realm, organizationID, groupID, name string) (Group, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return Group{}, fmt.Errorf("failed to get organization: %w", err)
	}

	groupURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "groups", groupID)
	if err != nil {
		return Group{}, fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodPut, groupURL, nil, groupPayload{Name: name})
	if err != nil {
		return Group{}, fmt.Errorf("failed to update group: %w", err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return Group{}, ErrGroupNotFound{ID: groupID}
	}
	if resp.StatusCode() == http.StatusConflict {
		return Group{}, ErrGroupAlreadyExists{Name: name}
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return Group{}, fmt.Errorf("failed to update group: status code: %d", resp.StatusCode())
	}

	return r.getOrgGroup(ctx, realm, organization.ID, groupID)
}

func (r *restKC) DeleteGroup(ctx context.Context, realm, organizationID, groupID string) error {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return fmt.Errorf("failed to get organization: %w", err)
	}

	groupURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "groups", groupID)
	if err != nil {
		return fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodDelete, groupURL, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete group: %w", err)
	}
	// DELETE is idempotent: a 404 means the group is already gone.
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return fmt.Errorf("failed to delete group: status code: %d", resp.StatusCode())
	}
	return nil
}

func (r *restKC) ListGroupMembers(ctx context.Context, realm, organizationID, groupID string) ([]OrganizationMember, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	listURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "groups", groupID, "members")
	if err != nil {
		return nil, fmt.Errorf("failed to join URL: %w", err)
	}

	queryParams := map[string]string{
		"max": fmt.Sprintf("%d", MaxOrganizationMembers),
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "GET", listURL, queryParams, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, ErrGroupNotFound{ID: groupID}
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	var users []User
	if err := json.Unmarshal(resp.Body(), &users); err != nil {
		return nil, fmt.Errorf("failed to unmarshal group members: %w", err)
	}

	res := make([]OrganizationMember, len(users))
	for i, u := range users {
		res[i] = OrganizationMember{
			Email: u.Email,
			Name:  fmt.Sprintf("%s %s", u.FirstName, u.LastName),
			ID:    u.ID,
		}
	}
	return res, nil
}

func (r *restKC) AddGroupMember(ctx context.Context, realm, organizationID, groupID, userID string) error {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return fmt.Errorf("failed to get organization: %w", err)
	}

	memberURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "groups", groupID, "members", userID)
	if err != nil {
		return fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodPut, memberURL, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to add group member: %w", err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return ErrGroupNotFound{ID: groupID}
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusCreated, http.StatusNoContent) {
		return fmt.Errorf("failed to add group member: status code: %d", resp.StatusCode())
	}
	return nil
}

func (r *restKC) RemoveGroupMember(ctx context.Context, realm, organizationID, groupID, userID string) error {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return fmt.Errorf("failed to get organization: %w", err)
	}

	memberURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "groups", groupID, "members", userID)
	if err != nil {
		return fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodDelete, memberURL, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to remove group member: %w", err)
	}
	// DELETE is idempotent: a 404 means the user is already absent from the group.
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return fmt.Errorf("failed to remove group member: status code: %d", resp.StatusCode())
	}
	return nil
}

func (r *restKC) listOrgGroups(ctx context.Context, realm, orgInternalID string) ([]Group, error) {
	listURL, err := r.buildRealmURL(realm, "organizations", orgInternalID, "groups")
	if err != nil {
		return nil, fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "GET", listURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	var groups []Group
	if err := json.Unmarshal(resp.Body(), &groups); err != nil {
		return nil, fmt.Errorf("failed to unmarshal groups: %w", err)
	}
	return groups, nil
}

func (r *restKC) getOrgGroup(ctx context.Context, realm, orgInternalID, groupID string) (Group, error) {
	getURL, err := r.buildRealmURL(realm, "organizations", orgInternalID, "groups", groupID)
	if err != nil {
		return Group{}, fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "GET", getURL, nil, nil)
	if err != nil {
		return Group{}, err
	}
	if resp.StatusCode() == http.StatusNotFound {
		return Group{}, ErrGroupNotFound{ID: groupID}
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
		return Group{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	var group Group
	if err := json.Unmarshal(resp.Body(), &group); err != nil {
		return Group{}, fmt.Errorf("failed to unmarshal group: %w", err)
	}
	return group, nil
}
