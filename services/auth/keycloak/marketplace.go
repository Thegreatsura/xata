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
	if v, ok := firstAttr(attributes, OrganizationAWSCustomerIDKey); ok {
		marketplace.CustomerID = v
	}
	if v, ok := firstAttr(attributes, OrganizationAWSProductIDKey); ok {
		marketplace.ProductID = v
	}
	if v, ok := firstAttr(attributes, OrganizationAWSAccountIDKey); ok {
		marketplace.AccountID = v
	}
	return marketplace
}
