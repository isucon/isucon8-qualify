all: build

.PHONY: clean
clean:
	rm -rf torb

deps:
	gb vendor restore

.PHONY: build
build:
	GOPATH=`pwd`:`pwd`/vendor go build -v torb
