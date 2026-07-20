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

// storageQoSClassForVAC returns the storage QoS class whose
// VolumeAttributesClass is vac under the given storage class
func storageQoSClassForVAC(storageClass, vac string) (string, bool) {
	for class, vacName := range volumeAttributesClassNames[storageClass] {
		if vacName == vac {
			return class, true
		}
	}
	return "", false
}
