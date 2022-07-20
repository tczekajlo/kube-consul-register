FROM golang:1.16-alpine as builder

WORKDIR /build
RUN apk add make bash git
COPY . ./
RUN make

FROM alpine
RUN apk --update --no-cache upgrade && \
    apk add --no-cache ca-certificates && \
    update-ca-certificates && \
    rm -rf /var/cache/apk/*

COPY --from=builder /build/dist/kube-consul-register /
WORKDIR /
ENTRYPOINT ["/kube-consul-register"]
