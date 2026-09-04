package rpc

import (
	"context"
	"fmt"

	"xata/services/projects/cells"

	projectsv1 "xata/gen/proto/projects/v1"
	"xata/services/projects/store"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Ensure clusters implements GRPCService interface.
var _ projectsv1.ProjectsServiceServer = (*ProjectsService)(nil)

// ProjectsService is a GRPC service for interacting with projects service.
type ProjectsService struct {
	// fail to compile if the service does not implement all the methods
	projectsv1.UnsafeProjectsServiceServer

	store store.ProjectsStore
	cells cells.Cells

	// nudger wakes the orgstatus worker after a desired-state write.
	nudger Nudger
}

// Nudger is woken when an organization's desired status changes.
type Nudger interface {
	Nudge()
}

// NewProjectsService creates a new ProjectsService.
func NewProjectsService(store store.ProjectsStore, cells cells.Cells) *ProjectsService {
	return &ProjectsService{
		store: store,
		cells: cells,
	}
}

// SetNudger wires the orgstatus worker so status writes are applied promptly
// instead of waiting for the worker's next poll.
func (p *ProjectsService) SetNudger(n Nudger) { p.nudger = n }

// CreateCell implements projectsv1.ProjectsServiceServer.
func (p *ProjectsService) CreateCell(ctx context.Context, input *projectsv1.CreateCellRequest) (*projectsv1.CreateCellResponse, error) {
	_, err := p.store.CreateCell(ctx, input.GetRegionId(), input.GetId(), input.GetClustersGrpcUrl(), input.GetIsPrimary())
	return &projectsv1.CreateCellResponse{}, err
}

// CreateRegion implements projectsv1.ProjectsServiceServer.
func (p *ProjectsService) CreateRegion(ctx context.Context, input *projectsv1.CreateRegionRequest) (*projectsv1.CreateRegionResponse, error) {
	provider, err := store.ParseProvider(input.GetProvider())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	flags := store.RegionFlags{
		PublicAccess:   input.GetPublicAccess(),
		BackupsEnabled: input.GetBackupsEnabled(),
		Provider:       provider,
	}
	if input.OrganizationId != nil {
		_, err = p.store.CreateOrganizationRegion(ctx, input.GetOrganizationId(), input.GetId(), flags, input.GetHostport())
	} else {
		_, err = p.store.CreateRegion(ctx, input.GetId(), flags, input.GetHostport())
	}

	return &projectsv1.CreateRegionResponse{}, err
}

// ListCells implements projectsv1.ProjectsServiceServer.
func (p *ProjectsService) ListCells(ctx context.Context, _ *projectsv1.ListCellsRequest) (*projectsv1.ListCellsResponse, error) {
	cells, err := p.store.ListAllCells(ctx)
	if err != nil {
		return nil, err
	}

	response := &projectsv1.ListCellsResponse{}
	for _, cell := range cells {
		response.Cells = append(response.Cells, &projectsv1.Cell{
			Id:              cell.ID,
			RegionId:        cell.RegionID,
			ClustersGrpcUrl: cell.ClustersGRPCURL,
			IsPrimary:       cell.Primary,
		})
	}
	return response, nil
}

// ListRegions implements projectsv1.ProjectsServiceServer.
func (p *ProjectsService) ListRegions(ctx context.Context, _ *projectsv1.ListRegionsRequest) (*projectsv1.ListRegionsResponse, error) {
	regions, err := p.store.ListAllRegions(ctx)
	if err != nil {
		return nil, err
	}

	response := &projectsv1.ListRegionsResponse{}
	for _, region := range regions {
		response.Regions = append(response.Regions, &projectsv1.Region{
			Id:             region.ID,
			PublicAccess:   region.PublicAccess,
			OrganizationId: region.OrganizationID,
			BackupsEnabled: region.BackupsEnabled,
			Provider:       string(region.Provider),
		})
	}
	return response, nil
}

// ValidateHierarchy implements projectsv1.ProjectsServiceServer.
func (p *ProjectsService) ValidateHierarchy(ctx context.Context, req *projectsv1.ValidateHierarchyRequest) (*projectsv1.ValidateHierarchyResponse, error) {
	err := p.store.ValidateHierarchy(ctx, req.GetOrganizationIds(), req.GetProjectIds(), req.GetBranchIds())
	if err != nil {
		return nil, err
	}

	return &projectsv1.ValidateHierarchyResponse{}, nil
}

// HasActiveProjects implements projectsv1.ProjectsServiceServer.
func (p *ProjectsService) HasActiveProjects(ctx context.Context, req *projectsv1.HasActiveProjectsRequest) (*projectsv1.HasActiveProjectsResponse, error) {
	projects, err := p.store.ListProjects(ctx, req.GetOrganizationId())
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return &projectsv1.HasActiveProjectsResponse{HasActiveProjects: len(projects) > 0}, nil
}

// UpdateOrganizationStatus implements projectsv1.ProjectsServiceServer.
//
// It records the desired state and returns. The orgstatus worker fans that
// state out to the fleet asynchronously.
func (p *ProjectsService) UpdateOrganizationStatus(ctx context.Context, req *projectsv1.UpdateOrganizationStatusRequest) (*projectsv1.UpdateOrganizationStatusResponse, error) {
	if _, err := p.store.UpsertOrganizationStatus(ctx, req.OrganizationId, req.Disabled); err != nil {
		return nil, fmt.Errorf("record organization [%s] status: %w", req.OrganizationId, err)
	}

	if p.nudger != nil {
		p.nudger.Nudge()
	}

	return &projectsv1.UpdateOrganizationStatusResponse{OrganizationId: req.OrganizationId}, nil
}

// DeleteProjectsInOrg implements projectsv1.ProjectsServiceServer.
func (p *ProjectsService) DeleteProjectsInOrg(ctx context.Context, req *projectsv1.DeleteProjectsInOrgRequest) (*projectsv1.DeleteProjectsInOrgResponse, error) {
	projects, err := p.store.ListProjects(ctx, req.OrganizationId)
	if err != nil {
		return nil, err
	}

	response := &projectsv1.DeleteProjectsInOrgResponse{
		OrganizationId: req.OrganizationId,
	}

	for _, project := range projects {
		var projectErrors []string
		branches, err := p.store.ListBranches(ctx, req.OrganizationId, project.ID)
		if err != nil {
			response.Errors = append(response.Errors, fmt.Sprintf("list branches for project %s: %v", project.ID, err))
			continue
		}

		for _, branch := range branches {
			err := p.store.DeleteBranch(ctx, req.OrganizationId, project.ID, branch.ID, func(b *store.Branch) error {
				return cells.DeprovisionBranch(ctx, req.OrganizationId, p.store, p.cells, b)
			})
			if err != nil {
				projectErrors = append(projectErrors, fmt.Sprintf("delete branch %s: %v", branch.ID, err))
				continue
			}
			response.BranchesDeleted++
			log.Ctx(ctx).Info().Msgf("Deleted branch [%s] in project [%s] for org [%s]", branch.ID, project.ID, req.OrganizationId)
		}

		if projectErrors == nil {
			if err := p.store.DeleteProject(ctx, req.OrganizationId, project.ID); err != nil {
				projectErrors = append(projectErrors, fmt.Sprintf("delete project %s: %v", project.ID, err))
			} else {
				response.ProjectsDeleted++
				log.Ctx(ctx).Info().Msgf("Deleted project [%s] for org [%s]", project.ID, req.OrganizationId)
			}
		}
		response.Errors = append(response.Errors, projectErrors...)
	}

	// The organization keeps no branches once every project is gone, so the
	// desired status has nothing left to apply to. Auth deletes the
	// organization itself after this call and never tells projects again.
	if len(response.Errors) == 0 {
		if err := p.store.DeleteOrganizationStatus(ctx, req.OrganizationId); err != nil {
			response.Errors = append(response.Errors, fmt.Sprintf("delete organization status: %v", err))
		}
	}

	return response, nil
}
