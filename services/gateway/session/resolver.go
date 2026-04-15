package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xata/internal/xvalidator"
)

var errMalformedHostname = errors.New("malformed hostname")

type BranchResolver interface {
	Resolve(ctx context.Context, serverName string) (*Branch, error)
}

type Branch struct {
	ID      string
	Address string
}

type CNPGBranchResolver struct {
	cnpgNamespace  string
	port           int
	knownEndpoints map[string]bool
}

func NewCNPGBranchResolver(cnpgNamespace string, port int, poolerEnabled bool) *CNPGBranchResolver {
	return &CNPGBranchResolver{
		cnpgNamespace: cnpgNamespace,
		port:          port,
		knownEndpoints: map[string]bool{
			"rw":     true,
			"ro":     true,
			"r":      true,
			"pooler": poolerEnabled,
		},
	}
}

func (r *CNPGBranchResolver) Resolve(ctx context.Context, serverName string) (*Branch, error) {
	branchName, endpointType, err := extractBranch(serverName, r.knownEndpoints)
	if err != nil {
		return nil, fmt.Errorf("read branch name: hostname [%s]: %w", serverName, err)
	}

	return &Branch{
		ID:      branchName,
		Address: fmt.Sprintf("branch-%s-%s.%s.svc.cluster.local:%d", branchName, endpointType, r.cnpgNamespace, r.port),
	}, nil
}

func extractBranch(fullHostname string, knownEndpoints map[string]bool) (string, string, error) {
	hostnamePart, _, found := strings.Cut(fullHostname, ".")
	if !found || hostnamePart == "" {
		return "", "", errMalformedHostname
	}

	branchName, endpointType := parseEndpoint(hostnamePart, knownEndpoints)

	if err := xvalidator.IsValidIdentifier(branchName); err != nil {
		return "", "", fmt.Errorf("invalid branch name: %w", err)
	}

	return branchName, endpointType, nil
}

// parseEndpoint splits a hostname part into a branch name and endpoint type
// by checking whether the segment after the last hyphen is a known endpoint.
// If no known endpoint suffix is found, the full hostname is the branch name
// and the endpoint defaults to "rw".
func parseEndpoint(hostnamePart string, knownEndpoints map[string]bool) (string, string) {
	lastHyphen := strings.LastIndex(hostnamePart, "-")
	if lastHyphen > 0 {
		suffix := hostnamePart[lastHyphen+1:]
		if knownEndpoints[suffix] {
			return hostnamePart[:lastHyphen], suffix
		}
	}
	return hostnamePart, "rw"
}

type ResolverFunc func(ctx context.Context, serverName string) (branch *Branch, err error)

func (f ResolverFunc) Resolve(ctx context.Context, serverName string) (*Branch, error) {
	return f(ctx, serverName)
}
