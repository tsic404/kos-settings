# Local entry points for the kos-settings extension package. CI runs `lint`,
# `test` and `acceptance`, one step per target; nothing here pushes or publishes.
SHELL := /bin/sh
GO ?= go
JS ?= node

.PHONY: all lint fmt-check vet js-check js-test test acceptance pack clean

all: lint test

# fmt-check fails on any file gofmt would rewrite, so a stale format cannot hide
# behind a passing test run.
fmt-check:
	@unformatted=$$(gofmt -l $$(git ls-files '*.go')); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt required for:"; echo "$$unformatted"; exit 1; \
	fi

vet:
	$(GO) vet ./...

# js-check parses every browser module; a syntax error is caught here rather
# than in the panel that loads it.
js-check:
	@for file in $$(git ls-files '*.js'); do $(JS) --check "$$file" || exit 1; done

js-test:
	$(JS) --test tests/acceptance/*/shell.test.js

lint: fmt-check vet js-check

test: js-test
	$(GO) test ./...

acceptance: js-test
	$(GO) test ./tests/... -count=1 -v

pack:
	$(GO) run ./cmd/kos-settings-pack -out dist/kos-settings.tar

clean:
	$(GO) clean -testcache
	rm -rf dist
