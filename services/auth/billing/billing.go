package billing

import (
	"context"
	"time"

	authv1 "xata/gen/proto/auth/v1"
)

//go:generate go run github.com/vektra/mockery/v3 --output billingmock --outpkg billingmock --with-expecter --name Client

const (
	TrialCreditExpiryDays      = 15
	CreditStatusActive         = "active"
	CreditStatusPendingPayment = "pending_payment"
)

type CollectionMethod string

const (
	CollectionMethodUnknown             CollectionMethod = "unknown"
	CollectionMethodStripePaymentMethod CollectionMethod = "stripe_payment_method"
	CollectionMethodMarketplace         CollectionMethod = "marketplace"
	CollectionMethodBankTransfer        CollectionMethod = "bank_transfer"
)

type SubscriptionStatus string

const SubscriptionStatusActive SubscriptionStatus = "active"

type Subscription struct {
	ID                 string
	Status             SubscriptionStatus
	AutoCollection     bool
	NetTerms           int
	DefaultInvoiceMemo string
}

type OrbBankTransferOptions struct {
	ExternalCustomerID string
	StripeCustomerID   string
	SubscriptionID     string
	NetTerms           int
	InvoiceMemo        string
}

type Credit struct {
	ID                    string
	Amount                float64
	MaximumInitialBalance float64
	EffectiveDate         time.Time
	ExpiryDate            time.Time
	Status                string
}

func (c *Credit) IsExpired() bool {
	// The orb client uses a zero time to indicate no expiry
	return !c.ExpiryDate.IsZero() && time.Now().After(c.ExpiryDate)
}

func (c Credit) IsActive(now time.Time) bool {
	if c.Status != CreditStatusActive || c.Amount <= 0 {
		return false
	}
	if !c.EffectiveDate.IsZero() && c.EffectiveDate.After(now) {
		return false
	}
	return c.ExpiryDate.IsZero() || c.ExpiryDate.After(now)
}

type PaymentMethodCard struct {
	Brand       string
	Last4       string
	ExpiryMonth int
	ExpiryYear  int
}

type PaymentMethod struct {
	Card PaymentMethodCard
}

type Customer struct {
	ID                   string
	CustomerID           string
	CustomerExternalID   string
	Name                 string
	Email                string
	PaymentProvider      string
	PaymentProviderID    string
	AutoCollection       bool
	Subscriptions        []Subscription
	Credits              []Credit
	DefaultPaymentMethod *PaymentMethod
	// HasValidPaymentMethod indicates Stripe has a supported default payment method. Use CanCollectPayment for collection decisions because marketplace billing does not require one.
	HasValidPaymentMethod bool
	Organization          *authv1.Organization
}

func (c *Customer) CollectionMethod() CollectionMethod {
	if c == nil {
		return CollectionMethodUnknown
	}
	// Auth normalizes unsupported collection methods to "unknown" before returning organizations, so this cast only produces declared CollectionMethod values.
	return CollectionMethod(c.Organization.GetBillingCollectionMethod())
}

func (c *Customer) CanCollectPayment() bool {
	method := c.CollectionMethod()
	return method == CollectionMethodMarketplace ||
		method == CollectionMethodBankTransfer ||
		(method == CollectionMethodStripePaymentMethod && c.HasValidPaymentMethod)
}

func (c *Customer) CurrentActiveCredit() float64 {
	var credit float64
	for _, credits := range c.Credits {
		if !credits.IsExpired() && credits.Amount > 0 {
			credit += float64(credits.Amount)
		}
	}

	return credit
}

func (c *Customer) TotalLifetimeCredits() float64 {
	var total float64
	for _, credit := range c.Credits {
		total += credit.MaximumInitialBalance
	}
	return total
}

func (c *Customer) ActiveCredits(now time.Time) []Credit {
	if c == nil {
		return nil
	}

	active := make([]Credit, 0, len(c.Credits))
	for _, credit := range c.Credits {
		if credit.IsActive(now) {
			active = append(active, credit)
		}
	}
	return active
}

func (c *Customer) TotalActiveCredits(now time.Time) float64 {
	if c == nil {
		return 0
	}

	var total float64
	for _, credit := range c.Credits {
		if credit.IsActive(now) {
			total += credit.Amount
		}
	}
	return total
}

