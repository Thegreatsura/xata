package ipfiltering

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "k8s.io/api/core/v1"
)

const (
	// ConfigMapName is the name of the ConfigMap storing branch IP filtering configuration
	ConfigMapName = "ipfiltering"
	// ConfigMapKey is the key in the ConfigMap storing the IP filtering JSON
	ConfigMapKey = "ipfiltering.json"
	// MaxRetries is the maximum number of retries for conflict resolution
	MaxRetries = 3
	// RetryDelay is the delay between retries
	RetryDelay = 100 * time.Millisecond
)

// IPFilteringConfig represents the IP filtering configuration for a branch
type IPFilteringConfig struct {
	Enabled bool     `json:"enabled"`
	Allowed []string `json:"allowed"`
}

// DefaultIPFilteringConfig returns the default IP filtering configuration (disabled)
func DefaultIPFilteringConfig() IPFilteringConfig {
	return IPFilteringConfig{
		Enabled: false,
		Allowed: []string{},
	}
}

// SetBranchIPFiltering sets the IP filtering configuration for a branch in the ConfigMap.
// It creates the ConfigMap if it doesn't exist and handles concurrent updates with retries.
// The ConfigMap stores all branch configs in a single JSON object under the key "ipfiltering.json".
func SetBranchIPFiltering(ctx context.Context, kubeClient client.Client, namespace, branchID string, config IPFilteringConfig) error {
	// Retry logic for handling conflicts
	for attempt := range MaxRetries {
		// Get the ConfigMap
		configMap := &v1.ConfigMap{}
		err := kubeClient.Get(ctx, client.ObjectKey{
			Name:      ConfigMapName,
			Namespace: namespace,
		}, configMap)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// ConfigMap doesn't exist, create it
				configMap = &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ConfigMapName,
						Namespace: namespace,
					},
					Data: make(map[string]string),
				}
			} else {
				return fmt.Errorf("getting ConfigMap: %w", err)
			}
		}

		// Initialize Data map if nil
		if configMap.Data == nil {
			configMap.Data = make(map[string]string)
		}

		// Parse existing JSON object (map[branchID]IPFilteringConfig)
		var allConfigs map[string]IPFilteringConfig
		if existingJSON, exists := configMap.Data[ConfigMapKey]; exists && existingJSON != "" {
			if err := json.Unmarshal([]byte(existingJSON), &allConfigs); err != nil {
				return fmt.Errorf("parsing existing IP filtering config: %w", err)
			}
		} else {
			// No existing config, start with empty map
			allConfigs = make(map[string]IPFilteringConfig)
		}

		// Update the branch entry
		allConfigs[branchID] = config

		// Marshal the entire config object back to JSON
		configJSON, err := json.Marshal(allConfigs)
		if err != nil {
			return fmt.Errorf("marshaling IP filtering config: %w", err)
		}

		// Update the ConfigMap with the new JSON
		configMap.Data[ConfigMapKey] = string(configJSON)

		// Try to create or update the ConfigMap
		if configMap.UID == "" {
			// ConfigMap doesn't exist, create it
			if err := kubeClient.Create(ctx, configMap); err != nil {
				if (apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)) && attempt < MaxRetries-1 {
					// Retry on conflict or already exists (race condition)
					time.Sleep(RetryDelay)
					continue
				}
				return fmt.Errorf("creating ConfigMap: %w", err)
			}
			return nil
		}

		// Update the ConfigMap
		if err := kubeClient.Update(ctx, configMap); err != nil {
			if apierrors.IsConflict(err) && attempt < MaxRetries-1 {
				// Retry on conflict - will refetch in next iteration
				time.Sleep(RetryDelay)
				continue
			}
			return fmt.Errorf("updating ConfigMap: %w", err)
		}
		return nil
	}

	return fmt.Errorf("updating ConfigMap after %d retries", MaxRetries)
}

// SetBranchesIPFiltering sets the IP filtering configuration for multiple branches in the ConfigMap.
// It creates the ConfigMap if it doesn't exist and handles concurrent updates with retries.
// The ConfigMap stores all branch configs in a single JSON object under the key "ipfiltering.json".
func SetBranchesIPFiltering(ctx context.Context, kubeClient client.Client, namespace string, branchIDs []string, config IPFilteringConfig) error {
	if len(branchIDs) == 0 {
		return nil // Nothing to do
	}

	// Retry logic for handling conflicts
	for attempt := range MaxRetries {
		// Get the ConfigMap
		configMap := &v1.ConfigMap{}
		err := kubeClient.Get(ctx, client.ObjectKey{
			Name:      ConfigMapName,
			Namespace: namespace,
		}, configMap)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// ConfigMap doesn't exist, create it
				configMap = &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ConfigMapName,
						Namespace: namespace,
					},
					Data: make(map[string]string),
				}
			} else {
				return fmt.Errorf("getting ConfigMap: %w", err)
			}
		}

		// Initialize Data map if nil
		if configMap.Data == nil {
			configMap.Data = make(map[string]string)
		}

		// Parse existing JSON object (map[branchID]IPFilteringConfig)
		var allConfigs map[string]IPFilteringConfig
		if existingJSON, exists := configMap.Data[ConfigMapKey]; exists && existingJSON != "" {
			if err := json.Unmarshal([]byte(existingJSON), &allConfigs); err != nil {
				return fmt.Errorf("parsing existing IP filtering config: %w", err)
			}
		} else {
			// No existing config, start with empty map
			allConfigs = make(map[string]IPFilteringConfig)
		}

		// Update all branch entries with the same config
		for _, branchID := range branchIDs {
			allConfigs[branchID] = config
		}

		// Marshal the entire config object back to JSON
		configJSON, err := json.Marshal(allConfigs)
		if err != nil {
			return fmt.Errorf("marshaling IP filtering config: %w", err)
		}

		// Update the ConfigMap with the new JSON
		configMap.Data[ConfigMapKey] = string(configJSON)

		// Try to create or update the ConfigMap
		if configMap.UID == "" {
			// ConfigMap doesn't exist, create it
			if err := kubeClient.Create(ctx, configMap); err != nil {
				if (apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)) && attempt < MaxRetries-1 {
					// Retry on conflict or already exists (race condition)
					time.Sleep(RetryDelay)
					continue
				}
				return fmt.Errorf("creating ConfigMap: %w", err)
			}
			return nil
		}

		// Update the ConfigMap
		if err := kubeClient.Update(ctx, configMap); err != nil {
			if apierrors.IsConflict(err) && attempt < MaxRetries-1 {
				// Retry on conflict - will refetch in next iteration
				time.Sleep(RetryDelay)
				continue
			}
			return fmt.Errorf("updating ConfigMap: %w", err)
		}
		return nil
	}

	return fmt.Errorf("updating ConfigMap after %d retries", MaxRetries)
}

