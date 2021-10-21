GOOPTS=GOARCH=amd64 CGO_ENABLED=0 GOOS=darwin

BIN=demo-controller

.PHONY: all build run test lint  gen fmt vendor

all: lint test docker push

build:
	${GOOPTS} go build -v -mod=mod -o ${BIN} cmd/main.go

install:
	kubectl apply -f manifests/crd.yaml

run: build
	./${BIN} --kubeconfig=${KUBECONFIG} \

lint:
	@golangci-lint run --color 'always'

fmt:
	@gofumpt -w -l pkg/

gen: vendor
	./hack/update-codegen.sh

vendor:
	go mod vendor

clean-cache:
	go clean -modcache

test:
	@go test -race -covermode=atomic -coverprofile coverage.out `go list ./internal/.../`
	@go tool cover -func coverage.out

tidy:
	go mod tidy
