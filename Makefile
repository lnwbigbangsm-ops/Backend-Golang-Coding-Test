.PHONY: run build test cover proto up down

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

test:
	go test -race -cover ./...

cover:
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

# Requires protoc, protoc-gen-go and protoc-gen-go-grpc on PATH.
proto:
	protoc -I api/proto \
		--go_out=. --go_opt=module=github.com/lnwbigbangsm-ops/backend-golang-coding-test \
		--go-grpc_out=. --go-grpc_opt=module=github.com/lnwbigbangsm-ops/backend-golang-coding-test \
		api/proto/user/v1/user.proto

up:
	docker compose up --build -d

down:
	docker compose down
