#!/bin/bash

ROOT_DIR=$(cd $(dirname $(dirname $0)) && pwd)
DB_DIR="$ROOT_DIR/db"
BENCH_DIR="$ROOT_DIR/bench"

mysql -uisucon -pisucon -e "DROP DATABASE IF EXISTS torb; CREATE DATABASE torb;"
mysql -uisucon -pisucon torb < "$DB_DIR/schema.sql"

if [ ! -f "$BENCH_DIR/isucon8q-initial-dataset.sql.gz" ]; then
  echo "Run the following command beforehand." 1>&2
  echo "$ ( cd \"$BENCH_DIR\" && bin/gen-initial-dataset )" 1>&2
  exit 1
fi
gzip -dc "$BENCH_DIR/isucon8q-initial-dataset.sql.gz" | mysql -uisucon -pisucon torb
