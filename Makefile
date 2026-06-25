.PHONY: run build test tidy clean up down

run: up

up:
	docker compose up -d --build

down:
	docker compose down

build:
	go build -o bt ./cmd/api

test:
	go test ./...

tidy:
	go mod tidy

clean:
	rm -f bt
