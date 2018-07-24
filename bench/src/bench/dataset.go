package bench

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math/rand"
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
	file, err := os.Open(filepath.Join(DataPath, "user.tsv"))
	must(err)
	defer file.Close()

	s := bufio.NewScanner(file)
	for i := 0; s.Scan(); i++ {
		line := strings.Split(s.Text(), "\t")
		nickname := line[0]
		addr := line[1]
		loginName := strings.Split(addr, "@")[0]

		if i < 1000 {
			user := &AppUser{
				ID:        uint(i + 1),
				LoginName: loginName,
				Password:  loginName + reverse(loginName),
				Nickname:  nickname,
			}
			DataSet.Users = append(DataSet.Users, user)
		} else {
			user := &AppUser{
				ID:        0, // auto increment
				LoginName: loginName,
				Password:  loginName + reverse(loginName),
				Nickname:  nickname,
			}
			DataSet.NewUsers = append(DataSet.NewUsers, user)
		}
	}
}

func prepareAdministratorDataSet() {
	administrator := &Administrator{
		ID:        uint(1),
		LoginName: "admin",
		Password:  "admin",
		Nickname:  "admin",
	}

	DataSet.Administrators = append(DataSet.Administrators, administrator)
}

func prepareEventDataSet() {
	file, err := os.Open(filepath.Join(DataPath, "event.tsv"))
	must(err)
	defer file.Close()

	s := bufio.NewScanner(file)
	for i := 0; s.Scan(); i++ {
		line := strings.Split(s.Text(), "\t")
		title := line[0]
		public_fg, _ := strconv.ParseBool(line[1])
		price, _ := strconv.Atoi(line[2])

		event := &Event{
			ID:       uint(i + 1),
			Title:    title,
			PublicFg: public_fg,
			Price:    uint(price),
		}

		DataSet.Events = append(DataSet.Events, event)
	}

	for i := 0; i < 100; i++ {
		event := &Event{
			ID:       0, // auto increment
			Title:    RandomAlphabetString(32),
			PublicFg: true,
			Price:    uint(rand.Intn(10) * 1000),
		}
		DataSet.NewEvents = append(DataSet.NewEvents, event)
	}
}

func prepareSheetDataSet() {
	SheetKinds := []struct {
		Rank     string
		TotalNum int
		Price    uint
	}{
		{"S", 50, 5000},
		{"A", 150, 3000},
		{"B", 300, 1000},
		{"C", 500, 0},
	}

	next_id := uint(1)
	for _, sheet_kind := range SheetKinds {
		for i := 0; i < sheet_kind.TotalNum; i++ {
			sheet := &Sheet{
				ID:    next_id,
				Rank:  strings.ToUpper(sheet_kind.Rank),
				Num:   uint(i + 1),
				Price: sheet_kind.Price,
			}
			next_id++
			DataSet.Sheets = append(DataSet.Sheets, sheet)
		}
	}
}

func PrepareDataSet() {
	log.Println("datapath", DataPath)
	prepareUserDataSet()
	prepareAdministratorDataSet()
	prepareEventDataSet()
	prepareSheetDataSet()
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
		case bool:
			if v {
				params[i] = strconv.Quote("1")
			} else {
				params[i] = strconv.Quote("0")
			}
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
	for _, user := range DataSet.Users {
		passDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(user.Password)))
		must(err)
		fbadf(w, "INSERT INTO users (id, nickname, login_name, pass_hash) VALUES (%s, %s, %s, %s);",
			user.ID, user.Nickname, user.LoginName, passDigest)
	}

	// administrator
	for _, administrator := range DataSet.Administrators {
		passDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(administrator.Password)))
		must(err)
		fbadf(w, "INSERT INTO administrators (id, nickname, login_name, pass_hash) VALUES (%s, %s, %s, %s);",
			administrator.ID, administrator.Nickname, administrator.LoginName, passDigest)
	}

	// event
	for _, event := range DataSet.Events {
		must(err)
		fbadf(w, "INSERT INTO events (id, title, public_fg, price) VALUES (%s, %s, %s, %s);",
			event.ID, event.Title, event.PublicFg, event.Price)
	}

	// sheet
	for _, sheet := range DataSet.Sheets {
		must(err)
		fbadf(w, "INSERT INTO sheets (id, `rank`, num, price) VALUES (%s, %s, %s, %s);",
			sheet.ID, sheet.Rank, sheet.Num, sheet.Price)
	}

	fbadf(w, "COMMIT;")
}
