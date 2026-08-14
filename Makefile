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
