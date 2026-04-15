package postgrescfg

//go:generate go run github.com/vektra/mockery/v3 --output mocks --outpkg mocks --with-expecter --name PostgresConfigProvider

import "maps"

import "fmt"

// PostgresConfigProvider defines the interface for PostgreSQL configuration operations
type PostgresConfigProvider interface {
	GetDefaultPostgresParameters(instanceType string, majorVersion int, image string, preloadLibraries []string) (map[string]string, error)
	GetParametersSpec(instanceType string, majorVersion int, image string, preloadLibraries []string) (ParametersMap, error)
	ValidateSettings(instanceType string, settings map[string]string, majorVersion int, image string, preloadLibraries []string) (map[string]error, error)
	GetDefaultPreloadLibraries(image string) ([]string, error)
	ValidatePreloadLibraries(image string, libraries []string) error
	FilterConfigurableParameters(params map[string]string, majorVersion int, image string, preloadLibraries []string) map[string]string
	GetConfigurableParameters(majorVersion int, image string, preloadLibraries []string) map[string]PostgresParameterSpec
}

// DefaultPostgresConfigProvider is the default implementation of PostgresConfigProvider
type DefaultPostgresConfigProvider struct{}

// GetDefaultPostgresParameters implements PostgresConfigProvider
func (p *DefaultPostgresConfigProvider) GetDefaultPostgresParameters(instanceType string, majorVersion int, image string, preloadLibraries []string) (map[string]string, error) {
	return GetDefaultPostgresParameters(instanceType, majorVersion, image, preloadLibraries)
}

// GetParametersSpec implements PostgresConfigProvider
func (p *DefaultPostgresConfigProvider) GetParametersSpec(instanceType string, majorVersion int, image string, preloadLibraries []string) (ParametersMap, error) {
	return GetParametersSpec(instanceType, majorVersion, image, preloadLibraries)
}

// ValidateSettings implements PostgresConfigProvider
func (p *DefaultPostgresConfigProvider) ValidateSettings(instanceType string, settings map[string]string, majorVersion int, image string, preloadLibraries []string) (map[string]error, error) {
	return ValidateSettings(instanceType, settings, majorVersion, image, preloadLibraries)
}

// GetDefaultPreloadLibraries implements PostgresConfigProvider
func (p *DefaultPostgresConfigProvider) GetDefaultPreloadLibraries(image string) ([]string, error) {
	return GetDefaultPreloadLibraries(image)
}

// ValidatePreloadLibraries implements PostgresConfigProvider
func (p *DefaultPostgresConfigProvider) ValidatePreloadLibraries(image string, libraries []string) error {
	return ValidatePreloadLibraries(image, libraries)
}

// FilterConfigurableParameters implements PostgresConfigProvider
func (p *DefaultPostgresConfigProvider) FilterConfigurableParameters(params map[string]string, majorVersion int, image string, preloadLibraries []string) map[string]string {
	return FilterConfigurableParameters(params, majorVersion, image, preloadLibraries)
}

// GetConfigurableParameters implements PostgresConfigProvider
func (p *DefaultPostgresConfigProvider) GetConfigurableParameters(majorVersion int, image string, preloadLibraries []string) map[string]PostgresParameterSpec {
	return GetConfigurableParameters(majorVersion, image, preloadLibraries)
}

// ConfigValueType represents the type of a configuration value
type ConfigValueType string

const (
	// ConfigValueDefault indicates the value is the PostgreSQL default
	ConfigValueDefault ConfigValueType = "default"
	// ConfigValueInstanceDefault indicates the value is the default for the specific instance size
	ConfigValueInstanceDefault ConfigValueType = "instance_default"
	// ConfigValueCustom indicates the value is custom set by the user
	ConfigValueCustom ConfigValueType = "custom"
)

type ParametersMap map[string]PostgresParameterSpec

// xataMicroOverrides is the default parameters for the xata.micro instance type
//
// pgtune:
// # DB Version: 17
// # OS Type: linux
// # DB Type: mixed
// # Total Memory (RAM): 1 GB
// # CPUs num: 1
// # Connections num: 50
// # Data Storage: san
var xataMicroOverrides ParametersMap = ParametersMap{
	"max_connections":              {DefaultValue: "50", MaxValue: "150"},
	"shared_buffers":               {DefaultValue: "256MB", MaxValue: "700MB"},
	"effective_cache_size":         {DefaultValue: "768MB"},
	"maintenance_work_mem":         {DefaultValue: "64MB"},
	"checkpoint_completion_target": {DefaultValue: "0.9"},
	"wal_buffers":                  {DefaultValue: "7864kB"},
	"default_statistics_target":    {DefaultValue: "100"},
	"random_page_cost":             {DefaultValue: "1.1"},
	"effective_io_concurrency":     {DefaultValue: "32"},
	"work_mem":                     {DefaultValue: "2259kB"},
	"huge_pages":                   {DefaultValue: "off"},
	"min_wal_size":                 {DefaultValue: "1GB"},
	"max_wal_size":                 {DefaultValue: "4GB"},
}

