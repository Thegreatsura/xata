package envcfg

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
)

type TolerationListField struct {
	Value []v1.Toleration
}

func (tl *TolerationListField) SetValue(raw string) error {
	if raw == "" {
		return fmt.Errorf("tolerations are required but not set")
	}

	entries := strings.Split(raw, ",")
	list := make([]v1.Toleration, 0, len(entries))

	for _, entry := range entries {
		// Format: key=value:effect
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid toleration format: %s", entry)
		}
		keyValue := strings.SplitN(parts[0], "=", 2)
		if len(keyValue) != 2 {
			return fmt.Errorf("invalid key=value in toleration: %s", parts[0])
		}
		t := v1.Toleration{
			Key:      keyValue[0],
			Value:    keyValue[1],
			Operator: v1.TolerationOpEqual,
			Effect:   v1.TaintEffect(parts[1]),
		}
		list = append(list, t)
	}
	tl.Value = list
	return nil
}
