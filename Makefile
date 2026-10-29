.PHONY: fmt fmt/check lint lint/workflows test ci

# CI and the pre-push hook run these; bump versions here only. The linters are
# not go.mod tools: a tool's requirements become every importer's, which would
# pull golangci-lint's whole dependency graph into code that uses this SDK.
# go run builds them outside the module graph.
GOLANGCI_LINT_VERSION := v2.15.0
ACTIONLINT_VERSION := v1.7.12
ZIZMOR_VERSION := 1.30.1

fmt:
	gofmt -s -w -e .
	go vet ./...

fmt/check:
	@test -z "$$(gofmt -s -l .)"

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

# zizmor's online audits (such as a pin whose tag has moved) need a GitHub
# token, so this fails rather than run offline and miss them. Under GitHub
# Actions it emits annotations instead of plain output.
lint/workflows:
	go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)
	@token="$${GH_TOKEN:-$$(gh auth token 2>/dev/null)}"; \
	test -n "$$token" || { echo "lint/workflows: set GH_TOKEN or run gh auth login" >&2; exit 1; }; \
	docker run --rm -v "$$PWD:/src" -w /src -e GH_TOKEN="$$token" \
		ghcr.io/zizmorcore/zizmor:$(ZIZMOR_VERSION) --persona pedantic $(if $(GITHUB_ACTIONS),--format=github) .

test:
	go test -shuffle=on -race -timeout=120s $(if $(COVERPROFILE),-coverprofile=$(COVERPROFILE)) ./...

ci: fmt/check lint test
