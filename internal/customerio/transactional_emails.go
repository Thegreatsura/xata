package customerio

type EmailMessageData interface {
	TriggerName() string
}

type DummyTestEmailV1 struct {
	UserName         string `json:"user_name"`
	OrganizationName string `json:"organization_name"`
}

func (DummyTestEmailV1) TriggerName() string {
	return "dummy_test_email_v1"
}
