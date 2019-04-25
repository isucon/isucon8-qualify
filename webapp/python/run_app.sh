#!/usr/bin/env bash
ROOT=/root/isucon8-qualify
APP=${ROOT}/webapp/python

/etc/init.d/mysql start
mysql -uroot -e"CREATE USER isucon@'%' IDENTIFIED BY 'isucon';"
mysql -uroot -e"GRANT ALL on torb.* TO isucon@'%';"
mysql -uroot -e"CREATE USER isucon@'localhost' IDENTIFIED BY 'isucon';"
mysql -uroot -e"GRANT ALL on torb.* TO isucon@'localhost';"
${ROOT}/db/init.sh
DB_DATABASE=torb DB_HOST=127.0.0.1 DB_PORT=3306 DB_USER=isucon DB_PASS=isucon python ${APP}/app.py
