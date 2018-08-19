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

	"bench/parameter"
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

		if i < parameter.InitialNumUsers {
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

	file, err := os.Open(filepath.Join(DataPath, "admin.tsv"))
	must(err)
	defer file.Close()

	nextID := uint(2)
	s := bufio.NewScanner(file)
	for i := 0; s.Scan(); i++ {
		line := strings.Split(s.Text(), "\t")
		nickname := line[0]
		addr := line[1]
		loginName := strings.Split(addr, "@")[0]

		administrator := &Administrator{
			ID:        nextID,
			LoginName: loginName,
			Password:  loginName + reverse(loginName),
			Nickname:  nickname,
		}
		nextID++
		DataSet.Administrators = append(DataSet.Administrators, administrator)
	}
}

func prepareEventDataSet() {
	nextID := uint(1)

	// Events from event.tsv which are not closed yet
	file, err := os.Open(filepath.Join(DataPath, "event.tsv"))
	must(err)
	defer file.Close()

	s := bufio.NewScanner(file)
	for i := 0; s.Scan(); i++ {
		line := strings.Split(s.Text(), "\t")
		title := line[0]
		publicFg, _ := strconv.ParseBool(line[1])
		closedFg, _ := strconv.ParseBool(line[2])
		price, _ := strconv.Atoi(line[3])

		event := &Event{
			ID:       nextID,
			Title:    title,
			PublicFg: publicFg,
			ClosedFg: closedFg,
			Price:    uint(price),
		}

		DataSet.Events = append(DataSet.Events, event)
		nextID += 1
	}

	// Old events which are already closed
	numClosedEvents := parameter.InitialNumClosedEvents
	priceStrides := numClosedEvents/10 + 1
	for i := 0; i < numClosedEvents; i++ {
		event := &Event{
			ID:       nextID,
			Title:    fmt.Sprintf("Event%04d", nextID),
			PublicFg: false,
			ClosedFg: true,
			Price:    uint(1000 + i/priceStrides*1000),
		}
		DataSet.ClosedEvents = append(DataSet.ClosedEvents, event)
		nextID += 1
	}
}

func prepareSheetDataSet() {
	DataSet.SheetKinds = []*SheetKind{
		{"S", 50, 5000},
		{"A", 150, 3000},
		{"B", 300, 1000},
		{"C", 500, 0},
	}

	nextID := uint(1)
	for _, sheetKind := range DataSet.SheetKinds {
		for i := uint(0); i < sheetKind.Total; i++ {
			sheet := &Sheet{
				ID:    nextID,
				Rank:  strings.ToUpper(sheetKind.Rank),
				Num:   uint(i + 1),
				Price: sheetKind.Price,
			}
			nextID++
			DataSet.Sheets = append(DataSet.Sheets, sheet)
		}
	}
}

func prepareReservationsDataSet() {
	nextID := uint(1)
	minUnixTimestamp := time.Date(2011, 8, 27, 10, 0, 0, 0, time.Local).Unix()
	maxUnixTimestamp := time.Date(2017, 10, 21, 10, 0, 0, 0, time.Local).Unix()
	for _, event := range DataSet.ClosedEvents {
		for _, sheet := range DataSet.Sheets {
			userID := uint(rand.Intn(len(DataSet.Users)) + 1)
			reservation := &Reservation{
				ID:         nextID,
				EventID:    event.ID,
				UserID:     userID,
				SheetID:    sheet.ID,
				SheetRank:  sheet.Rank,
				SheetNum:   sheet.Num,
				ReservedAt: int64(rand.Int63n(maxUnixTimestamp-minUnixTimestamp) + minUnixTimestamp), // TODO(sonots): randomize nsec
			}
			nextID++
			DataSet.Reservations = append(DataSet.Reservations, reservation)
		}
	}
}

func PrepareDataSet() {
	log.Println("datapath", DataPath)
	prepareUserDataSet()
	prepareAdministratorDataSet()
	prepareEventDataSet()
	prepareSheetDataSet()
	prepareReservationsDataSet()
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
	for _, event := range append(DataSet.Events, DataSet.ClosedEvents...) {
		fbadf(w, "INSERT INTO events (id, title, public_fg, closed_fg, price) VALUES (%s, %s, %s, %s, %s);",
			event.ID, event.Title, event.PublicFg, event.ClosedFg, event.Price)
	}

	// sheet
	for _, sheet := range DataSet.Sheets {
		fbadf(w, "INSERT INTO sheets (id, `rank`, num, price) VALUES (%s, %s, %s, %s);",
			sheet.ID, sheet.Rank, sheet.Num, sheet.Price)
	}

	// reservation
	for i, reservation := range DataSet.Reservations {
		if i%1000 == 0 {
			fbadf(w, ";INSERT INTO reservations (id, event_id, sheet_id, user_id, reserved_at) VALUES ")
		} else {
			fbadf(w, ", ")
		}
		fbadf(w, "(%s, %s, %s, %s, %s)", reservation.ID, reservation.EventID, reservation.SheetID, reservation.UserID, time.Unix(reservation.ReservedAt, 0).UTC().Format("2006-01-02 15:04:05"))
	}
	fbadf(w, ";")

	fbadf(w, "COMMIT;")
}
