FROM alpine:3.5

RUN apk --update upgrade && \
    apk add --no-cache ca-certificates && \
    update-ca-certificates && \
    rm -rf /var/cache/apk/*

COPY dist/kube-consul-register /

WORKDIR /

CMD ["/kube-consul-register"]
