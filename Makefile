include .env

bench/exec:
	ssh -i ${KEY} centos@${IP} make -C /home/centos/isucon8-qualify/bench bench

go/replace: go/push go/build go/restart

go/push:
	tar --exclude ./webapp/go/torb --exclude ./webapp/go/vendor/ -zcvf ./torb.tar.gz ./webapp/go/*
	scp -i ${KEY} torb.tar.gz centos@${IP}:/tmp
	ssh -i ${KEY} centos@${IP} tar -zxvf /tmp/torb.tar.gz -C /tmp
	ssh -i ${KEY} centos@${IP} rm -rf /home/centos/isucon8-qualify/webapp/go
	ssh -i ${KEY} centos@${IP} mv /tmp/webapp/go /home/centos/isucon8-qualify/webapp/

go/build:
	ssh -i ${KEY} centos@${IP} make -C /home/centos/isucon8-qualify/webapp/go build

go/restart:
	ssh -i ${KEY} centos@${IP} sudo systemctl restart torb.go
