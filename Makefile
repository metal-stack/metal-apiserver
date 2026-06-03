SHA := $(shell git rev-parse --short=8 HEAD)
GITVERSION := $(shell git describe --long --all)
BUILDDATE := $(shell date -Iseconds)
VERSION := $(or ${VERSION},$(shell git describe --tags --exact-match 2> /dev/null || git symbolic-ref -q --short HEAD || git rev-parse --short HEAD))

CGO_ENABLED := 1
LINKMODE := -extldflags '-static -s -w'

MINI_LAB_KUBECONFIG := $(shell pwd)/../mini-lab/.kubeconfig

ifeq ($(CI),true)
  GO_TEST_ARGS=-p 1 -count=1
else
  GO_TEST_ARGS=
endif

all: test server

.PHONY: server
server:
	go build -tags netgo,osusergo,urfave_cli_no_docs \
		 -ldflags "$(LINKMODE) -X 'github.com/metal-stack/v.Version=$(VERSION)' \
								   -X 'github.com/metal-stack/v.Revision=$(GITVERSION)' \
								   -X 'github.com/metal-stack/v.GitSHA1=$(SHA)' \
								   -X 'github.com/metal-stack/v.BuildDate=$(BUILDDATE)'" \
	   -o bin/server github.com/metal-stack/metal-apiserver/cmd/server
	strip bin/server

.PHONY: test
test:
	go test ./... -race -coverpkg=./... -coverprofile=coverage.out -covermode=atomic $(GO_TEST_ARGS) -timeout=300s && go tool cover -func=coverage.out

.PHONY: bench
bench:
	CGO_ENABLED=1 go test -bench=. -run=^$$ ./... -benchmem -timeout 20m

.PHONY: mocks
mocks:
	rm -rf pkg/db/generic/mocks
	rm -rf pkg/db/repository/mocks

	# docker run --rm \
	# 	--user $$(id -u):$$(id -g) \
	# 	-w /work \
	# 	-v $(PWD):/work \
	# 	vektra/mockery:latest --keeptree --inpackage --dir pkg/db/repository --output pkg/test/mocks --all --log-level debug

	# mockery --keeptree --inpackage --dir pkg/db/generic --output pkg/db/generic/mocks --all --log-level debug
	# mockery --keeptree --inpackage --dir pkg/db/repository --output pkg/db/repository/mocks --all --log-level debug

.PHONY: golint
golint:
	golangci-lint run -p bugs -p unused -D protogetter

.PHONY: mini-lab-push
mini-lab-push: server
	docker build -f Dockerfile -t metalstack/metal-apiserver:pr .
	kind --name metal-control-plane load docker-image metalstack/metal-apiserver:pr
	kubectl --kubeconfig=$(MINI_LAB_KUBECONFIG) patch deployments.apps -n metal-control-plane metal-apiserver --patch='{"spec":{"template":{"spec":{"containers":[{"name": "apiserver","imagePullPolicy":"IfNotPresent","image":"metalstack/metal-apiserver:pr"}]}}}}'
	kubectl --kubeconfig=$(MINI_LAB_KUBECONFIG) delete pod -n metal-control-plane -l app=metal-apiserver
