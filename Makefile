VERSION ?= $(shell git describe --tags --always --dirty)

build:
	go build -ldflags="-X github.com/dukerupert/arnor/cmd.Version=$(VERSION)" -o arnor .
