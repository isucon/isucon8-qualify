all: build

deps:
	go get -u github.com/constabulary/gb/...
	gb vendor restore

.PHONY: build
build:
	GOPATH=`pwd`:`pwd`/vendor go install ./src/cmd/...

.PHONY: race
race:
	GOPATH=`pwd`:`pwd`/vendor go install -race ./src/cmd/...
