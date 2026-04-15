package policy

import rego.v1

default allow := false

allow if {
	valid_input
	valid_scope
	valid_organization
	valid_project
	valid_branch
}

# Input validation checks
valid_input if {
	input.request.method != ""
	input.request.path != ""
	input.request.scopes != null
	is_array(input.request.scopes)

	input.claims.organizations != null
	is_object(input.claims.organizations)
	input.claims.scopes != null
	is_array(input.claims.scopes)
	input.claims.projects != null
	is_array(input.claims.projects)
	input.claims.branches != null
	is_array(input.claims.branches)
}

# Scope check: all required scopes must be granted, unless wildcard is present
valid_scope if {
	"*" in input.claims.scopes
} else if {
	every scope in input.request.scopes {
		scope_allowed(scope, input.claims.scopes)
	}
}

scope_allowed(scope, claims) if {
	scope in claims
} else if {
	# Allow read scopes to be satisfied by corresponding write scopes
	# e.g., "project:read" can be satisfied by "project:write"
	endswith(scope, ":read")
	replace(scope, ":read", ":write") in claims
}

# Organization access check
valid_organization if {
	not contains(input.request.path, ":organizationID")
} else if {
	input.request.organization in object.keys(input.claims.organizations)
}

# Project access check
valid_project if {
	not contains(input.request.path, ":projectID")
} else if {
	permission_granted(input.request.project, input.claims.projects)
}

# Branch access check
valid_branch if {
	not contains(input.request.path, ":branchID")
} else if {
	permission_granted(input.request.branch, input.claims.branches)
}

# Permission check with support for "*"
permission_granted(value, allowed) if {
	"*" in allowed
} else if {
	value in allowed
}
