.PHONY: test lint build build-arm64 installer-test

test:
	go test ./...

lint:
	go vet ./...

build:
	go build -buildvcs=false -trimpath -o bin/ufi-tools-u60pro ./cmd/ufi-tools-u60pro

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/ufi-tools-u60pro-linux-arm64 ./cmd/ufi-tools-u60pro
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/verify-release-linux-arm64 ./cmd/verify-release
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/ufi-plugin-store-linux-arm64 ./cmd/ufi-plugin-store
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/sendat-linux-arm64 ./cmd/sendat

installer-test:
	./installer/test-transaction.sh
