test:
	go test -v `go list ./... | grep -v 'vendor'`

build:
	go build -o ./bin/controller ./cmd/controller

codegen:
	./hack/update-codegen.sh
