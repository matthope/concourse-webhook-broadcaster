PKG:=github.com/matthope/concourse-webhook-broadcaster
IMAGE?=sapcc/concourse-webhook-broadcaster
VERSION?=$(shell git describe --tags)
build:
	go build -v -ldflags="-X go.szostok.io/version.version=$(VERSION)" -o bin/webhook-broadcaster $(PKG)
	#go build -v -o bin/webhook-broadcaster $(PKG)

docker:
	go test -v
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/linux/webhook-broadcaster $(PKG)
	docker build -t $(IMAGE):$(VERSION) .

push:
	docker push $(IMAGE):$(VERSION)
