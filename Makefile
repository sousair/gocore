BIN := $(PWD)/bin

.PHONY: tools test lint

tools:
	GOBIN=$(BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

test:
	go test ./...

lint:
	go vet ./...
	$(BIN)/golangci-lint run ./...
