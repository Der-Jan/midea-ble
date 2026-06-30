.PHONY: help build build-linux run install uninstall where fmt vet test tidy clean

GO    ?= go
APP   := midea-ble-go
PKG   := ./cmd/$(APP)
BIN   := build/$(APP)
GOBIN := $(shell $(GO) env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell $(GO) env GOPATH)/bin
endif

# 版本注入（注入 main.*）
VERSION    ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT_ID  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS    := -X 'main.Version=$(VERSION)' \
              -X 'main.Commit=$(COMMIT_ID)' \
              -X 'main.BuildTime=$(BUILD_TIME)'

.DEFAULT_GOAL := help

help: ## 显示此帮助
	@echo 'midea-ble-go — 美的/华凌空调蓝牙直连控制 CLI'
	@echo ''
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## 编译到 build/$(APP)（带版本注入）
	@mkdir -p build
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

build-linux: ## 交叉编译 Linux amd64
	@mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags "-s -w $(LDFLAGS)" \
		-o $(BIN)-linux-amd64 $(PKG)

run: build ## 编译并运行（无参进入 REPL；ARGS= 传子命令，如 make run ARGS="scan")
	./$(BIN) $(ARGS)

install: ## go install 到 $(GOBIN)，之后任意目录直接敲 $(APP)
	$(GO) install -ldflags "$(LDFLAGS)" $(PKG)
	@echo "已安装到 $(GOBIN)/$(APP)"
	@case ":$$PATH:" in *":$(GOBIN):"*) ;; \
		*) echo "提示：$(GOBIN) 不在 PATH 里，加到 shell 配置：export PATH=\"$(GOBIN):\$$PATH\"" ;; \
	esac

uninstall: ## 卸载已安装的 $(APP)
	@rm -fv $(GOBIN)/$(APP)

where: ## 打印安装路径
	@echo $(GOBIN)/$(APP)

fmt: ## go fmt
	$(GO) fmt ./...

vet: ## go vet
	$(GO) vet ./...

test: ## 跑测试（离线，含协议一致性向量）
	$(GO) test ./...

tidy: ## go mod tidy
	$(GO) mod tidy

clean: ## 清理 build/
	rm -rf build
