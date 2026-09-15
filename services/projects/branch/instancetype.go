package branch

import (
	"context"
	"fmt"
	"strconv"

	"xata/services/projects/store"

	"k8s.io/apimachinery/pkg/api/resource"
)

// InstanceTypeByName returns the instance type with the given name, enforcing
// maxAllowedInstanceType (0 means no limit).
func (s *Service) InstanceTypeByName(ctx context.Context, organizationID, region, name string, maxAllowedInstanceType int) (store.InstanceType, error) {
	instanceTypes, err := s.store.ListInstanceTypes(ctx, organizationID, region)
	if err != nil {
		return store.InstanceType{}, err
	}
	for _, instance := range instanceTypes {
		if instance.Name == name {
			if maxAllowedInstanceType != 0 && instance.VCPUsRequest > maxAllowedInstanceType {
				return store.InstanceType{}, fmt.Errorf("instance type %s is not available on your current plan; please add a payment method in your billing settings or contact support to enable larger instances", name)
			}
			return instance, nil
		}
	}
	return store.InstanceType{}, fmt.Errorf("instance type %s is not found", name)
}

// InstanceTypeByResources returns the instance type name matching the given
// vcpu/memory resources, or the fallback "custom" type for combinations that do
// not map to a named instance type.
func (s *Service) InstanceTypeByResources(ctx context.Context, organizationID, region, cpuRequest, cpuLimit, memory string) (name string, err error) {
	instanceTypes, err := s.store.ListInstanceTypes(ctx, organizationID, region)
	if err != nil {
		return "", err
	}

	vcpusRequest, err := ParseCPUResource(cpuRequest)
	if err != nil {
		return "", err
	}

	vcpusLimit, err := ParseCPUResource(cpuLimit)
	if err != nil {
		return "", err
	}

	ram, err := strconv.Atoi(memory)
	if err != nil {
		return "", err
	}

	for _, instance := range instanceTypes {
		if instance.VCPUsRequest == vcpusRequest && instance.VCPUsLimit == vcpusLimit && instance.RAM == ram {
			return instance.Name, nil
		}
	}

	// for invalid combinations we will return the FallbackInstanceType defined as "custom" string as not to break the UI in the case we needed to amend the configs manually
	return FallbackInstanceType, nil
}

// ParseCPUResource parses a k8s cpu spec into milliCPUs.
func ParseCPUResource(cpuSpec string) (int, error) {
	quantity, err := resource.ParseQuantity(cpuSpec)
	if err != nil {
		return 0, fmt.Errorf("failed to parse cpu resource: %w", err)
	}
	return int(quantity.MilliValue()), nil
}
