#!/bin/bash
set -ue

export DB_DATABASE=torb
export DB_HOST=localhost
export DB_PORT=3306
export DB_USER=root
export DB_PASS=

exec plackup -R lib -R app.psgi -R views app.psgi
