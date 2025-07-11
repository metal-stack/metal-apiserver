SHA := $(shell git rev-parse --short=8 HEAD)
GITVERSION := $(shell git describe --long --all)
BUILDDATE := $(shell date -Iseconds)
VERSION := $(or ${VERSION},$(shell git describe --tags --exact-match 2> /dev/null || git symbolic-ref -q --short HEAD || git rev-parse --short HEAD))

CGO_ENABLED := 1
LINKMODE := -extldflags '-static -s -w'

MINI_LAB_KUBECONFIG := $(shell pwd)/../mini-lab/.kubeconfig

all: test-opa lint-opa test server

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
	go test ./... -race -coverpkg=./... -coverprofile=coverage.out -covermode=atomic -p 2 -timeout=300s && go tool cover -func=coverage.out

.PHONY: test-opa
test-opa:
	@$(MAKE) -C pkg/auth test

.PHONY: lint-opa
lint-opa:
	cd pkg/auth && $(MAKE) lint

.PHONY: opa-fmt
opa-fmt:
	docker pull openpolicyagent/opa:latest-static
	docker run --rm -it --user $$(id -u):$$(id -g) -v $(PWD)/pkg/auth/authentication:/work openpolicyagent/opa:latest-static fmt --v1-compatible --rego-v1 -w /work
	docker run --rm -it --user $$(id -u):$$(id -g) -v $(PWD)/pkg/auth/authorization:/work openpolicyagent/opa:latest-static fmt --v1-compatible --rego-v1 -w /work

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

.PHONY: run
run:
	go run github.com/metal-stack/metal-apiserver/cmd/server serve \
		--log-level debug \
		--stage DEV \
		--masterdata-api-ca-path pkg/test/certs/ca.pem \
		--masterdata-api-cert-path pkg/test/certs/client.pem \
		--masterdata-api-cert-key-path pkg/test/certs/client-key.pem \
		--rethinkdb-addresses localhost:28015 \
		--rethinkdb-dbname metal \
		--rethinkdb-password rethink \
		--rethinkdb-user admin \
		--oidc-client-id w33moxuvwvbfyencb8y05 \
		--oidc-client-secret Ov2BHsxF4XyvIVDRuV8LD9dfkaDFfo8M \
		--oidc-end-session-url http://localhost:3001/oidc/session/end \
		--oidc-discovery-url http://localhost:3001/oidc/.well-known/openid-configuration \
		--session-secret geheim

.PHONY: masterdata-up
masterdata-up:
	docker pull ghcr.io/metal-stack/masterdata-api || true
	docker network create metalstack || true
	docker run -d --name masterdatadb --network metalstack -e POSTGRES_PASSWORD="password" -e POSTGRES_USER="masterdata" -e POSTGRES_DB="masterdata" postgres:17-alpine
	sleep 5
	docker run -d --name masterdata-api -p 50051:50051 -e MASTERDATA_API_DBHOST=masterdatadb --network metalstack -v $(PWD)/pkg/test/certs:/certs ghcr.io/metal-stack/masterdata-api

.PHONY: masterdata-rm
masterdata-rm:
	docker rm -f masterdatadb masterdata-api
	docker network rm metalstack

.PHONY: rethinkdb-up
rethinkdb-up:
	docker run -d --name metaldb -p 28015:28015 -p 8080:8080 -e RETHINKDB_PASSWORD=rethink  rethinkdb:2.4.4-bookworm-slim rethinkdb --bind all --directory /tmp --initial-password rethink

.PHONY: rethinkdb-rm
rethinkdb-rm:
	docker rm -f metaldb

.PHONY: auditing-up
auditing-up:
	docker run -d --name auditing -p 7700:7700 -e MEILI_MASTER_KEY=geheim getmeili/meilisearch:v1.6.2

.PHONY: auditing-rm
auditing-rm:
	docker rm -f auditing


.PHONY: mini-lab-push
mini-lab-push:
	make server
	docker build -f Dockerfile -t metalstack/metal-apiserver:latest .
	kind --name metal-control-plane load docker-image metalstack/metal-apiserver:latest
	kubectl --kubeconfig=$(MINI_LAB_KUBECONFIG) patch deployments.apps -n metal-control-plane metal-apiserver --patch='{"spec":{"template":{"spec":{"containers":[{"name": "apiserver","imagePullPolicy":"IfNotPresent","image":"metalstack/metal-apiserver:latest"}]}}}}'
	kubectl --kubeconfig=$(MINI_LAB_KUBECONFIG) delete pod -n metal-control-plane -l app=metal-apiserver
