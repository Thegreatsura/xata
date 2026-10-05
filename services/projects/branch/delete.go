package branch

import "context"

// DeleteProject deprovisions a project and all its branches through the
// provisioner. Exposed so SaaS wrappers (the Vercel resource handler) can tear
// down a resource's project without holding their own cells/provisioner.
func (s *Service) DeleteProject(ctx context.Context, organizationID, projectID string) error {
	_, err := s.provisioner.DeleteProject(ctx, organizationID, projectID)
	return err
}
