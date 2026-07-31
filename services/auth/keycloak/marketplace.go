package keycloak

import "fmt"

type MarketplaceAttributes interface {
	Validate() error
	BuildKeycloakAttributes() map[string][]string
}

const AWSMarketplaceProviderName = string(OrganizationMarketplaceProviderAWS)

type AWSMarketplace struct {
	CustomerID string
	ProductID  string
	AccountID  string
}

func (a AWSMarketplace) Validate() error {
	if a.CustomerID == "" {
		return fmt.Errorf("aws marketplace: customerID is required")
	}
	if a.ProductID == "" {
		return fmt.Errorf("aws marketplace: productID is required")
	}
	if a.AccountID == "" {
		return fmt.Errorf("aws marketplace: accountID is required")
	}
	return nil
}

func (a AWSMarketplace) BuildKeycloakAttributes() map[string][]string {
	return map[string][]string{
		OrganizationMarketplaceKey:   {AWSMarketplaceProviderName},
		OrganizationAWSCustomerIDKey: {a.CustomerID},
		OrganizationAWSProductIDKey:  {a.ProductID},
		OrganizationAWSAccountIDKey:  {a.AccountID},
	}
}

func AWSMarketplaceFromKeycloakAttributes(attributes map[string][]string) AWSMarketplace {
	marketplace := AWSMarketplace{}
	if v, ok := FirstAttr(attributes, OrganizationAWSCustomerIDKey); ok {
		marketplace.CustomerID = v
	}
	if v, ok := FirstAttr(attributes, OrganizationAWSProductIDKey); ok {
		marketplace.ProductID = v
	}
	if v, ok := FirstAttr(attributes, OrganizationAWSAccountIDKey); ok {
		marketplace.AccountID = v
	}
	return marketplace
}

const VercelMarketplaceProviderName = string(OrganizationMarketplaceProviderVercel)

// VercelMarketplace links an organization to a Vercel Marketplace installation.
// Unlike AWS, the marketplace identifiers come from the signed request claims
// (there is no acting Xata user), so the org carries them directly.
type VercelMarketplace struct {
	// InstallationID is the Vercel installation id (icfg_...) — the specific
	// installation of our integration, keyed 1:1 to this Xata org.
	InstallationID string
	// AccountID is the Vercel account/team (claims.account_id) that owns the
	// installation, i.e. the customer behind it. Descriptive/traceability only;
	// not unique per org since one account may hold several installations.
	AccountID string
	// Email is the account's contact email (from Get Account Information), used
	// for the Orb billing customer and kept on the org for support/traceability.
	Email string
}

func (v VercelMarketplace) Validate() error {
	if v.InstallationID == "" {
		return fmt.Errorf("vercel marketplace: installationID is required")
	}
	if v.AccountID == "" {
		return fmt.Errorf("vercel marketplace: accountID is required")
	}
	return nil
}

func (v VercelMarketplace) BuildKeycloakAttributes() map[string][]string {
	return map[string][]string{
		OrganizationMarketplaceKey:          {VercelMarketplaceProviderName},
		OrganizationVercelInstallationIDKey: {v.InstallationID},
		OrganizationVercelAccountIDKey:      {v.AccountID},
		OrganizationVercelAccountEmailKey:   {v.Email},
	}
}

func VercelMarketplaceFromKeycloakAttributes(attributes map[string][]string) VercelMarketplace {
	marketplace := VercelMarketplace{}
	if v, ok := FirstAttr(attributes, OrganizationVercelInstallationIDKey); ok {
		marketplace.InstallationID = v
	}
	if v, ok := FirstAttr(attributes, OrganizationVercelAccountIDKey); ok {
		marketplace.AccountID = v
	}
	if v, ok := FirstAttr(attributes, OrganizationVercelAccountEmailKey); ok {
		marketplace.Email = v
	}
	return marketplace
}
