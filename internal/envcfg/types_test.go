package envcfg

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
)

func Test_Tolerations(t *testing.T) {
	type args struct {
		s []string
	}
	tests := []struct {
		name    string
		args    args
		want    []v1.Toleration
		wantErr bool
	}{
		{
			name: "key=value:effect",
			args: args{
				s: []string{"foo=bar:NoSchedule"},
			},
			want: []v1.Toleration{
				{
					Key:      "foo",
					Operator: v1.TolerationOpEqual,
					Value:    "bar",
					Effect:   v1.TaintEffect("NoSchedule"),
				},
			},
			wantErr: false,
		},
		{
			name: "key=value",
			args: args{
				s: []string{"foo=bar"},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "key",
			args: args{
				s: []string{"foo"},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl := TolerationListField{}
			err := tl.SetValue(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetValue error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tl.Value, tt.want) {
				t.Errorf("SetValue = %v, want %v", tl.Value, tt.want)
			}
		})
	}
}
