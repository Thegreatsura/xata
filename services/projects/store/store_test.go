package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstanceTypeCPURequest(t *testing.T) {
	tests := []struct {
		milliCPUs int
		want      string
	}{
		{milliCPUs: 0, want: "0m"},
		{milliCPUs: 250, want: "250m"},
		{milliCPUs: 500, want: "500m"},
		{milliCPUs: 999, want: "999m"},
		{milliCPUs: 1000, want: "1"},
		{milliCPUs: 2000, want: "2"},
		{milliCPUs: 3500, want: "3"},
		{milliCPUs: 32000, want: "32"},
	}

	for _, tt := range tests {
		got := InstanceType{VCPUsRequest: tt.milliCPUs}.CPURequest()
		require.Equal(t, tt.want, got, "milliCPUs=%d", tt.milliCPUs)
	}
}

func TestInstanceTypeCPULimit(t *testing.T) {
	tests := []struct {
		milliCPUs int
		want      string
	}{
		{milliCPUs: 500, want: "500m"},
		{milliCPUs: 2000, want: "2"},
	}

	for _, tt := range tests {
		got := InstanceType{VCPUsLimit: tt.milliCPUs}.CPULimit()
		require.Equal(t, tt.want, got, "milliCPUs=%d", tt.milliCPUs)
	}
}

func TestInstanceTypeMemory(t *testing.T) {
	tests := []struct {
		ram  int
		want string
	}{
		{ram: 1, want: "1Gi"},
		{ram: 128, want: "128Gi"},
	}

	for _, tt := range tests {
		got := InstanceType{RAM: tt.ram}.Memory()
		require.Equal(t, tt.want, got, "ram=%d", tt.ram)
	}
}
