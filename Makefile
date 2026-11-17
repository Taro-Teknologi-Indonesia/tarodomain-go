# Run inside the dev container: ./dev make check
# Every directory that has a go.mod is a module; nested modules added later are
# picked up without editing this file.
MODULES := $(patsubst %/go.mod,%,$(shell find . -name go.mod -not -path './spec/*' -not -path './.git/*' | sort))

.PHONY: check test fmt

check:
	@set -e; for m in $(MODULES); do \
		echo "== $$m"; \
		( cd $$m; \
		  out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi; \
		  go vet ./...; \
		  staticcheck ./...; \
		  go test -race ./...; \
		  GOOS=js GOARCH=wasm go build ./... ); \
	done

test:
	@set -e; for m in $(MODULES); do ( cd $$m; go test -race ./... ); done

fmt:
	@set -e; for m in $(MODULES); do ( cd $$m; gofmt -w . ); done
