package keycloak

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAWSMarketplace_Validate(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		marketplace AWSMarketplace
		wantErr     string
	}{
		"all fields set": {
			marketplace: AWSMarketplace{
				CustomerID: "cust-1",
				ProductID:  "prod-1",
				AccountID:  "acct-1",
			},
			wantErr: "",
		},
		"missing customerID": {
			marketplace: AWSMarketplace{
				ProductID: "prod-1",
				AccountID: "acct-1",
			},
			wantErr: "aws marketplace: customerID is required",
		},
		"missing productID": {
			marketplace: AWSMarketplace{
				CustomerID: "cust-1",
				AccountID:  "acct-1",
			},
			wantErr: "aws marketplace: productID is required",
		},
		"missing accountID": {
			marketplace: AWSMarketplace{
				CustomerID: "cust-1",
				ProductID:  "prod-1",
			},
			wantErr: "aws marketplace: accountID is required",
		},
		"all empty": {
			marketplace: AWSMarketplace{},
			wantErr:     "aws marketplace: customerID is required",
		},
		"empty string customerID": {
			marketplace: AWSMarketplace{
				CustomerID: "",
				ProductID:  "prod-1",
				AccountID:  "acct-1",
			},
			wantErr: "aws marketplace: customerID is required",
		},
		"empty string productID": {
			marketplace: AWSMarketplace{
				CustomerID: "cust-1",
				ProductID:  "",
				AccountID:  "acct-1",
			},
			wantErr: "aws marketplace: productID is required",
		},
		"empty string accountID": {
			marketplace: AWSMarketplace{
				CustomerID: "cust-1",
				ProductID:  "prod-1",
				AccountID:  "",
			},
			wantErr: "aws marketplace: accountID is required",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := tc.marketplace.Validate()
			if tc.wantErr == "" {
				require.NoError(t, got)
				return
			}
			require.Error(t, got)
			assert.Equal(t, tc.wantErr, got.Error())
		})
	}
}

func TestAWSMarketplace_BuildKeycloakAttributes(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		marketplace AWSMarketplace
		want        map[string][]string
	}{
		"populated fields": {
			marketplace: AWSMarketplace{
				CustomerID: "cust-1",
				ProductID:  "prod-1",
				AccountID:  "acct-1",
			},
			want: map[string][]string{
				OrganizationMarketplaceKey:   {"aws"},
				OrganizationAWSCustomerIDKey: {"cust-1"},
				OrganizationAWSProductIDKey:  {"prod-1"},
				OrganizationAWSAccountIDKey:  {"acct-1"},
			},
		},
		"zero value": {
			marketplace: AWSMarketplace{},
			want: map[string][]string{
				OrganizationMarketplaceKey:   {"aws"},
				OrganizationAWSCustomerIDKey: {""},
				OrganizationAWSProductIDKey:  {""},
				OrganizationAWSAccountIDKey:  {""},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := tc.marketplace.BuildKeycloakAttributes()
			assert.Equal(t, tc.want, got)
		})
	}
}
