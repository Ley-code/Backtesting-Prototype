.PHONY: run build test tidy clean

run:
	go run ./cmd/api

build:
	go build -o bt ./cmd/api

test:
	go test ./...

tidy:
	go mod tidy

clean:
	rm -rf .cache bt
