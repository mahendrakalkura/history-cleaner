.PHONY: build lint test run benchmarks

build:
	goimports -w .
	go get ./...
	go mod tidy
	go build -o main .

lint:
	golangci-lint run ./...

test:
	go test -v -race -coverprofile=coverage.out ./...
	@echo ""
	@echo "Coverage:"
	@go tool cover -func=coverage.out | grep total | awk '{print $$3}'

run: build
	./main

benchmarks:
	go test -bench=. -benchmem ./...
