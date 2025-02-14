.PHONY: test
test:
	go test $(GOTEST_FLAGS) -race ./...

.PHONY: lint
lint:
	go tool golangci-lint run

.PHONY: fmt
fmt:
	go tool gofumpt -l -w .

.PHONY: install-tools
install-tools:
	go mod tidy

.PHONY: generate
generate:
	go generate ./...

