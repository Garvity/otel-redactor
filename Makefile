GO ?= go
VERSION ?= dev
IMAGE ?= otel-redactor:local

.PHONY: help build fmt fmt-check vet test check validate demo bench fuzz vuln container container-check
help:
	@echo 'build check demo bench fuzz vuln container container-check'

build:
	$(GO) build -trimpath -ldflags '-s -w -X main.version=$(VERSION)' -o bin/otel-redactor ./cmd/otel-redactor
	$(GO) build -trimpath -o bin/redactor-demo ./cmd/redactor-demo

fmt:
	gofmt -w cmd internal processor

fmt-check:
	@test -z "$$(gofmt -l cmd internal processor)" || (echo 'Run make fmt'; exit 1)

vet:
	$(GO) vet ./...

test:
	$(GO) test -race -count=1 -coverprofile=coverage.out ./...

validate: build
	./bin/otel-redactor validate --config examples/collector.yaml
	OTEL_REDACTOR_BACKEND=https://backend.example.test:4318 ./bin/otel-redactor validate --config examples/production.yaml
	./bin/otel-redactor validate --config examples/container.yaml

check: fmt-check vet test validate

demo:
	$(GO) run ./cmd/redactor-demo

bench:
	$(GO) test ./processor/otelredactorprocessor -run '^$$' -bench BenchmarkTraceRedaction -benchmem -count=3

fuzz:
	$(GO) test ./processor/otelredactorprocessor -run '^$$' -fuzz FuzzRecursiveRedaction -fuzztime=10s -parallel=2

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

container:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

container-check: container
	docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges $(IMAGE) validate --config /etc/otel-redactor/config.yaml
