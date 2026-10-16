.PHONY: fmt fmt/check lint lint/workflows test ci

# CI and the pre-push hook both run lint/workflows; bump versions here only.
# actionlint is not a go.mod tool: its go.yaml.in/yaml/v4 pre-release
# conflicts with golangci-lint's, and go run builds outside the module graph.
ACTIONLINT_VERSION := v1.7.12
ZIZMOR_VERSION := 1.30.1

fmt:
	gofmt -s -w -e .
	go vet ./...

fmt/check:
	@test -z "$$(gofmt -s -l .)"

lint:
	go tool golangci-lint run

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
