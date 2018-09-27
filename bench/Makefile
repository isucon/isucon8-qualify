all: build

deps:
	go get -u github.com/constabulary/gb/...
	gb vendor restore

.PHONY: build
build:
	GOPATH=`pwd`:`pwd`/vendor go install ./src/cmd/...

.PHONY: build-linux
build-linux:
	GOPATH=`pwd`:`pwd`/vendor GOOS=linux GOARCH=amd64 go install ./src/cmd/...
	mv bin/linux_amd64/bench bin.Linux.x86_64/bench

.PHONY: race
race:
	GOPATH=`pwd`:`pwd`/vendor go install -race ./src/cmd/...

clean:
	rm -f isucon8q-initial-dataset.sql.gz
	go clean -r -n -x -cache -testcache
