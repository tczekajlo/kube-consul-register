FROM alpine:3.5

ENV GLIBC_VERSION=2.23-r3 \
    LANG=C.UTF-8

RUN apk --update --no-cache upgrade && \
    apk add --no-cache ca-certificates curl libstdc++ && \
    for pkg in glibc-${GLIBC_VERSION} glibc-bin-${GLIBC_VERSION} glibc-i18n-${GLIBC_VERSION}; do curl -sSL https://github.com/andyshinn/alpine-pkg-glibc/releases/download/${GLIBC_VERSION}/${pkg}.apk -o /tmp/${pkg}.apk; done && \
    apk add --allow-untrusted /tmp/*.apk && \
    rm -v /tmp/*.apk && \
    ( /usr/glibc-compat/bin/localedef --force --inputfile POSIX --charmap UTF-8 C.UTF-8 || true ) && \
    update-ca-certificates && \
    apk del curl glibc-i18n && \
    rm -rf /var/cache/apk/*

COPY dist/kube-consul-register /

WORKDIR /

ENTRYPOINT ["/kube-consul-register"]
