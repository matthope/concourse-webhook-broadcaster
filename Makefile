PKG:=github.com/sapcc/webhook-broadcaster
IMAGE:=sapcc/concourse-webhook-broadcaster
VERSION:=0.8.0
build:
	go build -v -o bin/webhook-broadcaster $(PKG)

docker:
	go test -v
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/linux/webhook-broadcaster $(PKG)
	docker build -t $(IMAGE):$(VERSION) .

push:
	docker push $(IMAGE):$(VERSION)
