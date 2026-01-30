.PHONY: build test lint fmt docker-build docker-up docker-down

build:
	go build -o bin/server ./cmd/server
	go build -o bin/gpucli ./cmd/gpucli

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down
