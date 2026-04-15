package spec

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=types.cfg.yml ../../../../openapi/gen/bundled/gateway.yaml
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=server.cfg.yml ../../../../openapi/gen/bundled/gateway.yaml
//go:generate go run mvdan.cc/gofumpt -w -modpath xata .
