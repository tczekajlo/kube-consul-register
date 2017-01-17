FROM alpine:3.5


RUN apk --update --no-cache upgrade && \
    apk add --no-cache ca-certificates && \
    update-ca-certificates && \
    rm -rf /var/cache/apk/*

COPY dist/kube-consul-register /

WORKDIR /

ENTRYPOINT ["/kube-consul-register"]
