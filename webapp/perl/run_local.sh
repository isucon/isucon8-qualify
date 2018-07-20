#!/bin/bash
set -ue

export DB_DATABASE=torb
export DB_HOST=localhost
export DB_PORT=3306
export DB_USER=isucon
export DB_PASS=isucon

exec plackup -R lib -R app.psgi -R views -p 8080 app.psgi