func (c *Customer) LastExpiry() *time.Time {
	if c == nil {
		return nil
	}

	var last *time.Time
	for _, credit := range c.Credits {
		last = laterExpiry(last, credit.ExpiryDate)
	}
	return last
}

func (c *Customer) LastExpiryWithBalance() *time.Time {
	if c == nil {
		return nil
	}

	var last *time.Time
	for _, credit := range c.Credits {
		if credit.Amount > 0 {
			last = laterExpiry(last, credit.ExpiryDate)
		}
	}
	return last
}

func (c *Customer) LastActiveCreditExpiry(now time.Time) *time.Time {
	if c == nil {
		return nil
	}

	var last *time.Time
	for _, credit := range c.Credits {
		if credit.IsActive(now) {
			last = laterExpiry(last, credit.ExpiryDate)
		}
	}
	return last
}

func laterExpiry(current *time.Time, candidate time.Time) *time.Time {
	if candidate.IsZero() || (current != nil && !candidate.After(*current)) {
		return current
	}
	return &candidate
}

type InvoiceAutoCollection struct {
	NumAttempts   int
	NextAttemptAt *time.Time
}

type InvoiceStatus string

const InvoiceStatusIssued InvoiceStatus = "issued"

type Invoice struct {
	ID                 string
	InvoiceNumber      string
	AmountDue          float64
	AmountDueCents     int64
	Total              float64
	Currency           string
	Status             InvoiceStatus
	WillAutoIssue      bool
	InvoiceDate        time.Time
	BillingPeriodStart time.Time
	DueDate            time.Time
	IssuedAt           time.Time
	PaidAt             time.Time
	InvoicePDF         *string
	AutoCollection     InvoiceAutoCollection
}

type InvoiceListOptions struct {
	Cursor   string
	Limit    int
	Statuses []InvoiceStatus
}

type InvoicesPage struct {
	Data       []Invoice
	HasMore    bool
	NextCursor *string
}

type UpcomingInvoice struct {
	CreatedAt        time.Time
	AmountDue        float64
	Currency         string
	TargetDate       time.Time
	Subtotal         float64
	Total            float64
	HostedInvoiceURL *string
}

type OrbCustomerMetadata struct {
	Marketplace string
}

type CreateCustomerResult struct {
	CreditAmount int
}

func (m OrbCustomerMetadata) Values() map[string]string {
	if m.Marketplace == "" {
		return nil
	}
	return map[string]string{"marketplace": m.Marketplace}
}

type StripeCustomer struct {
	ID             string
	OrganizationID string
}

type BankTransferAddress struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

type FindStripeBankTransferCustomerOptions struct {
	OrganizationID         string
	LinkedStripeCustomerID string
}

type ConfigureStripeBankTransferCustomerOptions struct {
	OrganizationID   string
	StripeCustomerID string
	Email            string
	Address          BankTransferAddress
}

type StripeFundingInstructions struct {
	Currency          string
	Network           string
	AccountHolderName string
	BankName          string
	AccountNumber     string
	RoutingNumber     string
	AccountType       string
}

type StripeBankTransferResult struct {
	CustomerID          string
	Created             bool
	FundingInstructions StripeFundingInstructions
}

type StripeCashBalanceTransaction struct {
	ID                   string
	CustomerID           string
	Currency             string
	NetAmount            float64
	FundedByBankTransfer bool
}

type CheckoutSession struct {
	URL string
}

type PaymentMethodSession struct {
	URL string
}

type StripePaymentMethodCard struct {
	ID    string
	Last4 string
}

