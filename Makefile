.PHONY: build test race vet lint run ci e2e studio build-all docker verify verify-all ledger-check

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/restitch ./cmd/restitch

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run

run:
	go run ./cmd/restitch

ci: vet lint race

e2e:
	go test -tags e2e ./tests/ -count=1 -v

studio:
	cd studio && npm ci && npm run build

build-all: build
	go build $(LDFLAGS) -o bin/restitch-studio ./cmd/restitch-studio

docker:
	docker build -t restitch:dev .

verify:
	scripts/verify.sh $(GATE)

verify-all:
	scripts/verify.sh all

ledger-check:
	scripts/check-ledger.sh
