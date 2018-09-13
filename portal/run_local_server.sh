#!/bin/bash

export DBIX_QUERYLOG_COLOR=green
export DBIX_QUERYLOG_EXPLAIN=1
plackup -s Gazelle -p 8888 -E production -MDBIx::QueryLog -a script/isucon8-portal-server -R lib
