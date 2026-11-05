PREFIX ?= /usr/local
BATS ?= npx --yes bats@1.13.0

.PHONY: test lint install uninstall demo

test: ## run the test suite
	$(BATS) test

lint: ## run ShellCheck
	shellcheck -x bin/git-lopper test/bin/gh
	shellcheck -s bash test/*.bash

install: ## install git-lopper into $(PREFIX)/bin
	install -d "$(DESTDIR)$(PREFIX)/bin"
	install -m 0755 bin/git-lopper "$(DESTDIR)$(PREFIX)/bin/git-lopper"

uninstall:
	rm -f "$(DESTDIR)$(PREFIX)/bin/git-lopper"

demo: ## open the app on a throwaway set of repositories
	@dir=$$(mktemp -d) && bash -c 'source test/fixture.bash && make_fixture "$$1"' _ "$$dir/code" && \
	FAKE_GH_DIR="$$dir/code/.prs" PATH="$$PWD/test/bin:$$PATH" LOPPER_CACHE_DIR="$$dir/cache" \
	LOPPER_BLOOM_DB=/nonexistent bin/git-lopper -C "$$dir/code"