type Client interface {
	// CreateCustomer creates a new customer in the billing system
	CreateCustomer(ctx context.Context, name, email, externalCustomerID string, organizationsCount int, metadata OrbCustomerMetadata) (CreateCustomerResult, error)
	// FetchOrbCustomer retrieves a customer using its Orb internal customer ID.
	FetchOrbCustomer(ctx context.Context, customerID string) (*Customer, error)
	// FetchCustomerByExternalID retrieves a customer record by the external customer ID.
	FetchCustomerByExternalID(ctx context.Context, externalCustomerID string) (*Customer, error)
	// FetchCustomerByStripeCustomerID retrieves a customer using its Stripe customer ID.
	FetchCustomerByStripeCustomerID(ctx context.Context, stripeCustomerID string) (*Customer, error)
	// ConfigureOrbCustomerForBankTransfers configures an Orb customer and subscription for manual collection through Stripe.
	ConfigureOrbCustomerForBankTransfers(ctx context.Context, opts OrbBankTransferOptions) error
	// FetchBillingCustomerWithDefaultPaymentMethod retrieves a customer with their default payment method details.
	FetchBillingCustomerWithDefaultPaymentMethod(ctx context.Context, externalCustomerID string) (*Customer, error)
	UpdateOrbCustomerEmail(ctx context.Context, externalCustomerID, email string) (*Customer, error)
	ListInvoices(ctx context.Context, externalCustomerID string, opts InvoiceListOptions) (*InvoicesPage, error)
	// FetchUpcomingInvoice fetches the next invoice for the customer's active subscription.
	// Customers are expected to have at most one active subscription.
	FetchUpcomingInvoice(ctx context.Context, externalCustomerID string) (*UpcomingInvoice, error)
	// ListCustomersCreatedAfter retrieves customer records created at or after the given time.
	ListCustomersCreatedAfter(ctx context.Context, createdAfter time.Time) ([]*Customer, error)
	// FetchStripeCustomer retrieves a Stripe customer by their Stripe customer ID.
	FetchStripeCustomer(ctx context.Context, stripeCustomerID string) (*StripeCustomer, error)
	// FindExistingStripeCustomerForBankTransfers returns a safe existing Stripe customer ID without writes.
	FindExistingStripeCustomerForBankTransfers(ctx context.Context, opts FindStripeBankTransferCustomerOptions) (stripeCustomerID string, err error)
	// ConfigureStripeCustomerForBankTransfers creates or configures a Stripe customer and ensures USD ACH funding instructions.
	ConfigureStripeCustomerForBankTransfers(ctx context.Context, opts ConfigureStripeBankTransferCustomerOptions) (StripeBankTransferResult, error)
	// FetchStripeCashBalanceTransaction retrieves a Stripe cash balance transaction by its customer and transaction IDs.
	FetchStripeCashBalanceTransaction(ctx context.Context, stripeCustomerID, transactionID string) (*StripeCashBalanceTransaction, error)
	// FetchPaymentIntentPaymentMethodID retrieves the payment method attached to a Stripe payment intent.
	FetchPaymentIntentPaymentMethodID(ctx context.Context, paymentIntentID string) (string, error)
	// FetchSetupIntentPaymentMethodID retrieves the payment method attached to a Stripe setup intent.
	FetchSetupIntentPaymentMethodID(ctx context.Context, setupIntentID string) (string, error)
	// FetchStripePaymentMethodCard retrieves Stripe card details for a payment method.
	FetchStripePaymentMethodCard(ctx context.Context, paymentMethodID string) (*StripePaymentMethodCard, error)
	// FetchInvoice retrieves an orb invoice. invoiceID is an Orb internal invoice id
	FetchInvoice(ctx context.Context, invoiceID string) (*Invoice, error)
	// EnsureDefaultPaymentMethod sets paymentMethodID as the Stripe customer's default payment method
	// if it is not already set.
	EnsureDefaultPaymentMethod(ctx context.Context, stripeCustomerID, paymentMethodID string) error
	// SetOrbCustomerStripeChargeProvider configures the Orb customer to charge the Stripe customer.
	SetOrbCustomerStripeChargeProvider(ctx context.Context, externalCustomerID, stripeCustomerID string) error
	// RefundPaymentIntent creates a Stripe refund for a payment intent.
	RefundPaymentIntent(ctx context.Context, paymentIntentID string, metadata map[string]string, idempotencyKey string) error
	// HasValidDefaultPaymentMethod returns true if the Stripe customer has a valid default payment method.
	HasValidDefaultPaymentMethod(ctx context.Context, stripeCustomerID string) (bool, error)
	// VoidInvoice voids an invoice in the billing system.
	VoidInvoice(ctx context.Context, invoiceID string) error
	// CountPendingInvoices returns the number of issued (unpaid) invoices that have had at least one payment attempt for the given external customer ID.
	CountPendingInvoices(ctx context.Context, externalCustomerID string) (int, error)
	// HasOutstandingInvoices returns true if the customer has any issued invoices or draft invoices with a non-zero total.
	HasOutstandingInvoices(ctx context.Context, externalCustomerID string) (bool, error)
	// FinalizeSubscription cancels any active Orb subscriptions for the customer immediately.
	FinalizeSubscription(ctx context.Context, externalCustomerID string) error
	// DeletePaymentMethod detaches the customer's default Stripe payment method, if any.
	DeletePaymentMethod(ctx context.Context, externalCustomerID string) error
	CreateStripeCheckoutSession(ctx context.Context, externalCustomerID string) (*CheckoutSession, error)
	CreateStripePaymentMethodSession(ctx context.Context, externalCustomerID string) (*PaymentMethodSession, error)
}

