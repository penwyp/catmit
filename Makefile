BINARY?=catmit
BIN_DIR?=bin
GOBIN?=$(shell go env GOPATH)/bin

.PHONY: build test lint e2e release clean install

# build: Build binary to bin directory
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) ./

# test: Run all unit tests
test:
	go test ./...

# lint: Run golangci-lint
lint:
	golangci-lint run

# e2e: Run end-to-end tests
e2e:
	go test ./test/e2e

# release: Release a specific version (usage: make release v0.0.1)
release:
	@if [ -z "$(filter-out $@,$(MAKECMDGOALS))" ]; then \
		echo "Usage: make release v0.0.1"; \
		exit 1; \
	fi
	@VERSION=$(filter-out $@,$(MAKECMDGOALS)); \
	echo "Releasing version $$VERSION"; \
	if ! echo "$$VERSION" | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' > /dev/null; then \
		echo "Error: Version must be in format v0.0.1"; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: Working directory is not clean. Please commit or stash changes."; \
		exit 1; \
	fi; \
	echo "Updating version to $$VERSION"; \
	git add -A; \
	git commit -m "chore: bump version to $$VERSION" || true; \
	git tag -a "$$VERSION" -m "Release $$VERSION"; \
	git push origin main; \
	git push origin "$$VERSION"; \
	echo "Version $$VERSION has been tagged and pushed successfully"

# goreleaser-release: Internal goreleaser release command
goreleaser-release:
	goreleaser release --clean --skip-validate --skip-lint

# install: Install binary to GOBIN
install: build
	@echo "Installing $(BINARY) to $(GOBIN)/$(BINARY)"
	@mkdir -p $(GOBIN)
	@cp $(BIN_DIR)/$(BINARY) $(GOBIN)/$(BINARY)
	@chmod +x $(GOBIN)/$(BINARY)
	@echo "Installation complete. $(BINARY) is now available at $(GOBIN)/$(BINARY)"

# clean: Remove bin directory
clean:
	rm -rf $(BIN_DIR)

# Allow arguments to be passed to make release
%:
	@: 