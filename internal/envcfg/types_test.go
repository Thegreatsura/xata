package envcfg

import (
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func TestTolerationListFieldSetValue(t *testing.T) {
	tests := map[string]struct {
		raw     string
		want    []v1.Toleration
		wantErr bool
	}{
		"one toleration": {
			raw: "foo=bar:NoSchedule",
			want: []v1.Toleration{
				{
					Key:      "foo",
					Operator: v1.TolerationOpEqual,
					Value:    "bar",
					Effect:   v1.TaintEffect("NoSchedule"),
				},
			},
		},
		"missing effect": {
			raw:     "foo=bar",
			wantErr: true,
		},
		"missing value and effect": {
			raw:     "foo",
			wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tl := TolerationListField{}
			err := tl.SetValue(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, tl.Value)
		})
	}
}