// smallDefaultParameters is the default parameters for the xata.small instance type
//
// pgtune:
// # DB Version: 17
// # OS Type: linux
// # DB Type: mixed
// # Total Memory (RAM): 2 GB
// # CPUs num: 1
// # Connections num: 100
// # Data Storage: san
var smallDefaultParameters ParametersMap = ParametersMap{
	"max_connections":              {DefaultValue: "100", MaxValue: "200"},
	"shared_buffers":               {DefaultValue: "512MB", MaxValue: "1.5GB"},
	"effective_cache_size":         {DefaultValue: "1536MB"},
	"maintenance_work_mem":         {DefaultValue: "128MB"},
	"checkpoint_completion_target": {DefaultValue: "0.9"},
	"wal_buffers":                  {DefaultValue: "16MB"},
	"default_statistics_target":    {DefaultValue: "100"},
	"random_page_cost":             {DefaultValue: "1.1"},
	"effective_io_concurrency":     {DefaultValue: "32"},
	"work_mem":                     {DefaultValue: "2427kB"},
	"huge_pages":                   {DefaultValue: "off"},
	"min_wal_size":                 {DefaultValue: "1GB"},
	"max_wal_size":                 {DefaultValue: "4GB"},
}

// mediumDefaultParameters is the default parameters for the xata.medium instance type
// pgtune:
// # DB Version: 17
// # OS Type: linux
// # DB Type: mixed
// # Total Memory (RAM): 4 GB
// # CPUs num: 1
// # Connections num: 200
// # Data Storage: san
var mediumDefaultParameters ParametersMap = ParametersMap{
	"max_connections":              {DefaultValue: "200", MaxValue: "400"},
	"shared_buffers":               {DefaultValue: "1GB", MaxValue: "3GB"},
	"effective_cache_size":         {DefaultValue: "3GB"},
	"maintenance_work_mem":         {DefaultValue: "256MB"},
	"checkpoint_completion_target": {DefaultValue: "0.9"},
	"wal_buffers":                  {DefaultValue: "16MB"},
	"default_statistics_target":    {DefaultValue: "100"},
	"random_page_cost":             {DefaultValue: "1.1"},
	"effective_io_concurrency":     {DefaultValue: "32"},
	"work_mem":                     {DefaultValue: "2520kB"},
	"huge_pages":                   {DefaultValue: "off"},
	"min_wal_size":                 {DefaultValue: "1GB"},
	"max_wal_size":                 {DefaultValue: "4GB"},
}

// largeDefaultParameters is the default parameters for the xata.large instance type
//
// pgtune:
// # DB Version: 17
// # OS Type: linux
// # DB Type: mixed
// # Total Memory (RAM): 8 GB
// # CPUs num: 2
// # Connections num: 400
// # Data Storage: san
var largeDefaultParameters ParametersMap = ParametersMap{
	"max_connections":              {DefaultValue: "400", MaxValue: "800"},
	"shared_buffers":               {DefaultValue: "2GB", MaxValue: "6GB"},
	"effective_cache_size":         {DefaultValue: "6GB"},
	"maintenance_work_mem":         {DefaultValue: "512MB"},
	"checkpoint_completion_target": {DefaultValue: "0.9"},
	"wal_buffers":                  {DefaultValue: "16MB"},
	"default_statistics_target":    {DefaultValue: "100"},
	"random_page_cost":             {DefaultValue: "1.1"},
	"effective_io_concurrency":     {DefaultValue: "32"},
	"work_mem":                     {DefaultValue: "2570kB"},
	"huge_pages":                   {DefaultValue: "off"},
	"min_wal_size":                 {DefaultValue: "1GB"},
	"max_wal_size":                 {DefaultValue: "4GB"},
}

// xataXLargeDefaultParameters is the default parameters for the xata.xlarge instance type
//
// pgtune:
// # DB Version: 17
// # OS Type: linux
// # DB Type: mixed
// # Total Memory (RAM): 16 GB
// # CPUs num: 4
// # Connections num: 800
// # Data Storage: san
var xataXLargeDefaultParameters ParametersMap = ParametersMap{
	"max_connections":                  {DefaultValue: "800", MaxValue: "1600"},
	"shared_buffers":                   {DefaultValue: "4GB", MaxValue: "12GB"},
	"effective_cache_size":             {DefaultValue: "12GB"},
	"maintenance_work_mem":             {DefaultValue: "1GB"},
	"checkpoint_completion_target":     {DefaultValue: "0.9"},
	"wal_buffers":                      {DefaultValue: "16MB"},
	"default_statistics_target":        {DefaultValue: "100"},
	"random_page_cost":                 {DefaultValue: "1.1"},
	"effective_io_concurrency":         {DefaultValue: "32"},
	"work_mem":                         {DefaultValue: "2608kB"},
	"huge_pages":                       {DefaultValue: "off"},
	"min_wal_size":                     {DefaultValue: "1GB"},
	"max_wal_size":                     {DefaultValue: "4GB"},
	"max_worker_processes":             {DefaultValue: "4"},
	"max_parallel_workers_per_gather":  {DefaultValue: "2"},
	"max_parallel_workers":             {DefaultValue: "4"},
	"max_parallel_maintenance_workers": {DefaultValue: "2"},
}

