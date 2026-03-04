.PHONY: build lint sqlc test

build:
	goimports -w .
	go get ./...
	go mod tidy
	go build -o main .

lint:
	golangci-lint run ./...

sqlc:
	sqlc generate
