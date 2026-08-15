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
