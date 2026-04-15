package opa

import _ "embed"

//go:embed policy/policy.rego
var Policy string

type RequestInput struct {
	Method       string   `json:"method"`
	Path         string   `json:"path"`
	Scopes       []string `json:"scopes"`
	Organization string   `json:"organization"`
	Project      string   `json:"project"`
	Branch       string   `json:"branch"`
}

type ClaimsInput struct {
	Scopes        []string                `json:"scopes"`
	Organizations map[string]Organization `json:"organizations"`
	Projects      []string                `json:"projects"`
	Branches      []string                `json:"branches"`
}

type Organization struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type PolicyInput struct {
	Request RequestInput `json:"request"`
	Claims  ClaimsInput  `json:"claims"`
}
