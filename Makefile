BIN := $(HOME)/.local/bin

.PHONY: build
build:
	go build -o $(BIN)/telescreen .

.PHONY: install
install: build
	$(BIN)/telescreen install

.PHONY: test
test:
	go vet ./...
	go test ./...

.PHONY: docs
docs:
	go run . docs

.PHONY: docs-check
docs-check: docs
	git diff --exit-code docs/reference

.PHONY: e2e
e2e:
	rm -rf covdata && mkdir -p covdata
	GOCOVERDIR=$(CURDIR)/covdata go test -count=1 -tags e2e ./e2e/
	go tool covdata textfmt -i covdata -o e2e.out
	go test -coverprofile=unit.out ./...
	@echo "unit total: $$(go tool cover -func=unit.out | tail -1 | awk '{print $$3}')"
	@echo "e2e total:  $$(go tool cover -func=e2e.out | tail -1 | awk '{print $$3}')"
