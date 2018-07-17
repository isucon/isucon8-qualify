package bench

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	DataPath = "./data"
	DataSet  BenchDataSet
)

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < len(r)/2; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func prepareUserDataSet() {
	log.Println("datapath", DataPath)
	file, err := os.Open(filepath.Join(DataPath, "user.tsv"))
	must(err)
	defer file.Close()

	s := bufio.NewScanner(file)
	for i := 0; s.Scan(); i++ {
		line := strings.Split(s.Text(), "\t")
		nickname := line[0]
		addr := line[1]
		loginName := strings.Split(addr, "@")[0]

		user := &AppUser{
			LoginName: loginName,
			Password:  loginName + reverse(loginName),
			Nickname:  nickname,
		}

		if i < 1000 {
			DataSet.Users = append(DataSet.Users, user)
		} else {
			DataSet.NewUsers = append(DataSet.NewUsers, user)
		}
	}
}

func PrepareDataSet() {
	prepareUserDataSet()
}

func fbadf(w io.Writer, f string, params ...interface{}) {
	for i, param := range params {
		switch v := param.(type) {
		case []byte:
			params[i] = fmt.Sprintf("_binary x'%s'", hex.EncodeToString(v))
		case *time.Time:
			params[i] = strconv.Quote(v.Format("2006-01-02 15:04:05"))
		case time.Time:
			params[i] = strconv.Quote(v.Format("2006-01-02 15:04:05"))
		default:
			params[i] = strconv.Quote(fmt.Sprint(v))
		}
	}
	fmt.Fprintf(w, f, params...)
}

func GenerateInitialDataSetSQL(outputPath string) {
	outFile, err := os.Create(outputPath)
	must(err)
	defer outFile.Close()

	w := gzip.NewWriter(outFile)
	defer w.Close()

	fbadf(w, "SET NAMES utf8mb4;")
	fbadf(w, "BEGIN;")

	// user
	for i, user := range DataSet.Users {
		passDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(user.Password)))
		must(err)
		fbadf(w, "INSERT INTO users (id, nickname, login_name, pass_hash) VALUES (%s, %s, %s, %s);",
			i+1, user.Nickname, user.LoginName, passDigest)
	}

	fbadf(w, "COMMIT;")
}
