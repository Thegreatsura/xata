package keycloak

type User struct {
	ID            string              `json:"id"`
	Username      string              `json:"username"`
	FirstName     string              `json:"firstName"`
	LastName      string              `json:"lastName"`
	Email         string              `json:"email"`
	EmailVerified bool                `json:"emailVerified"`
	Attributes    map[string][]string `json:"attributes,omitempty"`

	Marketplace   string `json:"-"`
	AWSCustomerID string `json:"-"`
	AWSProductID  string `json:"-"`
	AWSAccountID  string `json:"-"`
}

type UserAttributesUpdate struct {
	Marketplace             *string `json:"marketplace,omitempty"`
	MarketplaceRegisteredAt *string `json:"marketplaceRegisteredAt,omitempty"`
	AWSAccountID            *string `json:"awsAccountId,omitempty"`
	AWSCustomerID           *string `json:"awsCustomerId,omitempty"`
	AWSProductID            *string `json:"awsProductId,omitempty"`
}