// xata2XLargeDefaultParameters is the default parameters for the xata.2xlarge instance type
//
// pgtune:
// # DB Version: 17
// # OS Type: linux
// # DB Type: mixed
// # Total Memory (RAM): 32 GB
// # CPUs num: 8
// # Connections num: 1600
// # Data Storage: san
var xata2XLargeDefaultParameters ParametersMap = ParametersMap{
	"max_connections":                  {DefaultValue: "1600", MaxValue: "3200"},
	"shared_buffers":                   {DefaultValue: "8GB", MaxValue: "24GB"},
	"effective_cache_size":             {DefaultValue: "24GB"},
	"maintenance_work_mem":             {DefaultValue: "2GB"},
	"checkpoint_completion_target":     {DefaultValue: "0.9"},
	"wal_buffers":                      {DefaultValue: "16MB"},
	"default_statistics_target":        {DefaultValue: "100"},
	"random_page_cost":                 {DefaultValue: "1.1"},
	"effective_io_concurrency":         {DefaultValue: "32"},
	"work_mem":                         {DefaultValue: "2608kB"},
	"huge_pages":                       {DefaultValue: "try"},
	"min_wal_size":                     {DefaultValue: "1GB"},
	"max_wal_size":                     {DefaultValue: "4GB"},
	"max_worker_processes":             {DefaultValue: "8"},
	"max_parallel_workers_per_gather":  {DefaultValue: "4"},
	"max_parallel_workers":             {DefaultValue: "8"},
	"max_parallel_maintenance_workers": {DefaultValue: "4"},
}

// xata4XLargeDefaultParameters is the default parameters for the xata.4xlarge instance type
//
// pgtune:
// # DB Version: 17
// # OS Type: linux
// # DB Type: mixed
// # Total Memory (RAM): 64 GB
// # CPUs num: 16
// # Connections num: 3200
// # Data Storage: san
var xata4XLargeDefaultParameters ParametersMap = ParametersMap{
	"max_connections":                  {DefaultValue: "3200", MaxValue: "5000"},
	"shared_buffers":                   {DefaultValue: "16GB", MaxValue: "48GB"},
	"effective_cache_size":             {DefaultValue: "48GB"},
	"maintenance_work_mem":             {DefaultValue: "2GB"},
	"checkpoint_completion_target":     {DefaultValue: "0.9"},
	"wal_buffers":                      {DefaultValue: "16MB"},
	"default_statistics_target":        {DefaultValue: "100"},
	"random_page_cost":                 {DefaultValue: "1.1"},
	"effective_io_concurrency":         {DefaultValue: "32"},
	"work_mem":                         {DefaultValue: "2608kB"},
	"huge_pages":                       {DefaultValue: "try"},
	"min_wal_size":                     {DefaultValue: "1GB"},
	"max_wal_size":                     {DefaultValue: "4GB"},
	"max_worker_processes":             {DefaultValue: "16"},
	"max_parallel_workers_per_gather":  {DefaultValue: "4"},
	"max_parallel_workers":             {DefaultValue: "16"},
	"max_parallel_maintenance_workers": {DefaultValue: "4"},
}

// xata8XLargeDefaultParameters is the default parameters for the xata.8xlarge instance type
//
// pgtune:
// # DB Version: 17
// # OS Type: linux
// # DB Type: mixed
// # Total Memory (RAM): 128 GB
// # CPUs num: 32
// # Connections num: 5000
// # Data Storage: san
var xata8XLargeDefaultParameters ParametersMap = ParametersMap{
	"max_connections":                  {DefaultValue: "5000", MaxValue: "5000"},
	"shared_buffers":                   {DefaultValue: "32GB", MaxValue: "96GB"},
	"effective_cache_size":             {DefaultValue: "96GB"},
	"maintenance_work_mem":             {DefaultValue: "2GB"},
	"checkpoint_completion_target":     {DefaultValue: "0.9"},
	"wal_buffers":                      {DefaultValue: "16MB"},
	"default_statistics_target":        {DefaultValue: "100"},
	"random_page_cost":                 {DefaultValue: "1.1"},
	"effective_io_concurrency":         {DefaultValue: "32"},
	"work_mem":                         {DefaultValue: "3334kB"},
	"huge_pages":                       {DefaultValue: "try"},
	"min_wal_size":                     {DefaultValue: "1GB"},
	"max_wal_size":                     {DefaultValue: "4GB"},
	"max_worker_processes":             {DefaultValue: "32"},
	"max_parallel_workers_per_gather":  {DefaultValue: "4"},
	"max_parallel_workers":             {DefaultValue: "32"},
	"max_parallel_maintenance_workers": {DefaultValue: "4"},
}

