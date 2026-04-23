package signoz

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -generate types,client -include-operation-ids QueryRangeV5 -o client.go -package signoz https://raw.githubusercontent.com/SigNoz/signoz/refs/tags/v0.117.1/docs/api/openapi.yml
