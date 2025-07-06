BINARY?=catmit
BIN_DIR?=bin
GOBIN?=$(shell go env GOPATH)/bin

.PHONY: build test lint e2e release clean install

# build: 编译二进制到 bin 目录
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) ./

test:
	go test ./...

lint:
	golangci-lint run

e2e:
	go test ./test/e2e

release:
	goreleaser release --clean --skip-validate --skip-lint

# install: 安装二进制到 GOBIN
install: build
	@echo "Installing $(BINARY) to $(GOBIN)/$(BINARY)"
	@mkdir -p $(GOBIN)
	@cp $(BIN_DIR)/$(BINARY) $(GOBIN)/$(BINARY)
	@chmod +x $(GOBIN)/$(BINARY)
	@echo "Installation complete. $(BINARY) is now available at $(GOBIN)/$(BINARY)"

# clean: 删除 bin 目录
clean:
	rm -rf $(BIN_DIR) 