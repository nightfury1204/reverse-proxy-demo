# stage 1: build
FROM golang:1.10-alpine AS builder
LABEL maintainer="nightfury1204"

# Add source code
RUN mkdir -p /go/src/github.com/nightfury1204/reverse-proxy-demo
ADD . /go/src/github.com/nightfury1204/reverse-proxy-demo

# Build binary
RUN cd /go/src/github.com/nightfury1204/reverse-proxy-demo && \
    GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/reverse-proxy-demo

# stage 2: lightweight "release"
FROM alpine:latest
LABEL maintainer="nightfury1204"

COPY --from=builder /go/bin/reverse-proxy-demo /bin/

ENTRYPOINT [ "/bin/reverse-proxy-demo" ]
