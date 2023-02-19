# syntax=docker/dockerfile:1
FROM golang:1.20.1-alpine3.17 AS base-golang

FROM base-golang AS build
RUN apk add git~=2

ARG TARGETOS TARGETARCH
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download -x

COPY ./cmd/ ./cmd/
COPY ./internal/ ./internal/
COPY *.go ./

RUN go test ./...

COPY ./.git ./.git

RUN go build -ldflags="-X go.szostok.io/version.version=`git describe --tags`" -o /webhook-broadcaster


FROM alpine:3.17
LABEL vcs-ref="https://github.com/matthope/concourse-webhook-broadcaster"
LABEL maintainer="hackers@ninetech.dev"

COPY --from=build /webhook-broadcaster /

EXPOSE 8080
ENTRYPOINT [ "/webhook-broadcaster" ]
