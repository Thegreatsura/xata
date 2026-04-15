package customerio

import (
	"context"
	"testing"

	"xata/internal/customerio/mocks"

	"github.com/customerio/go-customerio/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSafeEmail(t *testing.T) {
	tests := []struct {
		name         string
		isProduction bool
		input        string
		want         string
	}{
		{
			name:         "external email in production",
			isProduction: true,
			input:        "user@example.com",
			want:         "user@example.com",
		},
		{
			name:         "xata.io email in production",
			isProduction: true,
			input:        "engineer@xata.io",
			want:         "engineer@xata.io",
		},
		{
			name:         "external email redirected to test email",
			isProduction: false,
			input:        "user@example.com",
			want:         "testemails@xata.io",
		},
		{
			name:         "xata.io email allowed in non-production",
			isProduction: false,
			input:        "engineer@xata.io",
			want:         "engineer@xata.io",
		},
		{
			name:         "another external email redirected",
			isProduction: false,
			input:        "customer@company.com",
			want:         "testemails@xata.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				isProduction: tt.isProduction,
			}
			result := client.safeEmail(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name             string
		cfg              Config
		wantErr          bool
		wantErrContains  string
		wantIsProduction bool
	}{
		{
			name: "requires API key",
			cfg: Config{
				CustomerIoAPIKey:       "",
				CustomerIoIsProduction: false,
			},
			wantErr:         true,
			wantErrContains: "CUSTOMER_IO_API_KEY is required",
		},
		{
			name: "success with non-production",
			cfg: Config{
				CustomerIoAPIKey:       "test-api-key",
				CustomerIoIsProduction: false,
			},
			wantErr:          false,
			wantIsProduction: false,
		},
		{
			name: "success with production",
			cfg: Config{
				CustomerIoAPIKey:       "test-api-key",
				CustomerIoIsProduction: true,
			},
			wantErr:          false,
			wantIsProduction: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
				if tt.wantErrContains != "" {
					assert.Contains(t, err.Error(), tt.wantErrContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, tt.wantIsProduction, client.isProduction)
			}
		})
	}
}

func TestStructToMap(t *testing.T) {
	data := DummyTestEmailV1{
		UserName:         "John Doe",
		OrganizationName: "Acme Corp",
	}

	result, err := structToMap(data)
	assert.NoError(t, err)
	assert.Equal(t, "John Doe", result["user_name"])
	assert.Equal(t, "Acme Corp", result["organization_name"])
}

func TestMessageDataInterface(t *testing.T) {
	testEmail := DummyTestEmailV1{
		UserName:         "John Doe",
		OrganizationName: "Acme Corp",
	}
	assert.Equal(t, "dummy_test_email_v1", testEmail.TriggerName())
}

func TestSendTransactionalEmail(t *testing.T) {
	mockAPI := mocks.NewAPIClientInterface(t)

	mockAPI.EXPECT().SendEmail(
		mock.Anything,
		mock.MatchedBy(func(req *customerio.SendEmailRequest) bool {
			return req.To == "monica@xata.io" &&
				req.TransactionalMessageID == "dummy_test_email_v1" &&
				req.MessageData["user_name"] == "Monica Sarbu" &&
				req.MessageData["organization_name"] == "Xata" &&
				req.Identifiers["email"] == "monica@xata.io"
		}),
	).Return(&customerio.SendEmailResponse{}, nil)

	client := &Client{
		api:          mockAPI,
		isProduction: false,
	}

	messageData := DummyTestEmailV1{
		UserName:         "Monica Sarbu",
		OrganizationName: "Xata",
	}

	err := SendTransactionalEmail(client, context.Background(), "monica@xata.io", messageData)

	require.NoError(t, err)
	mockAPI.AssertExpectations(t)
}
