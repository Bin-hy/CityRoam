.PHONY: help test lint build fmt migrate seed eval dev compose-config

help:
	@printf "test            run Go tests\n"
	@printf "lint            run go vet\n"
	@printf "build           build all Go packages\n"
	@printf "fmt             format Go files\n"
	@printf "migrate         run goose migrations\n"
	@printf "seed            import Beijing POI snapshot\n"
	@printf "eval            run evaluation suite\n"
	@printf "dev             start local compose stack\n"
	@printf "compose-config  validate compose file\n"

test:
	go test -race ./...

lint:
	go vet ./...

build:
	go build ./...

fmt:
	gofmt -w .

migrate:
	goose -dir migrations postgres "$(DATABASE_URL)" up

seed:
	go run ./cmd/seed

eval:
	go run ./cmd/eval

dev:
	docker compose -f deploy/docker-compose.yml up --build

compose-config:
	docker compose -f deploy/docker-compose.yml config
