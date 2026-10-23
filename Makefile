.PHONY: build test race vet lint run ci e2e studio build-all docker deploy-lint verify verify-all ledger-check

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

# internal/studio/dist is embedded into the binary, but nothing under it is
# committed: `npm run build` produces it. Fresh clones must build the
# frontend before any Go command that compiles ./... (CI builds it first).
build-all:
	@if [ ! -f internal/studio/dist/index.html ]; then \
		echo "==> studio assets missing; building frontend first"; \
		$(MAKE) studio; \
	fi
	$(MAKE) build

docker:
	docker build -t restitch:dev .

# Lints the deployment files. The placeholder values satisfy the variables
# that deploy/docker-compose.yml demands; they are never used to run.
KUBECONFORM_VERSION := v0.7.0
deploy-lint:
	POSTGRES_PASSWORD=lint RESTITCH_BOOTSTRAP_EMAIL=lint@example.com RESTITCH_BOOTSTRAP_PASSWORD=lint \
		docker compose -f deploy/docker-compose.yml config -q
	go run github.com/yannh/kubeconform/cmd/kubeconform@$(KUBECONFORM_VERSION) -strict -summary deploy/kubernetes

verify:
	scripts/verify.sh $(GATE)

verify-all:
	scripts/verify.sh all

ledger-check:
	scripts/check-ledger.sh
