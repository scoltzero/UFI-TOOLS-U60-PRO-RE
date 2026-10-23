VERSION ?= 0.0.1
LDFLAGS_VERSION = -X main.version=$(VERSION)

.PHONY: test lint vuln build build-arm64 build-lan build-installers android-check installer-test license-test stage5-integration

test:
	go list -e ./... | rg -v '/third_party/' | xargs go test
	node --test internal/webui/request_flow_test.cjs

lint:
	go list -e ./... | rg -v '/third_party/' | xargs go vet

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 $$(go list -e ./... | rg -v '/third_party/')

build:
	go build -buildvcs=false -trimpath -ldflags "$(LDFLAGS_VERSION)" -o bin/ufi-tools-u60pro ./cmd/ufi-tools-u60pro

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w $(LDFLAGS_VERSION)" -o bin/ufi-tools-u60pro-linux-arm64 ./cmd/ufi-tools-u60pro
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/verify-release-linux-arm64 ./cmd/verify-release
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/ufi-plugin-store-linux-arm64 ./cmd/ufi-plugin-store
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/sendat-linux-arm64 ./cmd/sendat

build-lan:
	go build -buildvcs=false -trimpath -o bin/ufi-lan-installer ./cmd/lan-installer

build-installers:
	go build -buildvcs=false -trimpath -o bin/install-lan ./cmd/install-lan
	GOOS=windows GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "-s -w" -o bin/install-lan-windows-amd64.exe ./cmd/install-lan
	GOOS=windows GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o bin/UFI-TOOLS-U60-Pro-Installer.exe ./cmd/installer-ui

android-check:
	cd installer/android && ./gradlew --no-daemon :app:lintDebug :app:assembleDebug

installer-test:
	./installer/test-transaction.sh
	./installer/test-install-adb.sh
	./installer/test-lifecycle-edges.sh
	python3 installer/test-adb-transport.py

license-test:
	@tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT HUP INT TERM; ./scripts/collect-licenses.sh "$$tmp"; test -s "$$tmp/modules.tsv"

# Requires the existing companion workspace (or UFI_PLUGIN_WORKSPACE), its
# Playwright package, installed Chrome and a host C compiler. Local fixtures only.
stage5-integration:
	UFI_STAGE5_BROWSER=1 go test ./internal/httpapi -run 'TestDevUIManagerShellContracts|TestStage5BrowserIntegration' -count=1 -v
	python3 scripts/test-stage5-data.py
	node scripts/test-stage5-plugin-runtime.cjs
