#!/bin/bash -ue

source ../env.sh

export DB_DATABASE
export DB_HOST
export DB_PORT
export DB_USER

$(npm bin)/ts-node index.ts
