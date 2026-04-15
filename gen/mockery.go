package mocks

// generate mocks for all proto files, put them under protomocks package
//go:generate go run github.com/vektra/mockery/v3 --output protomocks --outpkg protomocks --with-expecter --recursive --name .+Client
