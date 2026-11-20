.PHONY: help test lint build fmt migrate seed eval skills-eval dev compose-config skills-verify skills-lock skills-lint skills-lint-strict

help:
	@printf "test            run Go tests\n"
	@printf "lint            run go vet\n"
	@printf "build           build all Go packages\n"
	@printf "fmt             format Go files\n"
	@printf "migrate         run goose migrations\n"
	@printf "seed            import Beijing POI snapshot\n"
	@printf "eval            run evaluation suite\n"
	@printf "skills-eval     run skill trigger eval against a real model (manual)\n"
	@printf "skills-verify   verify skills against skills.lock.json\n"
	@printf "skills-lock     pin skill versions and hashes\n"
	@printf "skills-lint     check skill authoring rules (warnings only)\n"
	@printf "skills-lint-strict  fail on any skill lint warning (CI)\n"
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

skills-eval:
	go run ./cmd/skilleval

skills-verify:
	go run ./cmd/skills

skills-lock:
	go run ./cmd/skills -write

skills-lint:
	go run ./cmd/skills -lint

skills-lint-strict:
	go run ./cmd/skills -lint -strict

dev:
	docker compose -f deploy/docker-compose.yml up --build

compose-config:
	docker compose -f deploy/docker-compose.yml config
