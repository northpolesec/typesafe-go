.PHONY: fmt fmt/check lint test ci

fmt:
	gofmt -s -w -e .
	go vet ./...

fmt/check:
	@test -z "$$(gofmt -s -l .)"

lint:
	go tool golangci-lint run

test:
	go test -shuffle=on -race -timeout=120s $(if $(COVERPROFILE),-coverprofile=$(COVERPROFILE)) ./...

ci: fmt/check lint test
