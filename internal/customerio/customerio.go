// Package customerio provides a client for sending transactional emails via the Customer.io API.
package customerio

//go:generate go run github.com/vektra/mockery/v3 --output mocks --outpkg mocks --with-expecter --name APIClientInterface

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/customerio/go-customerio/v3"
	"github.com/rs/zerolog/log"
)

type APIClientInterface interface {
	SendEmail(ctx context.Context, req *customerio.SendEmailRequest) (*customerio.SendEmailResponse, error)
}

type Client struct {
	api          APIClientInterface
	isProduction bool
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.CustomerIoAPIKey == "" {
		return nil, fmt.Errorf("CUSTOMER_IO_API_KEY is required")
	}

	apiClient := customerio.NewAPIClient(cfg.CustomerIoAPIKey, customerio.WithRegion(customerio.RegionEU))
	return &Client{api: apiClient, isProduction: cfg.CustomerIoIsProduction}, nil
}

const safeEmailRecipient = "testemails@xata.io"

func (c *Client) safeEmail(toEmail string) string {
	if c.isProduction {
		return toEmail
	}

	if strings.HasSuffix(toEmail, "@xata.io") {
		return toEmail
	}

	return safeEmailRecipient
}

func SendTransactionalEmail[T EmailMessageData](c *Client, ctx context.Context, to string, messageData T) error {
	messageDataMap, err := structToMap(messageData)
	if err != nil {
		return fmt.Errorf("failed to convert message data: %w", err)
	}

	safeToEmail := c.safeEmail(to)

	// customer.io documentation: https://docs.customer.io/journeys/transactional-email/#examples-and-api-parameters (click 'with template' and 'trigger name')
	request := customerio.SendEmailRequest{
		To:                     safeToEmail,
		TransactionalMessageID: messageData.TriggerName(),
		MessageData:            messageDataMap,
		// We always use email as the identifier at Xata
		Identifiers: map[string]string{
			"email": safeToEmail,
		},
	}

	log.Info().
		Str("original to", to).
		Bool("is_production", c.isProduction).
		Str("to (safe)", safeToEmail).
		Str("transactional_message_id", messageData.TriggerName()).
		Interface("message_data", messageDataMap).
		Msg("Sending Customer.io transactional email")

	_, err = c.api.SendEmail(ctx, &request)
	if err != nil {
		log.Error().
			Err(err).
			Str("transactional_message_id", messageData.TriggerName()).
			Msg("Failed to send Customer.io transactional email")
		return fmt.Errorf("failed to send Customer.io email: %w", err)
	}

	return nil
}

func structToMap(data any) (map[string]any, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}

	return result, nil
}
