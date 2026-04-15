package policy_test

import data.policy

# valid_input tests
test_valid_input_allow if {
	policy.valid_input with input as {
		"request": {
			"method": "GET",
			"path": "/path",
			"scopes": [],
		},
		"claims": {
			"scopes": [],
			"organizations": {},
			"projects": [],
			"branches": [],
		},
	}
}

test_valid_input_deny_missing_request_method if {
	not policy.valid_input with input as {
		"request": {
			"path": "/path",
			"scopes": [],
		},
		"claims": {
			"scopes": [],
			"organizations": {},
			"projects": [],
			"branches": [],
		},
	}
}

# valid_scope tests
test_valid_scope_allow_wildcard if {
	policy.valid_scope with input as {
		"request": {"scopes": ["read"]},
		"claims": {"scopes": ["*"]},
	}
}

test_valid_scope_allow_exact_match if {
	policy.valid_scope with input as {
		"request": {"scopes": ["read"]},
		"claims": {"scopes": ["read"]},
	}
}

test_valid_scope_deny_no_match if {
	not policy.valid_scope with input as {
		"request": {"scopes": ["write"]},
		"claims": {"scopes": ["read"]},
	}
}

# valid_organization tests
test_valid_organization_allow_no_org_in_path if {
	policy.valid_organization with input as {
		"request": {"path": "/path"},
		"claims": {"organizations": {"org1": {"ID": "org1", "Status": "enabled"}}},
	}
}

test_valid_organization_allow_match if {
	policy.valid_organization with input as {
		"request": {"path": "/orgs/:organizationID", "organization": "org1"},
		"claims": {"organizations": {"org1": {"ID": "org1", "Status": "enabled"}}},
	}
}

test_valid_organization_deny_no_match if {
	not policy.valid_organization with input as {
		"request": {"path": "/orgs/:organizationID", "organization": "org2"},
		"claims": {"organizations": {"org1": {"ID": "org1", "Status": "enabled"}}},
	}
}

# valid_project tests
test_valid_project_allow_no_project_in_path if {
	policy.valid_project with input as {
		"request": {"path": "/path"},
		"claims": {"projects": ["proj1"]},
	}
}

test_valid_project_allow_wildcard if {
	policy.valid_project with input as {
		"request": {"path": "/projects/:projectID", "project": "proj1"},
		"claims": {"projects": ["*"]},
	}
}

test_valid_project_allow_match if {
	policy.valid_project with input as {
		"request": {"path": "/projects/:projectID", "project": "proj1"},
		"claims": {"projects": ["proj1"]},
	}
}

test_valid_project_deny_no_match if {
	not policy.valid_project with input as {
		"request": {"path": "/projects/:projectID", "project": "proj2"},
		"claims": {"projects": ["proj1"]},
	}
}

# valid_branch tests
test_valid_branch_allow_no_branch_in_path if {
	policy.valid_branch with input as {
		"request": {"path": "/path"},
		"claims": {"branches": ["main"]},
	}
}

test_valid_branch_allow_wildcard if {
	policy.valid_branch with input as {
		"request": {"path": "/branches/:branchID", "branch": "dev"},
		"claims": {"branches": ["*"]},
	}
}

test_valid_branch_allow_match if {
	policy.valid_branch with input as {
		"request": {"path": "/branches/:branchID", "branch": "main"},
		"claims": {"branches": ["main"]},
	}
}

test_valid_branch_deny_no_match if {
	not policy.valid_branch with input as {
		"request": {"path": "/branches/:branchID", "branch": "dev"},
		"claims": {"branches": ["main"]},
	}
}

# allow rule tests
test_allow_simple_get if {
	policy.allow with input as {
		"request": {
			"method": "GET",
			"path": "/path",
			"scopes": ["read"],
		},
		"claims": {
			"scopes": ["read"],
			"organizations": {},
			"projects": [],
			"branches": [],
		},
	}
}

test_deny_missing_scope if {
	not policy.allow with input as {
		"request": {
			"method": "GET",
			"path": "/path",
			"scopes": ["read", "write"],
		},
		"claims": {
			"scopes": ["read"],
			"organizations": {},
			"projects": [],
			"branches": [],
		},
	}
}

test_allow_org_project_branch_access if {
	policy.allow with input as {
		"request": {
			"method": "POST",
			"path": "/orgs/:organizationID/projects/:projectID/branches/:branchID",
			"scopes": ["write"],
			"organization": "myorg",
			"project": "myproject",
			"branch": "mybranch",
		},
		"claims": {
			"scopes": ["write"],
			"organizations": {"myorg": {"ID": "myorg", "Status": "enabled"}},
			"projects": ["myproject"],
			"branches": ["mybranch"],
		},
	}
}

test_deny_org_mismatch if {
	not policy.allow with input as {
		"request": {
			"method": "POST",
			"path": "/orgs/:organizationID/projects/:projectID/branches/:branchID",
			"scopes": ["write"],
			"organization": "otherorg",
			"project": "myproject",
			"branch": "mybranch",
		},
		"claims": {
			"scopes": ["write"],
			"organizations": {"myorg": {"ID": "myorg", "Status": "enabled"}},
			"projects": ["myproject"],
			"branches": ["mybranch"],
		},
	}
}

test_valid_scope_allow_write_implies_read if {
	policy.valid_scope with input as {
		"request": {"scopes": ["branch:read"]},
		"claims": {"scopes": ["branch:write"]},
	}
}

test_valid_scope_deny_read_does_not_imply_write if {
	not policy.valid_scope with input as {
		"request": {"scopes": ["branch:write"]},
		"claims": {"scopes": ["branch:read"]},
	}
}

test_valid_scope_allow_mixed_read_and_write if {
	policy.valid_scope with input as {
		"request": {"scopes": ["branch:read", "branch:write"]},
		"claims": {"scopes": ["branch:write"]},
	}
}

test_valid_scope_deny_partial_missing_write if {
	not policy.valid_scope with input as {
		"request": {"scopes": ["branch:read", "branch:write"]},
		"claims": {"scopes": ["branch:read"]},
	}
}
