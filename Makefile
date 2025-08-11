.PHONY: tidy run build

tidy:
	go mod tidy

run:
	go run ./cmd/server

build:
	go build -o bin/im ./cmd/server 