#!/bin/bash -ue

source ../env.sh

export DB_DATABASE
export DB_HOST
export DB_PORT
export DB_USER
export DB_PASS

export NODE_ENV=development
$(npm bin)/ts-node-dev index.ts
