package clusters

import (
	stderrors "errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/errors"
)

// errBackupsNotEnabled is returned when an update sets the backup
// configuration of a branch that has no backups.
var errBackupsNotEnabled = stderrors.New("backups are not enabled on the branch")

// ClusterNotFoundError is returned when trying to retrieve/update/delete a cluster that does not exist
func ClusterNotFoundError(clusterID string) error {
	return status.Errorf(codes.NotFound, "cluster [%s] not found", clusterID)
}

// SecretNotFoundForIDError is returned when trying to retrieve/update/delete a secret that does not exist
func SecretNotFoundForIDError(id string) error {
	return status.Errorf(codes.NotFound, "secret not found for ID: [%s]", id)
}

// k8sErrorToGRPCError maps Kubernetes API errors to gRPC errors
func k8sErrorToGRPCError(err error) error {
	switch {
	case errors.IsNotFound(err):
		return status.Errorf(codes.NotFound, "resource not found: %v", err)
	case errors.IsAlreadyExists(err):
		return status.Errorf(codes.AlreadyExists, "resource already exists: %v", err)
	case errors.IsInvalid(err):
		return status.Errorf(codes.InvalidArgument, "invalid resource: %v", err)
	case errors.IsForbidden(err):
		return status.Errorf(codes.PermissionDenied, "forbidden: %v", err)
	case errors.IsConflict(err):
		return status.Errorf(codes.Aborted, "conflict: %v", err)
	default:
		return status.Errorf(codes.Unknown, "kubernetes error: %v", err)
	}
}
