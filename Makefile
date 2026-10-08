MODULE := github.com/AESalnikov/depscout
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)
GREMLINS := CGO_ENABLED=0 go run github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
# Single worker: parallel mutants starve go test and TIMED OUT → CI cancel.
GREMLINS_FLAGS := --workers=1 --test-cpu=1 --timeout-coefficient=10 \
	--conditionals-boundary=false --invert-negatives=false --increment-decrement=false \
	--threshold-efficacy=55 --threshold-mcover=40

.PHONY: build test race cover vet lint staticcheck vuln fuzz integration mutate fmt tidy clean run help check release verify-release

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-16s %s\n", $$1, $$2}'

build: ## Build ./depscout
	go build -ldflags "$(LDFLAGS)" -o depscout .

test: ## Run unit tests (no integration / Docker)
	go test ./...

race: ## Unit tests with -race
	go test -race ./...

cover: ## Unit tests with coverage (must be 100%)
	go test ./... -coverprofile=coverage.out -covermode=atomic
	@go tool cover -func=coverage.out | tail -1
	@go tool cover -func=coverage.out | awk '/total:/ { if ($$3 != "100.0%") { print "coverage is not 100%: " $$3; exit 1 } }'

vet: ## go vet
	go vet ./...

staticcheck: ## staticcheck ./...
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...

lint: ## golangci-lint
	CGO_ENABLED=0 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...

vuln: ## govulncheck ./...
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

fuzz: ## Short fuzz smoke
	go test ./internal/maven/ -fuzz=FuzzCompare -fuzztime=5s
	go test ./internal/maven/ -fuzz=FuzzVersionsFromHTMLListing -fuzztime=5s
	go test ./internal/maven/ -fuzz=FuzzParseMetadata -fuzztime=5s
	go test ./internal/maven/ -fuzz=FuzzImplementationGAVsFromMarkerPOM -fuzztime=5s
	go test ./internal/gradle/ -fuzz=FuzzParseBuildScriptSnippets -fuzztime=5s
	go test ./internal/gradle/ -fuzz=FuzzParseCatalogTOML -fuzztime=5s
	go test ./internal/gradle/ -fuzz=FuzzParseWrapperProperties -fuzztime=5s

integration: ## Testcontainers Artifactory fixture (needs Docker)
	go test -tags=integration -count=1 -timeout=5m -run TestIntegration ./internal/scout/

mutate: ## Mutation testing (gremlins on internal/maven)
	$(GREMLINS) unleash $(GREMLINS_FLAGS) ./internal/maven

fmt: ## Format sources
	gofmt -w .

tidy: ## Sync go.mod / go.sum
	go mod tidy

clean: ## Remove built binary / coverage / dist
	rm -f depscout depscout-* coverage.out coverage.html
	rm -rf dist

release: ## Local snapshot (без cosign/SBOM — полный релиз на tag через CI)
	go run github.com/goreleaser/goreleaser/v2@v2.12.0 release --snapshot --clean --skip=sign,sbom

# Пример: make verify-release TAG=v0.1.0
verify-release: ## Проверить GitHub Release (cosign + checksums + slsa-verifier)
	@test -n "$(TAG)" || (echo "usage: make verify-release TAG=vX.Y.Z" >&2; exit 2)
	./scripts/verify-release.sh "$(TAG)"

check: fmt vet lint vuln race cover ## fmt + vet + lint + vuln + race + 100% coverage
