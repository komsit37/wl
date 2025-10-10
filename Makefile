SHELL := /bin/bash

MODULE := github.com/komsit37/wl
BIN := wl
CMD := ./cmd/wl

.PHONY: all build run fmt vet tidy hooks help

all: build

build:
	go build -o $(BIN) $(CMD)

run:
	go run $(CMD)

fmt:
	go fmt ./...
	goimports -w -local $(MODULE) . 2>/dev/null || true

vet:
	go vet ./...

tidy:
	go mod tidy

hooks:
	git config core.hooksPath .githooks

help:
	@echo "Targets: build, run, fmt, vet, tidy, hooks"