// GetBranchIPFiltering retrieves the IP filtering configuration for a branch from the ConfigMap.
// Returns the default configuration (disabled) if the ConfigMap or branch entry doesn't exist.
func GetBranchIPFiltering(ctx context.Context, kubeClient client.Client, namespace, branchID string) (IPFilteringConfig, error) {
	configMap := &v1.ConfigMap{}
	err := kubeClient.Get(ctx, client.ObjectKey{
		Name:      ConfigMapName,
		Namespace: namespace,
	}, configMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// ConfigMap doesn't exist, return default
			return DefaultIPFilteringConfig(), nil
		}
		return DefaultIPFilteringConfig(), fmt.Errorf("getting ConfigMap: %w", err)
	}

	// Check if the config JSON exists
	configJSON, exists := configMap.Data[ConfigMapKey]
	if !exists || configJSON == "" {
		// No config data, return default
		return DefaultIPFilteringConfig(), nil
	}

	// Parse the JSON object (map[branchID]IPFilteringConfig)
	var allConfigs map[string]IPFilteringConfig
	if err := json.Unmarshal([]byte(configJSON), &allConfigs); err != nil {
		// Invalid JSON, return default
		return DefaultIPFilteringConfig(), fmt.Errorf("unmarshaling IP filtering config: %w", err)
	}

	// Check if the branch entry exists
	config, exists := allConfigs[branchID]
	if !exists {
		// Branch entry doesn't exist, return default
		return DefaultIPFilteringConfig(), nil
	}

	return config, nil
}

// DeleteBranchIPFiltering removes the IP filtering configuration entry for a branch from the ConfigMap.
// It handles concurrent updates with retries and is idempotent (no error if entry doesn't exist).
func DeleteBranchIPFiltering(ctx context.Context, kubeClient client.Client, namespace, branchID string) error {
	// Retry logic for handling conflicts
	for attempt := range MaxRetries {
		// Get the ConfigMap
		configMap := &v1.ConfigMap{}
		err := kubeClient.Get(ctx, client.ObjectKey{
			Name:      ConfigMapName,
			Namespace: namespace,
		}, configMap)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// ConfigMap doesn't exist, nothing to delete - idempotent
				return nil
			}
			return fmt.Errorf("getting ConfigMap: %w", err)
		}

		// Check if the config JSON exists
		if configMap.Data == nil {
			// No data, nothing to delete - idempotent
			return nil
		}

		configJSON, exists := configMap.Data[ConfigMapKey]
		if !exists || configJSON == "" {
			// No config data, nothing to delete - idempotent
			return nil
		}

		// Parse the JSON object (map[branchID]IPFilteringConfig)
		var allConfigs map[string]IPFilteringConfig
		if err := json.Unmarshal([]byte(configJSON), &allConfigs); err != nil {
			return fmt.Errorf("parsing existing IP filtering config: %w", err)
		}

		// Check if the branch entry exists
		if _, exists := allConfigs[branchID]; !exists {
			// Branch entry doesn't exist, nothing to delete - idempotent
			return nil
		}

		// Delete the branch entry
		delete(allConfigs, branchID)

		// Marshal the updated config object back to JSON
		updatedJSON, err := json.Marshal(allConfigs)
		if err != nil {
			return fmt.Errorf("marshaling IP filtering config: %w", err)
		}

		// Update the ConfigMap with the new JSON
		configMap.Data[ConfigMapKey] = string(updatedJSON)

		// Update the ConfigMap
		if err := kubeClient.Update(ctx, configMap); err != nil {
			if apierrors.IsConflict(err) && attempt < MaxRetries-1 {
				// Retry on conflict - will refetch in next iteration
				time.Sleep(RetryDelay)
				continue
			}
			return fmt.Errorf("updating ConfigMap: %w", err)
		}
		return nil
	}

	return fmt.Errorf("deleting branch entry from ConfigMap after %d retries", MaxRetries)
}
