include .env

bench/exec:
	ssh -i ${KEY} centos@${IP} make -C /home/centos/isucon8-qualify/bench bench

go/replace: go/push go/build go/restart

go/push:
	scp -i ${KEY} -r webapp/go centos@${IP}:/home/centos/isucon8-qualify/webapp/go

go/build:
	ssh -i ${KEY} centos@${IP} make -C /home/centos/isucon8-qualify/webapp/go build

go/restart:
	ssh -i ${KEY} centos@${IP} sudo systemctl restart torb.go
