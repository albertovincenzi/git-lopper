PREFIX ?= /usr/local
BATS ?= npx --yes bats@1.13.0
BIN := bin/git-lopper

.PHONY: build test lint install uninstall demo

build: ## build bin/git-lopper
	go build -o $(BIN) ./cmd/git-lopper

test: build ## unit tests with the race detector, then end-to-end tests on throwaway repositories
	go test -race ./...
	$(BATS) test

lint: ## go vet, gofmt and ShellCheck for the test scripts
	go vet ./...
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	shellcheck test/bin/gh
	shellcheck -s bash test/*.bash test/*.bats

install: build ## install git-lopper into $(PREFIX)/bin
	install -d "$(DESTDIR)$(PREFIX)/bin"
	install -m 0755 $(BIN) "$(DESTDIR)$(PREFIX)/bin/git-lopper"

uninstall:
	rm -f "$(DESTDIR)$(PREFIX)/bin/git-lopper"

demo: build ## open the app on a throwaway set of repositories
	@dir=$$(mktemp -d) && bash -c 'source test/fixture.bash && make_fixture "$$1"' _ "$$dir/code" && \
	FAKE_GH_DIR="$$dir/code/.prs" PATH="$$PWD/test/bin:$$PATH" LOPPER_CACHE_DIR="$$dir/cache" \
	LOPPER_BLOOM_DB=/nonexistent $(BIN) -C "$$dir/code"
