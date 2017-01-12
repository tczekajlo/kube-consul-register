SHELL := /bin/bash

PREFIX = kube-consul-register

TESTARGS ?= -race

CURRENTDIR = $(shell pwd)
SOURCEDIR = $(CURRENTDIR)
APP_SOURCES := $(shell find $(SOURCEDIR) -name '*.go' -not -path '$(SOURCEDIR)/vendor/*')

PATH := $(CURRENTDIR)/bin:$(PATH)

VERSION?=$(shell git describe --tags)

LD_FLAGS = -ldflags "-X main.VERSION=$(VERSION) -s -w"

all: build

.PHONY: clean build docker check
default: build
build: dist/kube-consul-controller

clean:
	rm -rf dist vendor

dist/kube-consul-controller: check
	mkdir -p $(@D)
	CGO_ENABLED=0 GOOS=linux go build $(LD_FLAGS) -v -o dist/kube-consul-register

docker:
	docker build -t $(PREFIX):$(VERSION) .

test:
	go test $(TESTARGS) ./...

check-deps:
	@which gometalinter > /dev/null || \
	(go get github.com/alecthomas/gometalinter && gometalinter --install)

check: check-deps test format
	gometalinter --deadline  720s --vendor -D gotype -D dupl -D gocyclo
	cd $(SOURCEDIR)/config; gometalinter --deadline  720s --vendor -D gotype -D dupl -D gocyclo
	cd $(SOURCEDIR)/controller; gometalinter --deadline  720s --vendor -D gotype -D dupl -D gocyclo
	cd $(SOURCEDIR)/consul; gometalinter --deadline  720s --vendor -D gotype -D dupl -D gocyclo
	cd $(SOURCEDIR)/utils; gometalinter --deadline  720s --vendor -D gotype -D dupl -D gocyclo

vendor:
	glide install --strip-vendor

format:
	goimports -w -l $(APP_SOURCES)
