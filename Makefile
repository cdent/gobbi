
.PHONY: test lint

test:
	go test -v ./...

lint:
	golangci-lint run
