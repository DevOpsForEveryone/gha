PREFIX ?= /usr/local
VERSION ?= $(shell git describe --tags --dirty --always | sed -e 's/^v//')
IS_SNAPSHOT = $(if $(findstring -, $(VERSION)),true,false)
MAJOR_VERSION = $(word 1, $(subst ., ,$(VERSION)))
MINOR_VERSION = $(word 2, $(subst ., ,$(VERSION)))
PATCH_VERSION = $(word 3, $(subst ., ,$(word 1,$(subst -, , $(VERSION)))))
NEW_VERSION ?= $(MAJOR_VERSION).$(MINOR_VERSION).$(shell echo $$(( $(PATCH_VERSION) + 1)) )

fix = false
ifeq (true,$(fix))
	FIX = --fix
endif

GHA ?= go run main.go

# GITHUB_TOKEN can be set as environment variable or via git credentials

.PHONY: pr
pr: tidy format-all lint test

.PHONY: setup-hooks
setup-hooks:
	./scripts/setup-hooks.sh

.PHONY: pre-commit
pre-commit:
	pre-commit run --all-files

.PHONY: build
build:
	go build -ldflags "-X main.version=$(VERSION)" -o dist/local/gha main.go

.PHONY: format
format:
	go fmt ./...

.PHONY: format-all
format-all:
	go fmt ./...
	npx prettier --write .

.PHONY: test
test:
	go test ./...
	$(GHA)

.PHONY: lint-go
lint-go:
	golangci-lint run $(FIX)

.PHONY: lint-js
lint-js:
	npx standard $(FIX)

.PHONY: lint-md
lint-md:
	npx markdownlint . $(FIX)

.PHONY: lint-rest
lint-rest:
	docker run --rm -it \
		-v $(PWD):/tmp/lint \
		-e GITHUB_STATUS_REPORTER=false \
		-e GITHUB_COMMENT_REPORTER=false \
		megalinter/megalinter-go:v5

.PHONY: lint
lint: lint-go lint-rest

.PHONY: lint-fix
lint-fix: lint-md lint-go

.PHONY: fix
fix:
	make lint-fix fix=true

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: install
install: build
	@cp dist/local/gha $(PREFIX)/bin/gha
	@chmod 755 $(PREFIX)/bin/gha
	@gha --version

.PHONY: installer
installer:
	@GO111MODULE=off go get github.com/goreleaser/godownloader
	godownloader -r DevOpsForEveryone/gha -o install.sh

.PHONY: promote
promote:
	@git fetch --tags
	@echo "VERSION:$(VERSION) IS_SNAPSHOT:$(IS_SNAPSHOT) NEW_VERSION:$(NEW_VERSION)"
ifeq (false,$(IS_SNAPSHOT))
	@echo "Unable to promote a non-snapshot"
	@exit 1
endif
ifneq ($(shell git status -s),)
	@echo "Unable to promote a dirty workspace"
	@exit 1
endif
	echo -n $(NEW_VERSION) > VERSION
	git add VERSION
	git commit -m "chore: bump VERSION to $(NEW_VERSION)"
	git tag -a -m "releasing v$(NEW_VERSION)" v$(NEW_VERSION)
	git push origin master
	git push origin v$(NEW_VERSION)

.PHONY: snapshot
snapshot:
	goreleaser build \
		--clean \
		--single-target \
		--snapshot

.PHONY: clean all

.PHONY: upgrade
upgrade:
	go get -u
	go mod tidy
