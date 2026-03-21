# General
WORKDIR = $(PWD)

# Go parameters
GOCMD = go
GOTEST = $(GOCMD) test

# Git config
GIT_VERSION ?=
GIT_DIST_PATH ?= $(PWD)/.git-dist
GIT_REPOSITORY = http://github.com/git/git.git

# Coverage
COVERAGE_REPORT = coverage.out
COVERAGE_MODE = count

# renovate: datasource=github-tags depName=golangci/golangci-lint
GOLANGCI_VERSION ?= v2.7.2
TOOLS_BIN := $(shell mkdir -p build/tools && realpath build/tools)

GOLANGCI = $(TOOLS_BIN)/golangci-lint-$(GOLANGCI_VERSION)
$(GOLANGCI):
	rm -f $(TOOLS_BIN)/golangci-lint*
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_VERSION)/install.sh | sh -s -- -b $(TOOLS_BIN) $(GOLANGCI_VERSION)
	mv $(TOOLS_BIN)/golangci-lint $(TOOLS_BIN)/golangci-lint-$(GOLANGCI_VERSION)

# Defines the maximum time each fuzz target will be executed for.
FUZZ_TIME ?= 10s
FUZZ_PKGS = $(shell grep -r --include='**_test.go' --files-with-matches 'func Fuzz' . | xargs -I{} dirname {})

build-git:
	@if [ -f $(GIT_DIST_PATH)/git ]; then \
		echo "nothing to do, using cache $(GIT_DIST_PATH)"; \
	else \
		git clone $(GIT_REPOSITORY) -b $(GIT_VERSION) --depth 1 --single-branch $(GIT_DIST_PATH); \
		cd $(GIT_DIST_PATH); \
		make configure; \
		./configure; \
		make all; \
	fi

test:
	@echo "running against `git version`"; \
	$(GOTEST) -race ./...
	$(GOTEST) -v _examples/common_test.go _examples/common.go --examples

test-coverage:
	@echo "running against `git version`"; \
	echo "" > $(COVERAGE_REPORT); \
	$(GOTEST) -coverprofile=$(COVERAGE_REPORT) -coverpkg=./... -covermode=$(COVERAGE_MODE) ./...

# CLI shim
CLI_BIN = $(WORKDIR)/build/bin
GIT_CLI_VERSION ?= v2.47.0

build-cli:
	mkdir -p $(CLI_BIN)
	$(GOCMD) build -o $(CLI_BIN)/git ./cli/git-go/...
	$(CLI_BIN)/git install

# Clone and build upstream git into .git-dist/ (needed for test-lib.sh,
# test-tool helpers, etc.). Cached — only clones/builds once.
build-git-testdeps:
	@if [ -f $(GIT_DIST_PATH)/git ]; then \
		echo "using cached git build at $(GIT_DIST_PATH)"; \
	else \
		echo "Cloning git $(GIT_CLI_VERSION) into $(GIT_DIST_PATH)..."; \
		git clone $(GIT_REPOSITORY) -b $(GIT_CLI_VERSION) --depth 1 --single-branch $(GIT_DIST_PATH); \
		echo "Building git in $(GIT_DIST_PATH)..."; \
		cd $(GIT_DIST_PATH) && make -j$$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4) all; \
	fi

# Run git test file(s) against the go-git CLI.
# Usage:
#   make test-cli                                  # run all key suites
#   make test-cli T=t7004-tag.sh                   # run a specific test
#   make test-cli T="t0001-init.sh t7004-tag.sh"   # run multiple
#   make test-cli-verbose T=t0001-init.sh          # verbose single test
test-cli: build-cli build-git-testdeps
	@bash cli/git-go/run-tests.sh $(GIT_DIST_PATH) $(CLI_BIN) $(T)

test-cli-verbose: build-cli build-git-testdeps
ifndef T
	$(error T is required. Usage: make test-cli-verbose T=t0001-init.sh)
endif
	@bash cli/git-go/run-tests.sh $(GIT_DIST_PATH) $(CLI_BIN) --verbose $(T)

# Re-generate the skip list by detecting which failures hit unimplemented commands.
gen-skip-list: build-cli build-git-testdeps
	bash cli/git-go/gen-skip-list.sh $(GIT_DIST_PATH) $(CLI_BIN)

clean:
	rm -rf $(GIT_DIST_PATH) build/

fuzz:
	@for path in $(FUZZ_PKGS); do \
		go test -fuzz=Fuzz -fuzztime=$(FUZZ_TIME) $$path; \
	done

validate: validate-lint validate-dirty ## Run validation checks.

validate-lint: $(GOLANGCI)
	$(GOLANGCI) run

validate-dirty:
ifneq ($(shell git status --porcelain --untracked-files=no),)
	@echo worktree is dirty
	@git --no-pager status
	@git --no-pager diff
	@exit 1
endif
