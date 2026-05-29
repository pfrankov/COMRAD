GO_FOUND := $(shell command -v go 2>/dev/null)
GO ?= $(if $(GO_FOUND),$(GO_FOUND),$(CURDIR)/.tools/go/bin/go)
DIST ?= dist
VERSION ?= dev
LDFLAGS := -s -w -X comrad/internal/comrad.Version=$(VERSION)

.PHONY: validate test go-test-packages build clean bootstrap-go dashboard-deps dashboard-build smoke check-network deploy-local deploy-production-manager rollback-local e2e-real

bootstrap-go:
	@scripts/ensure-go.sh

dashboard-deps:
	cd web/dashboard && npm ci

dashboard-build: dashboard-deps
	cd web/dashboard && npm run build

validate: dashboard-build bootstrap-go
	scripts/check-guardrails.sh
	$(MAKE) go-test-packages

test: dashboard-build bootstrap-go
	$(MAKE) go-test-packages

go-test-packages: bootstrap-go
	@set -eu; \
	packages="$$( $(GO) list ./... )"; \
	packages="$$(printf '%s\n' "$$packages" | grep -v '/web/dashboard/node_modules/' || true)"; \
	if [ -z "$$packages" ]; then \
		echo "no Go packages found" >&2; \
		exit 1; \
	fi; \
	$(GO) test $$packages

build: dashboard-build bootstrap-go clean
	mkdir -p $(DIST)/bundle-darwin-arm64/bin $(DIST)/bundle-darwin-arm64/scripts $(DIST)/bundle-darwin-arm64/manifests
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/comrad-manager-darwin-arm64 ./cmd/manager
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/comrad-worker-darwin-arm64 ./cmd/worker
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/comrad-manager-linux-amd64 ./cmd/manager
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/comrad-manifest ./cmd/manifest
	cp $(DIST)/comrad-manager-darwin-arm64 $(DIST)/bundle-darwin-arm64/bin/comrad-manager
	cp $(DIST)/comrad-worker-darwin-arm64 $(DIST)/bundle-darwin-arm64/bin/comrad-worker
	cp scripts/run-local-manager.sh scripts/run-local-worker.sh scripts/install-worker-macos.sh scripts/smoke-local.sh scripts/check-network.sh scripts/rollback-local.sh scripts/llama-runtime.env $(DIST)/bundle-darwin-arm64/scripts/
	chmod +x $(DIST)/bundle-darwin-arm64/scripts/*.sh
	scripts/bundle-llama-macos.sh $(DIST)/bundle-darwin-arm64/bin
	scripts/sign-macos-bundle.sh $(DIST)/bundle-darwin-arm64/bin
	$(DIST)/comrad-manifest -root $(DIST) -out $(DIST)/manifests.json $(DIST)/comrad-manager-darwin-arm64 $(DIST)/comrad-worker-darwin-arm64 $(DIST)/comrad-manager-linux-amd64
	$(DIST)/comrad-manifest -root $(DIST)/bundle-darwin-arm64 -out $(DIST)/bundle-darwin-arm64/manifests/artifacts.json $(DIST)/bundle-darwin-arm64/bin/comrad-manager $(DIST)/bundle-darwin-arm64/bin/comrad-worker $(DIST)/bundle-darwin-arm64/bin/llama-server
	tar -C $(DIST) -czf $(DIST)/comrad-local-darwin-arm64.tar.gz bundle-darwin-arm64
	$(DIST)/comrad-manifest -root $(DIST) -out $(DIST)/release-manifest.json $(DIST)/comrad-manager-darwin-arm64 $(DIST)/comrad-worker-darwin-arm64 $(DIST)/comrad-manager-linux-amd64 $(DIST)/comrad-local-darwin-arm64.tar.gz $(DIST)/manifests.json

clean:
	rm -rf $(DIST)

smoke:
	scripts/smoke-local.sh

check-network:
	scripts/check-network.sh

deploy-local: build
	@echo "local bundle ready: $(DIST)/comrad-local-darwin-arm64.tar.gz"

deploy-production-manager:
	scripts/deploy-manager-debian.sh

rollback-local:
	scripts/rollback-local.sh

e2e-real: build
	scripts/e2e-real-llama.sh