type NoopBilling struct{}

func (n *NoopBilling) CreateCustomer(ctx context.Context, name, email, externalCustomerID string, organizationsCount int, metadata OrbCustomerMetadata) (CreateCustomerResult, error) {
	return CreateCustomerResult{}, nil
}

func (n *NoopBilling) FetchOrbCustomer(ctx context.Context, customerID string) (*Customer, error) {
	return nil, nil
}

func (n *NoopBilling) FetchCustomerByExternalID(_ context.Context, _ string) (*Customer, error) {
	return nil, nil
}

func (n *NoopBilling) FetchCustomerByStripeCustomerID(_ context.Context, _ string) (*Customer, error) {
	return nil, nil
}

func (n *NoopBilling) ConfigureOrbCustomerForBankTransfers(_ context.Context, _ OrbBankTransferOptions) error {
	return nil
}

func (n *NoopBilling) FetchBillingCustomerWithDefaultPaymentMethod(_ context.Context, _ string) (*Customer, error) {
	return nil, nil
}

func (n *NoopBilling) UpdateOrbCustomerEmail(_ context.Context, _, _ string) (*Customer, error) {
	return nil, nil
}

func (n *NoopBilling) ListInvoices(_ context.Context, _ string, _ InvoiceListOptions) (*InvoicesPage, error) {
	return nil, nil
}

func (n *NoopBilling) FetchUpcomingInvoice(_ context.Context, _ string) (*UpcomingInvoice, error) {
	return nil, nil
}

func (n *NoopBilling) ListCustomersCreatedAfter(_ context.Context, _ time.Time) ([]*Customer, error) {
	return nil, nil
}

func (n *NoopBilling) FetchInvoice(_ context.Context, _ string) (*Invoice, error) {
	return nil, nil
}

func (n *NoopBilling) FetchStripeCustomer(_ context.Context, _ string) (*StripeCustomer, error) {
	return nil, nil
}

func (n *NoopBilling) FindExistingStripeCustomerForBankTransfers(_ context.Context, _ FindStripeBankTransferCustomerOptions) (string, error) {
	return "", nil
}

func (n *NoopBilling) ConfigureStripeCustomerForBankTransfers(_ context.Context, _ ConfigureStripeBankTransferCustomerOptions) (StripeBankTransferResult, error) {
	return StripeBankTransferResult{}, nil
}

func (n *NoopBilling) FetchStripeCashBalanceTransaction(_ context.Context, _, _ string) (*StripeCashBalanceTransaction, error) {
	return nil, nil
}

func (n *NoopBilling) FetchPaymentIntentPaymentMethodID(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (n *NoopBilling) FetchSetupIntentPaymentMethodID(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (n *NoopBilling) FetchStripePaymentMethodCard(_ context.Context, _ string) (*StripePaymentMethodCard, error) {
	return nil, nil
}

func (n *NoopBilling) EnsureDefaultPaymentMethod(_ context.Context, _, _ string) error {
	return nil
}

func (n *NoopBilling) SetOrbCustomerStripeChargeProvider(_ context.Context, _, _ string) error {
	return nil
}

func (n *NoopBilling) RefundPaymentIntent(_ context.Context, _ string, _ map[string]string, _ string) error {
	return nil
}

func (n *NoopBilling) HasValidDefaultPaymentMethod(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (n *NoopBilling) VoidInvoice(_ context.Context, _ string) error {
	return nil
}

func (n *NoopBilling) CountPendingInvoices(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (n *NoopBilling) HasOutstandingInvoices(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (n *NoopBilling) FinalizeSubscription(_ context.Context, _ string) error {
	return nil
}

func (n *NoopBilling) DeletePaymentMethod(_ context.Context, _ string) error {
	return nil
}

func (n *NoopBilling) CreateStripeCheckoutSession(_ context.Context, _ string) (*CheckoutSession, error) {
	return nil, nil
}

func (n *NoopBilling) CreateStripePaymentMethodSession(_ context.Context, _ string) (*PaymentMethodSession, error) {
	return nil, nil
}
