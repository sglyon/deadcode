# deadcode — common dev tasks.
#
# `make` (no target) is `make help`.

.PHONY: help build install test vet fmt snapshot release clean dogfood

help:
	@echo "deadcode dev targets"
	@echo ""
	@echo "  make build      Local build for the current OS/arch (./deadcode)"
	@echo "  make install    Install to \$$GOPATH/bin via go install"
	@echo "  make test       Run all unit tests"
	@echo "  make vet        Run go vet"
	@echo "  make fmt        Run gofmt -w on the tree"
	@echo "  make dogfood    Build and scan deadcode against itself"
	@echo "  make snapshot   Build all release artifacts locally (no publish)"
	@echo "                  Output lands in dist/ — gitignored"
	@echo "  make release    Cut a real release from the current git tag"
	@echo "                  (requires GITHUB_TOKEN, runs goreleaser release)"
	@echo "  make clean      Remove ./deadcode and dist/"

build:
	go build -o deadcode .

install:
	go install .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

dogfood: build
	./deadcode scan --pretty=always .

snapshot:
	goreleaser release --snapshot --clean

release:
	goreleaser release --clean

clean:
	rm -f deadcode
	rm -rf dist/
