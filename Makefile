.PHONY: test coverage build run docker-up

test:
	go test ./...

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

build:
	mkdir -p dist
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/forumdesk ./cmd/forumdesk

run:
	go run ./cmd/forumdesk

docker-up:
	docker compose up -d --build