var defaultParametersByInstanceType map[string]ParametersMap = map[string]ParametersMap{
	"xata.micro":   xataMicroOverrides,
	"xata.small":   smallDefaultParameters,
	"xata.medium":  mediumDefaultParameters,
	"xata.large":   largeDefaultParameters,
	"xata.xlarge":  xataXLargeDefaultParameters,
	"xata.2xlarge": xata2XLargeDefaultParameters,
	"xata.4xlarge": xata4XLargeDefaultParameters,
	"xata.8xlarge": xata8XLargeDefaultParameters,
}

func GetDefaultPostgresConfigByInstanceType(instanceName string) (ParametersMap, error) {
	params, exists := defaultParametersByInstanceType[instanceName]
	if !exists {
		return ParametersMap{}, fmt.Errorf("unknown instance size %s", instanceName)
	}
	return params, nil
}

func MergeParametersMaps(base, override ParametersMap) ParametersMap {
	merged := make(ParametersMap, len(base))

	// Copy base into merged
	maps.Copy(merged, base)

	// Apply overrides
	for k, overrideVal := range override {
		if baseVal, exists := merged[k]; exists {
			// Merge fields: override non-zero fields from overrideVal
			if overrideVal.DefaultValue != "" {
				baseVal.DefaultValue = overrideVal.DefaultValue
			}
			if overrideVal.MinValue != "" {
				baseVal.MinValue = overrideVal.MinValue
			}
			if overrideVal.MaxValue != "" {
				baseVal.MaxValue = overrideVal.MaxValue
			}
			if overrideVal.Description != "" {
				baseVal.Description = overrideVal.Description
			}
			if len(overrideVal.Values) > 0 {
				baseVal.Values = overrideVal.Values
			}
			merged[k] = baseVal
		} else {
			panic(fmt.Sprintf("override parameter %q not found in base map", k))
		}
	}

	return merged
}

func GetDefaultPostgresParameters(instanceType string, majorVersion int, image string, preloadLibraries []string) (map[string]string, error) {
	overrides, err := GetDefaultPostgresConfigByInstanceType(instanceType)
	if err != nil {
		return nil, err
	}
	merged := MergeParametersMaps(GetConfigurableParameters(majorVersion, image, preloadLibraries), overrides)

	params := make(map[string]string)
	for k, v := range merged {
		params[k] = v.DefaultValue
	}

	return params, nil
}

func GetParametersSpec(instanceType string, majorVersion int, image string, preloadLibraries []string) (ParametersMap, error) {
	overrides, err := GetDefaultPostgresConfigByInstanceType(instanceType)
	if err != nil {
		return ParametersMap{}, err
	}
	merged := MergeParametersMaps(GetConfigurableParameters(majorVersion, image, preloadLibraries), overrides)

	return merged, nil
}

// DetermineConfigValueType determines whether a parameter value is the PostgreSQL default,
// the default for the given instance size, or a custom value set by the user.
func DetermineConfigValueType(instanceSize, paramName, paramValue string, majorVersion int, image string, preloadLibraries []string) (ConfigValueType, error) {
	// Get the configurable parameters to check against PostgreSQL defaults
	configurableParams := GetConfigurableParameters(majorVersion, image, preloadLibraries)

	// Check if this is a known configurable parameter
	postgresDefault, exists := configurableParams[paramName]
	if !exists {
		return ConfigValueCustom, fmt.Errorf("unknown configurable parameter: %s", paramName)
	}

	// Get instance-specific defaults first
	instanceParams, err := GetDefaultPostgresConfigByInstanceType(instanceSize)
	if err != nil {
		return ConfigValueCustom, fmt.Errorf("failed to get instance config: %w", err)
	}

	// Check if the value matches the instance-specific default first (more specific)
	if instanceParam, hasInstanceDefault := instanceParams[paramName]; hasInstanceDefault {
		if instanceParam.DefaultValue == paramValue {
			return ConfigValueInstanceDefault, nil
		}
	}

	// If it's a known configurable parameter, check against PostgreSQL default
	if postgresDefault.DefaultValue == paramValue {
		return ConfigValueDefault, nil
	}

	// If it doesn't match either default, it's custom
	return ConfigValueCustom, nil
}
