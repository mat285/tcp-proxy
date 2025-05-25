FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /var/code

ADD go.mod go.mod
ADD main.go main.go

RUN go mod tidy

ARG TARGETOS TARGETARCH

RUN \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -installsuffix "static" \
    -o /bin/proxy \
    ./main.go

FROM alpine:3.18 as app
COPY --from=builder /bin/proxy /bin/proxy
ENTRYPOINT ["/bin/proxy"]
