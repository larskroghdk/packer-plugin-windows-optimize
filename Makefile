GOPATH              := $(shell go env GOPATH | tr '\\' '/')
GORELEASER_VERSION  := 2.18.2
STATICCHECK_VERSION := 2025.1.1
PACKER_SDC_VERSION  := $(shell go list -m -f '{{.Version}}' github.com/hashicorp/packer-plugin-sdk)
GORELEASER          ?= $(GOPATH)/bin/goreleaser
STATICCHECK         ?= $(GOPATH)/bin/staticcheck
PACKER_SDC          ?= $(GOPATH)/bin/packer-sdc
PWSH                ?= pwsh
PESTER_SHELL        ?= powershell.exe
PACKER              ?= packer
API_VERSION          = $(shell go run . describe 2>/dev/null | sed -n 's/.*"api_version":"\([^"]*\)".*/\1/p')

.PHONY: check lint fmt-check doccheck catalog-check pslint hclfmt typecheck test test-go test-ps test-integration fmt generate generate-check catalog init build release-snapshot release clean

check: lint typecheck test

lint: fmt-check doccheck catalog-check pslint hclfmt
	$(STATICCHECK) ./...

fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

doccheck:
	go run ./cmd/doccheck .
	@sh ./psdoccheck.sh

catalog-check:
	go run ./cmd/catalogdoc -check

pslint:
	$(PWSH) -NoProfile -Command '$$ErrorActionPreference = "Stop"; $$r = "guest/scripts", "test/pester" | ForEach-Object { Invoke-ScriptAnalyzer -Path $$_ -Recurse -Settings ./PSScriptAnalyzerSettings.psd1 }; $$r; if ($$r) { exit 1 }'

hclfmt:
	$(PACKER) fmt -check -recursive examples
	$(PACKER) fmt -check -recursive test/integration

typecheck:
	go build ./...
	go vet ./...
	go vet -tags integration ./test/integration

test: test-go test-ps

test-go:
	go test ./...

test-ps:
	$(PESTER_SHELL) -NoProfile -ExecutionPolicy Bypass -Command '$$env:PSModulePath = (Join-Path ([Environment]::GetFolderPath("MyDocuments")) "WindowsPowerShell\Modules") + ";" + [Environment]::GetEnvironmentVariable("PSModulePath", "Machine"); Import-Module Pester -MinimumVersion 5.0; $$c = New-PesterConfiguration; $$c.Run.Path = "test/pester"; $$c.Run.Exit = $$true; $$c.Output.Verbosity = "Detailed"; Invoke-Pester -Configuration $$c'

test-integration:
	go test -tags integration -v -count=1 -timeout 90m ./test/integration

fmt:
	gofmt -w .
	$(PACKER) fmt -recursive examples
	$(PACKER) fmt -recursive test/integration

generate: $(PACKER_SDC)
	cd provisioner && GOFILE=config.go GOPACKAGE=provisioner $(PACKER_SDC) mapstructure-to-hcl2 -type Config
	gofmt -w provisioner/config.hcl2spec.go

generate-check: generate
	git diff --exit-code provisioner/config.hcl2spec.go || (echo 'provisioner/config.hcl2spec.go is stale: run make generate and commit it'; exit 1)

catalog:
	go run ./cmd/catalogdoc

init: $(STATICCHECK) $(PACKER_SDC)
	go mod download

$(GORELEASER):
	GOTOOLCHAIN=auto go install github.com/goreleaser/goreleaser/v2@v$(GORELEASER_VERSION)

$(STATICCHECK):
	go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)

$(PACKER_SDC):
	go install github.com/hashicorp/packer-plugin-sdk/cmd/packer-sdc@$(PACKER_SDC_VERSION)

build: $(GORELEASER)
	API_VERSION="$(API_VERSION)" $(GORELEASER) build --skip=validate --clean --single-target

release-snapshot: $(GORELEASER)
	API_VERSION="$(API_VERSION)" $(GORELEASER) release --snapshot --skip=publish,sign --clean

release: $(GORELEASER)
	API_VERSION="$(API_VERSION)" $(GORELEASER) release --clean

clean:
	rm -rf dist reports windows-optimize-report-*.json
