// Package storageqos defines the storage QoS class names shared between the
// control plane and the dataplane. The values cross the projects->clusters
// gRPC boundary in the storage_qos_class fields. The dataplane maps these
// values to VolumeAttributesClass names for each storageclass.
package storageqos

const (
	ClassMicro   = "micro"
	ClassSmall   = "small"
	ClassMedium  = "medium"
	ClassLarge   = "large"
	ClassXLarge  = "xlarge"
	Class2XLarge = "2xlarge"
	Class4XLarge = "4xlarge"
	Class8XLarge = "8xlarge"
)
