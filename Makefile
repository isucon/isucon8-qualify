include .env

bench/remote:
	ssh -i ${KEY} centos@${IP} make -C /home/centos/isucon8-qualify/bench bench

deploy: build/linux push torb.go/restart

push:
	scp -i ${KEY} ./webapp/go/torb centos@${IP}:/tmp
	ssh -i ${KEY} centos@${IP} sudo mv /tmp/torb /home/isucon/torb/webapp/go/torb

build/linux:
	cd ./webapp/go && GOPATH=`pwd`:`pwd`/vendor GOARCH="amd64" GOOS="linux" go build -o torb src/torb/app.go

torb.go/restart:
	ssh -i ${KEY} centos@${IP} sudo systemctl restart torb.go
