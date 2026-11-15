HOSTNAME=registry.terraform.io
NAMESPACE=johnciavarella
NAME=komodo
BINARY=terraform-provider-${NAME}
VERSION=3.0.0
OS_ARCH=$(shell go env GOOS)_$(shell go env GOARCH)

default: build

build:
	go build -o ${BINARY}

install: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}
	mv ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}

test:
	go test ./... -v

testacc:
	TF_ACC=1 go test ./... -v -timeout 120m

generate:
	go generate ./...
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name komodo

.PHONY: build install test testacc generate
