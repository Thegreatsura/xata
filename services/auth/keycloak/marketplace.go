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
