package opa

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/open-policy-agent/opa/v1/rego"
)

//go:embed policy/policy.rego
var policyModule string

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

// normalized turns nil slices and maps into empty ones; a nil value marshals to null, which the policy's valid_input rule rejects.
func (in PolicyInput) normalized() PolicyInput {
	in.Request.Scopes = orEmpty(in.Request.Scopes)
	in.Claims.Scopes = orEmpty(in.Claims.Scopes)
	in.Claims.Projects = orEmpty(in.Claims.Projects)
	in.Claims.Branches = orEmpty(in.Claims.Branches)
	if in.Claims.Organizations == nil {
		in.Claims.Organizations = map[string]Organization{}
	}
	return in
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Policy is the compiled authorization policy. Evaluate through Allow, which normalizes the input.
type Policy struct {
	query rego.PreparedEvalQuery
}

// NewPolicy compiles and prepares the embedded authorization policy.
func NewPolicy(ctx context.Context) (*Policy, error) {
	query, err := rego.New(
		rego.Query("data.policy.allow"),
		rego.Module("policy.rego", policyModule),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("prepare authorization policy: %w", err)
	}
	return &Policy{query: query}, nil
}

// Allow reports whether the policy permits the request.
func (p *Policy) Allow(ctx context.Context, input PolicyInput) (bool, error) {
	res, err := p.query.Eval(ctx, rego.EvalInput(input.normalized()))
	if err != nil {
		return false, fmt.Errorf("evaluate authorization policy: %w", err)
	}
	return res.Allowed(), nil
}
