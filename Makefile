.PHONY: build test test-integration vet lint fmt cover

build:
	go build ./...

test:
	go test ./...

test-integration:
	go test -tags integration ./outbox/... ./inbox/... ./ratelimit/...

vet:
	go vet ./...

lint:
	docker run --rm -v "$(CURDIR)":/app -w /app golangci/golangci-lint:latest-alpine golangci-lint run ./...

fmt:
	gofmt -l .

cover:
	go test ./... -cover

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
