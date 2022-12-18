# Build the manager binary
FROM golang:1.19 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY apis/ apis/
COPY controller/ controller/
COPY apiserver/ apiserver/

# Build
RUN if [ "$(uname -m)" = "aarch64" ]; then \
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GO111MODULE=on go build -a -o /apiserver cmd/apiserver/main.go; \
    else \
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o /apiserver cmd/apiserver/main.go; \
    fi

RUN if [ "$(uname -m)" = "aarch64" ]; then \
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GO111MODULE=on go build -a -o /controller cmd/controller/main.go; \
    else \
        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o /controller cmd/controller/main.go; \
    fi

FROM gcr.io/distroless/base:latest as controller
WORKDIR /
COPY --from=builder /controller /controller

ENTRYPOINT ["/controller"]

FROM gcr.io/distroless/base:latest as apiserver
WORKDIR /
COPY --from=builder /apiserver /apiserver

EXPOSE 8081

ENTRYPOINT ["/apiserver"]
