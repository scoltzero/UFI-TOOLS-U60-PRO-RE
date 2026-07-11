.PHONY: test lint vuln build build-arm64 build-installers installer-test

test:
	go list -e ./... | rg -v '/third_party/' | xargs go test

lint:
	go list -e ./... | rg -v '/third_party/' | xargs go vet

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 $$(go list -e ./... | rg -v '/third_party/')

build:
	go build -buildvcs=false -trimpath -o bin/ufi-tools-u60pro ./cmd/ufi-tools-u60pro

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/ufi-tools-u60pro-linux-arm64 ./cmd/ufi-tools-u60pro
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/verify-release-linux-arm64 ./cmd/verify-release
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/ufi-plugin-store-linux-arm64 ./cmd/ufi-plugin-store
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/sendat-linux-arm64 ./cmd/sendat

build-installers:
	go build -buildvcs=false -trimpath -o bin/install-lan ./cmd/install-lan
	GOOS=windows GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/install-lan-windows-amd64.exe ./cmd/install-lan
	GOOS=windows GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o bin/UFI-TOOLS-U60-Pro-Installer.exe ./cmd/installer-ui

installer-test:
	./installer/test-transaction.sh
