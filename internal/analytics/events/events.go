package events

import "time"

type Event struct {
	Name       string
	Properties map[string]any
	OrgID      string
	Timestamp  time.Time
}

func NewOrganizationCreatedEvent(organizationID string) Event {
	return Event{
		Name:  "organization created",
		OrgID: organizationID,
		Properties: map[string]any{
			"organization": organizationID,
		},
	}
}

func NewProjectCreatedEvent(organizationID, projectID string) Event {
	return Event{
		Name:  "project created",
		OrgID: organizationID,
		Properties: map[string]any{
			"organization": organizationID,
			"project":      projectID,
		},
	}
}

func NewProjectDeletedEvent(organizationID, projectID string) Event {
	return Event{
		Name:  "project deleted",
		OrgID: organizationID,
		Properties: map[string]any{
			"organization": organizationID,
			"project":      projectID,
		},
	}
}

func NewBranchFromConfigurationEvent(organizationID, projectID, branchID, region string, image, instanceType string, replicas int, storageSize *int32) Event {
	props := map[string]any{
		"organization":  organizationID,
		"project":       projectID,
		"branch":        branchID,
		"region":        region,
		"child_branch":  false,
		"image":         image,
		"instance_type": instanceType,
		"replicas":      replicas,
	}

	if storageSize != nil {
		props["storage_size"] = int(*storageSize)
	}

	return Event{
		Name:       "branch created",
		OrgID:      organizationID,
		Properties: props,
	}
}

func NewBranchFromParentEvent(organizationID, projectID, parentID, branchID, region string) Event {
	return Event{
		Name:  "branch created",
		OrgID: organizationID,
		Properties: map[string]any{
			"organization": organizationID,
			"project":      projectID,
			"branch":       branchID,
			"region":       region,
			"parent_id":    parentID,
			"child_branch": true,
		},
	}
}

func NewBranchDeletedEvent(organizationID, projectID, branchID string) Event {
	return Event{
		Name:  "branch deleted",
		OrgID: organizationID,
		Properties: map[string]any{
			"organization": organizationID,
			"project":      projectID,
			"branch":       branchID,
		},
	}
}

func NewProjectUpdatedEvent(organizationID, projectID string, changedFields []string, newValues map[string]any) Event {
	return Event{
		Name:  "project updated",
		OrgID: organizationID,
		Properties: map[string]any{
			"organization":   organizationID,
			"project":        projectID,
			"changed_fields": changedFields,
			"new_values":     newValues,
		},
	}
}

func NewBranchUpdatedEvent(organizationID, projectID, branchID string, changedFields []string, newValues map[string]any) Event {
	return Event{
		Name:  "branch updated",
		OrgID: organizationID,
		Properties: map[string]any{
			"organization":   organizationID,
			"project":        projectID,
			"branch":         branchID,
			"changed_fields": changedFields,
			"new_values":     newValues,
		},
	}
}

func NewPaymentMethodAttachedEvent(organizationID, provider string) Event {
	return Event{
		Name:  "payment method attached",
		OrgID: organizationID,
		Properties: map[string]any{
			"organization": organizationID,
			"provider":     provider,
		},
	}
}

func NewInvoicePaidEvent(organizationID, marketplace, currency string, amountDue, total float64, paidAt time.Time) Event {
	return Event{
		Name:      "invoice paid",
		OrgID:     organizationID,
		Timestamp: paidAt,
		Properties: map[string]any{
			"organization": organizationID,
			"marketplace":  marketplace,
			"amount_due":   amountDue,
			"total":        total,
			"currency":     currency,
		},
	}
}

func NewMemberInvitedEvent(organizationID, email string) Event {
	return Event{
		Name:  "member invited",
		OrgID: organizationID,
		Properties: map[string]any{
			"organization":  organizationID,
			"invitee_email": email,
		},
	}
}

func NewBranchRestoredFromBackupEvent(organizationID, projectID, sourceBranchID, newBranchID string) Event {
	return Event{
		Name:  "branch restored from backup",
		OrgID: organizationID,
		Properties: map[string]any{
			"organization":    organizationID,
			"project":         projectID,
			"source_branch":   sourceBranchID,
			"restored_branch": newBranchID,
		},
	}
}
