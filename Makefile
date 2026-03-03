.PHONY: build test test-go test-worker lint deploy clean

build:
	go build -o randomchat ./cmd/randomchat

test: test-go test-worker

test-go:
	go test ./...

test-worker:
	npx vitest run

lint:
	go vet ./...

deploy:
	npx wrangler deploy

clean:
	rm -f randomchat
