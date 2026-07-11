.PHONY: test lint build build-arm64 bundle

test:
	go test ./...

lint:
	go vet ./...

build:
	go build -buildvcs=false -trimpath -o bin/ufi-tools-u60pro ./cmd/ufi-tools-u60pro

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/ufi-tools-u60pro-linux-arm64 ./cmd/ufi-tools-u60pro
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/verify-release-linux-arm64 ./cmd/verify-release

bundle: build-arm64
	mkdir -p build/bundle
	cp bin/ufi-tools-u60pro-linux-arm64 build/bundle/ufi-tools-u60pro
	cp bin/verify-release-linux-arm64 build/bundle/verify-release
	cp installer/bundle/service.sh build/bundle/service.sh
	chmod 0755 build/bundle/ufi-tools-u60pro build/bundle/verify-release build/bundle/service.sh
	@test -f build/bundle/manifest.json || { echo "add a signed build/bundle/manifest.json"; exit 1; }
	@test -f build/bundle/public-key.hex || { echo "add build/bundle/public-key.hex"; exit 1; }
