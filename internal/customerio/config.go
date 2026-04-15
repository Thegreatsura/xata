package customerio

type Config struct {
	CustomerIoAPIKey       string `env:"CUSTOMER_IO_API_KEY" env-description:"Customer.io API key for connecting to customer.io 'app' API"`
	CustomerIoIsProduction bool   `env:"CUSTOMER_IO_IS_PRODUCTION" env-default:"false" env-description:"Whether to send emails to actual recipients (true) or redirect to testemails@xata.io (false)"`
}
