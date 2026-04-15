package openapi

import (
	_ "embed"
)

//go:embed gen/public.yaml
var OpenAPIYAML string

//go:embed gen/openapi.json
var OpenAPIJSON string
