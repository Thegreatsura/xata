package clusters

import "xata/internal/storageqos"

// volumeAttributesClassNames maps storage QoS class names to corresponding VolumeAttributesClass
// names for each storageclass
var volumeAttributesClassNames = map[string]map[string]string{
	"xatastor": {
		storageqos.ClassMicro:   "xatastor-micro",
		storageqos.ClassSmall:   "xatastor-small",
		storageqos.ClassMedium:  "xatastor-medium",
		storageqos.ClassLarge:   "xatastor-large",
		storageqos.ClassXLarge:  "xatastor-xlarge",
		storageqos.Class2XLarge: "xatastor-2xlarge",
		storageqos.Class4XLarge: "xatastor-4xlarge",
		storageqos.Class8XLarge: "xatastor-8xlarge",
	},
}
