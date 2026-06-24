.PHONY: build clean lint

build:
	go build -o runagent ./cmd/runagent

install:
	go install ./cmd/runagent

lint:
	go tool -modfile tools/go.mod golangci-lint run ./...

clean:
	rm -f runagent
