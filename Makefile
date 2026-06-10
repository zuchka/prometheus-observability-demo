BINDIR ?= bin

.PHONY: build test vet clean

build:
	mkdir -p $(BINDIR)
	go build -o $(BINDIR)/demo-api ./cmd/demo-api
	go build -o $(BINDIR)/demo-load ./cmd/demo-load

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf $(BINDIR) coverage.out

