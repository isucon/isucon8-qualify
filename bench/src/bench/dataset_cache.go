package bench

import (
	"encoding/gob"
	"io"
	"os"
	"path/filepath"
)

const (
	DataSetCachePath = "./cache"
)

func writeDatasetCache(name string, object interface{}) error {
	filePath := filepath.Join(DataSetCachePath, name+".gob")

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}

	err = gob.NewEncoder(file).Encode(object)
	if err != nil {
		file.Close()
		return err
	}

	return file.Close()
}

func readDatasetCache(name string, v interface{}) error {
	filePath := filepath.Join(DataSetCachePath, name+".gob")

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	err = gob.NewDecoder(file).Decode(v)
	if err != nil && err != io.EOF {
		return err
	}

	return nil
}
