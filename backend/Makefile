.PHONY: server portail test build

server:
	VIVANT_DEV=1 go run ./cmd/vivant-server

portail:
	go run ./cmd/demo-portail

test:
	go vet ./... && go test ./...

build:
	go build -o bin/ ./cmd/...
