.PHONY: help build build-linux aar run install uninstall where fmt vet test tidy clean

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

# ---- Android AAR（gomobile bind）----
# 环境变量未设置时自动探测本机默认位置；可用 `make aar ANDROID_HOME=/xxx` 覆盖。
# 注意：这些变量行不能写行尾注释——make 会把 # 前的空格也吞进值里。
ANDROID_HOME     ?= $(HOME)/Library/Android/sdk
ANDROID_NDK_HOME ?= $(shell ls -d $(ANDROID_HOME)/ndk/* 2>/dev/null | tail -1)
# gomobile bind 用 javac -source/-target 1.8，JDK≥22 已移除，必须用 ≤21 的 JDK。
JAVA_HOME        ?= $(shell /usr/libexec/java_home -v '17' 2>/dev/null || /usr/libexec/java_home -v '21' 2>/dev/null || /usr/libexec/java_home 2>/dev/null)
# NDK 27 最低支持 21（gomobile 默认 androidapi=16 会报不匹配）。
ANDROID_API      ?= 21
# 有 tag 用 tag，无 tag 用提交号，dirty 时附加 -dirty。
AAR_VERSION      ?= $(VERSION)

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

aar: ## 用 gomobile 生成 Android AAR 到 build/（自动建目录，文件名含 git 版本）
	@mkdir -p build
	@test -n "$(ANDROID_NDK_HOME)" || { echo "✗ 未找到 NDK，请设置 ANDROID_NDK_HOME"; exit 1; }
	@test -n "$(JAVA_HOME)" || { echo "✗ 未找到 JDK，请设置 JAVA_HOME"; exit 1; }
	@echo "→ ANDROID_HOME=$(ANDROID_HOME)"
	@echo "→ ANDROID_NDK_HOME=$(ANDROID_NDK_HOME)"
	@echo "→ JAVA_HOME=$(JAVA_HOME)"
	ANDROID_HOME="$(ANDROID_HOME)" \
	ANDROID_NDK_HOME="$(ANDROID_NDK_HOME)" \
	JAVA_HOME="$(JAVA_HOME)" \
	$(GO) tool gomobile bind -androidapi=$(ANDROID_API) -target=android \
		-javapkg com.sorinyang.mideable \
		-o build/midea-ble-$(AAR_VERSION).aar ./mobile
	@echo "✓ 生成 AAR: build/midea-ble-$(AAR_VERSION).aar"

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
