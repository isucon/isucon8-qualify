package main

import (
	"bench"
	"flag"
)

var dataPath = flag.String("data", "./data", "path to data directory")

func main() {
	bench.DataPath = *dataPath
	bench.PrepareDataSet()
	bench.GenerateInitialDataSetSQL("../db/isucon8q-initial-dataset.sql.gz")
}
