package extensions

//go:generate go run github.com/vektra/mockery/v3 --output mocks --outpkg mocks --with-expecter --name ExtensionsProvider

// ExtensionsProvider defines the interface for PostgreSQL extension operations
type ExtensionsProvider interface {
	GetExtensions(image string) []ExtensionSpec
	IsExtensionAvailable(image string, extensionName string) bool
	GetExtension(image string, extensionName string) *ExtensionSpec
	GetPreloadRequiredExtensions(image string) []ExtensionSpec
	GetAllOfferings() []string
	GetVersionsForOffering(offering string) []string
}

// DefaultExtensionsProvider is the default implementation of ExtensionsProvider
type DefaultExtensionsProvider struct{}

// GetExtensions implements ExtensionsProvider
func (p *DefaultExtensionsProvider) GetExtensions(image string) []ExtensionSpec {
	return GetExtensions(image)
}

// IsExtensionAvailable implements ExtensionsProvider
func (p *DefaultExtensionsProvider) IsExtensionAvailable(image string, extensionName string) bool {
	return IsExtensionAvailable(image, extensionName)
}

// GetExtension implements ExtensionsProvider
func (p *DefaultExtensionsProvider) GetExtension(image string, extensionName string) *ExtensionSpec {
	return GetExtension(image, extensionName)
}

// GetPreloadRequiredExtensions implements ExtensionsProvider
func (p *DefaultExtensionsProvider) GetPreloadRequiredExtensions(image string) []ExtensionSpec {
	return GetPreloadRequiredExtensions(image)
}

// GetAllOfferings implements ExtensionsProvider
func (p *DefaultExtensionsProvider) GetAllOfferings() []string {
	return GetAllOfferings()
}

// GetVersionsForOffering implements ExtensionsProvider
func (p *DefaultExtensionsProvider) GetVersionsForOffering(offering string) []string {
	return GetVersionsForOffering(offering)
}
