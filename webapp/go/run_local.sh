#!/bin/bash
set -ue

export DB_DATABASE=torb
export DB_HOST=localhost
export DB_PORT=3306
export DB_USER=isucon
export DB_PASS=isucon

 lsof -i:8080 | grep torb | grep LISTEN | awk '{print $2}' | xargs -IPID kill -TERM PID
./torb
