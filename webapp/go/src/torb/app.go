package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "net/http/pprof"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/middleware"
	measure "github.com/najeira/measure"
)

type User struct {
	ID        int64  `json:"id,omitempty"`
	Nickname  string `json:"nickname,omitempty"`
	LoginName string `json:"login_name,omitempty"`
	PassHash  string `json:"pass_hash,omitempty"`
}

type Event struct {
	ID       int64  `json:"id,omitempty"`
	Title    string `json:"title,omitempty"`
	PublicFg bool   `json:"public,omitempty"`
	ClosedFg bool   `json:"closed,omitempty"`
	Price    int64  `json:"price,omitempty"`

	Total   int                `json:"total"`
	Remains int                `json:"remains"`
	Sheets  map[string]*Sheets `json:"sheets,omitempty"`
}

type Sheets struct {
	Total   int      `json:"total"`
	Remains int      `json:"remains"`
	Detail  []*Sheet `json:"detail,omitempty"`
	Price   int64    `json:"price"`
}

type Sheet struct {
	ID    int64  `json:"-"`
	Rank  string `json:"-"`
	Num   int64  `json:"num"`
	Price int64  `json:"-"`

	Mine           bool       `json:"mine,omitempty"`
	Reserved       bool       `json:"reserved,omitempty"`
	ReservedAt     *time.Time `json:"-"`
	ReservedAtUnix int64      `json:"reserved_at,omitempty"`
}

type Reservation struct {
	ID         int64      `json:"id"`
	EventID    int64      `json:"-"`
	SheetID    int64      `json:"-"`
	UserID     int64      `json:"-"`
	ReservedAt *time.Time `json:"-"`
	CanceledAt *time.Time `json:"-"`

	Event              *Event `json:"event,omitempty"`
	SheetRank          string `json:"sheet_rank,omitempty"`
	SheetNum           int64  `json:"sheet_num,omitempty"`
	Price              int64  `json:"price,omitempty"`
	ReservedAtUnix     int64  `json:"reserved_at,omitempty"`
	CanceledAtUnix     int64  `json:"canceled_at,omitempty"`
	LatestActionAtUnix int64  `json:"-",omitempty`
}

type Administrator struct {
	ID        int64  `json:"id,omitempty"`
	Nickname  string `json:"nickname,omitempty"`
	LoginName string `json:"login_name,omitempty"`
	PassHash  string `json:"pass_hash,omitempty"`
}

var SheetsMaster []Sheet

func sessUserID(c echo.Context) User {
	defer measure.Start(
		"sessUserID",
	).Stop()

	var user User
	sess, _ := session.Get("session", c)
	if x, ok := sess.Values["user_id"]; ok {
		user.ID = x.(int64)
	}
	if x, ok := sess.Values["user_nickname"]; ok {
		user.Nickname = x.(string)
	}
	return user
}

func sessSetUserID(c echo.Context, user *User) {
	defer measure.Start(
		"sessSetUserID",
	).Stop()

	sess, _ := session.Get("session", c)
	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   3600,
		HttpOnly: true,
	}
	sess.Values["user_id"] = user.ID
	sess.Values["user_nickname"] = user.Nickname
	sess.Save(c.Request(), c.Response())
}

func sessDeleteUserID(c echo.Context) {
	defer measure.Start(
		"sessDeleteUserID",
	).Stop()

	sess, _ := session.Get("session", c)
	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   3600,
		HttpOnly: true,
	}
	delete(sess.Values, "user_id")
	sess.Save(c.Request(), c.Response())
}

func sessAdministratorID(c echo.Context) int64 {
	defer measure.Start(
		"sessAdministratorID",
	).Stop()

	sess, _ := session.Get("session", c)
	var administratorID int64
	if x, ok := sess.Values["administrator_id"]; ok {
		administratorID, _ = x.(int64)
	}
	return administratorID
}

func sessSetAdministratorID(c echo.Context, id int64) {
	defer measure.Start(
		"sessSetAdministratorID",
	).Stop()

	sess, _ := session.Get("session", c)
	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   3600,
		HttpOnly: true,
	}
	sess.Values["administrator_id"] = id
	sess.Save(c.Request(), c.Response())
}

func sessDeleteAdministratorID(c echo.Context) {
	defer measure.Start(
		"sessDeleteAdministratorID",
	).Stop()

	sess, _ := session.Get("session", c)
	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   3600,
		HttpOnly: true,
	}
	delete(sess.Values, "administrator_id")
	sess.Save(c.Request(), c.Response())
}

func loginRequired(next echo.HandlerFunc) echo.HandlerFunc {
	defer measure.Start(
		"loginRequired",
	).Stop()

	return func(c echo.Context) error {
		defer measure.Start(
			"loginRequired-1",
		).
			Stop()

		if _, err := getLoginUser(c); err != nil {
			return resError(c, "login_required", 401)
		}
		return next(c)
	}
}

func adminLoginRequired(next echo.HandlerFunc) echo.HandlerFunc {
	defer measure.Start(
		"adminLoginRequired",
	).Stop()

	return func(c echo.Context) error {
		defer measure.Start(
			"adminLoginRequired-1",
		).Stop()

		if _, err := getLoginAdministrator(c); err != nil {
			return resError(c, "admin_login_required", 401)
		}
		return next(c)
	}
}

func getLoginUser(c echo.Context) (*User, error) {
	defer measure.Start(
		"getLoginUser",
	).Stop()

	var user User
	user = sessUserID(c)
	if user.ID == 0 {
		return nil, errors.New("not logged in")
	}
	return &user, nil
	//err := db.QueryRow("SELECT id, nickname FROM users WHERE id = ?", userID).Scan(&user.ID, &user.Nickname)
	//return &user, err
}

func getLoginAdministrator(c echo.Context) (*Administrator, error) {
	defer measure.Start(
		"getLoginAdministrator",
	).Stop()

	administratorID := sessAdministratorID(c)
	if administratorID == 0 {
		return nil, errors.New("not logged in")
	}
	var administrator Administrator
	err := db.QueryRow("SELECT id, nickname FROM administrators WHERE id = ?", administratorID).Scan(&administrator.ID, &administrator.Nickname)
	return &administrator, err
}

func getEvents(all bool) ([]*Event, error) {
	defer measure.Start(
		"getEvents",
	).Stop()

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Commit()

	rows, err := tx.Query("SELECT * FROM events ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.Title, &event.PublicFg, &event.ClosedFg, &event.Price); err != nil {
			return nil, err
		}
		if !all && !event.PublicFg {
			continue
		}
		events = append(events, &event)
	}

	for i, v := range events {
		//event, err := getEvent(v.ID, -1)
		event, err := getEventWithoutDetail(v)
		if err != nil {
			return nil, err
		}
		for k := range event.Sheets {
			event.Sheets[k].Detail = nil
		}
		events[i] = event
	}

	return events, nil
}

// user_idを使わないイベント情報取得用
func getEventWithoutDetail(event *Event) (*Event, error) {
	defer measure.Start("getEventWithoutDetail").Stop()

	event.Sheets = map[string]*Sheets{}
	rows, err := db.Query("SELECT sheet_id FROM reservations WHERE event_id = ? AND canceled_at IS NULL", event.ID)
	if err != nil {
		return nil, err
	}

	var reservedList []Sheet
	// とりあえず予約一覧取得
	for rows.Next() {
		var sheet_id int64
		if err := rows.Scan(&sheet_id); err != nil {
			return nil, err
		}
		reservedList = append(reservedList, getSheetsInfo(sheet_id))
	}

	//var sheets Sheets
	event.Sheets = map[string]*Sheets{
		"S": {},
		"A": {},
		"B": {},
		"C": {},
	}
	event.Remains = 1000
	event.Total = 1000
	event.Sheets["S"].Remains = 50
	event.Sheets["S"].Total = 50
	event.Sheets["S"].Price = 5000 + event.Price
	event.Sheets["A"].Remains = 150
	event.Sheets["A"].Total = 150
	event.Sheets["A"].Price = 3000 + event.Price
	event.Sheets["B"].Remains = 300
	event.Sheets["B"].Total = 300
	event.Sheets["B"].Price = 1000 + event.Price
	event.Sheets["C"].Remains = 500
	event.Sheets["C"].Total = 500
	event.Sheets["C"].Price = event.Price
	for _, reservation := range reservedList {
		event.Sheets[reservation.Rank].Remains--
		event.Remains--
	}

	return event, nil
}

func getEvent(eventID, loginUserID int64) (*Event, error) {
	defer measure.Start(
		"getEvent",
	).Stop()

	var event Event
	if err := db.QueryRow("SELECT * FROM events WHERE id = ?", eventID).Scan(&event.ID, &event.Title, &event.PublicFg, &event.ClosedFg, &event.Price); err != nil {
		return nil, err
	}
	event.Sheets = map[string]*Sheets{
		"S": {},
		"A": {},
		"B": {},
		"C": {},
	}

	// TODO: なんとかする
	event.Remains = 1000
	event.Sheets["S"].Remains = 50
	event.Sheets["A"].Remains = 150
	event.Sheets["B"].Remains = 300
	event.Sheets["C"].Remains = 500

	// この変更で最大100ms以上かかっていたものが目視で~15ms
	//	reserved_sheets, err := db.Query("SELECT COALESCE(user_id, 0) AS user_id, sheets.id, reserved_at, sheets.rank, sheets.price, sheets.num FROM sheets LEFT OUTER JOIN reservations ON sheets.id = reservations.sheet_id AND event_id = ? AND canceled_at IS NULL", eventID)
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	for reserved_sheets.Next() {
	//		var sheet Sheet
	//		var reservation Reservation
	//		if err := reserved_sheets.Scan(&reservation.UserID, &sheet.ID, &reservation.ReservedAt, &sheet.Rank, &sheet.Price, &sheet.Num); err != nil {
	//			return nil, err
	//		}
	//
	//		if reservation.ReservedAt != nil {
	//			sheet.Mine = reservation.UserID == loginUserID
	//			sheet.Reserved = true
	//			sheet.ReservedAtUnix = reservation.ReservedAt.Unix()
	//			event.Remains--
	//			event.Sheets[sheet.Rank].Remains--
	//		}
	//
	//		event.Total++
	//		event.Sheets[sheet.Rank].Total++
	//		event.Sheets[sheet.Rank].Price = event.Price + sheet.Price
	//		event.Sheets[sheet.Rank].Detail = append(event.Sheets[sheet.Rank].Detail, &sheet)
	//	}

	event.Total = 1000
	event.Sheets["S"].Total = 50
	event.Sheets["A"].Total = 150
	event.Sheets["B"].Total = 300
	event.Sheets["C"].Total = 500
	reserved_sheets, err := db.Query("SELECT user_id, sheet_id, reserved_at FROM reservations WHERE event_id = ? AND canceled_at IS NULL", eventID)
	if err != nil {
		return nil, err
	}
	reserved := make(map[int64]Reservation)
	for reserved_sheets.Next() {
		var r Reservation
		if err := reserved_sheets.Scan(&r.UserID, &r.SheetID, &r.ReservedAt); err != nil {
			return nil, err
		}
		reserved[r.SheetID] = r
	}

	for _, s := range SheetsMaster {
		s := s
		if reserved[s.ID].ReservedAt != nil {
			s.Mine = reserved[s.ID].UserID == loginUserID
			s.Reserved = true
			s.ReservedAtUnix = reserved[s.ID].ReservedAt.Unix()
			event.Remains--
			event.Sheets[s.Rank].Remains--
		}
		event.Sheets[s.Rank].Price = event.Price + s.Price
		event.Sheets[s.Rank].Detail = append(event.Sheets[s.Rank].Detail, &s)
	}

	return &event, nil
}

func sanitizeEvent(e *Event) *Event {
	defer measure.Start(
		"sanitizeEvent",
	).Stop()

	sanitized := *e
	sanitized.Price = 0
	sanitized.PublicFg = false
	sanitized.ClosedFg = false
	return &sanitized
}

func fillinUser(next echo.HandlerFunc) echo.HandlerFunc {
	defer measure.Start(
		"fillinUser",
	).Stop()

	return func(c echo.Context) error {
		defer measure.Start(
			"fillinUser-1",
		).Stop()

		if user, err := getLoginUser(c); err == nil {
			c.Set("user", user)
		}
		return next(c)
	}
}

func fillinAdministrator(next echo.HandlerFunc) echo.HandlerFunc {
	defer measure.Start(
		"fillinAdministrator",
	).Stop()

	return func(c echo.Context) error {
		defer measure.Start(
			"fillinAdministrator-1",
		).Stop()

		if administrator, err := getLoginAdministrator(c); err == nil {
			c.Set("administrator", administrator)
		}
		return next(c)
	}
}

func validateRank(rank string) bool {
	defer measure.Start(
		"validateRank",
	).Stop()

	ranks := [4]string{"S", "A", "B", "C"}
	for _, r := range ranks {
		if r == rank {
			return true
		}
	}

	return false
}

type Renderer struct {
	templates *template.Template
}

func (r *Renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	defer measure.Start(
		"Render").
		Stop()

	return r.templates.ExecuteTemplate(w, name, data)
}

var db *sql.DB

func main() {
	defer measure.Start(
		"main").Stop()

	go func() {
		defer measure.Start(
			"main-1").
			Stop()

		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4",
		os.Getenv("DB_USER"), os.Getenv("DB_PASS"),
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"),
		os.Getenv("DB_DATABASE"),
	)

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}

	SheetsMaster, err = getSheetsMaster()
	if err != nil {
		log.Fatal(err)
	}

	e := echo.New()
	funcs := template.FuncMap{
		"encode_json": func(v interface{}) string {
			defer measure.Start(
				"main-2").
				Stop()

			b, _ := json.Marshal(v)
			return string(b)
		},
	}
	e.Renderer = &Renderer{
		templates: template.Must(template.New("").Delims("[[", "]]").Funcs(funcs).ParseGlob("views/*.tmpl")),
	}
	e.Use(session.Middleware(sessions.NewCookieStore([]byte("secret"))))
	// e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{Output: os.Stderr}))
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{Output: ioutil.Discard}))
	e.Static("/", "public")
	e.GET("/", func(c echo.Context) error {
		defer measure.Start(
			"main-3").
			Stop()

		events, err := getEvents(false)
		if err != nil {
			return err
		}
		for i, v := range events {
			events[i] = sanitizeEvent(v)
		}
		return c.Render(200, "index.tmpl", echo.Map{
			"events": events,
			"user":   c.Get("user"),
			"origin": c.Scheme() + "://" + c.Request().Host,
		})
	}, fillinUser)
	e.GET("/initialize", func(c echo.Context) error {
		defer measure.Start(
			"main-4").
			Stop()

		cmd := exec.Command("../../db/init.sh")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		err := cmd.Run()
		if err != nil {
			return nil
		}

		// インデックス作成
		_, err = db.Exec("CREATE INDEX user_id_index ON reservations(user_id)")
		if err != nil {
			return nil
		}
		_, err = db.Exec("CREATE INDEX event_cancel_index ON reservations(event_id, canceled_at)")
		if err != nil {
			return nil
		}

		return c.NoContent(204)
	})
	e.POST("/api/users", func(c echo.Context) error {
		defer measure.Start(
			"main-5").
			Stop()

		var params struct {
			Nickname  string `json:"nickname"`
			LoginName string `json:"login_name"`
			Password  string `json:"password"`
		}
		c.Bind(&params)

		tx, err := db.Begin()
		if err != nil {
			return err
		}

		var user User
		if err := tx.QueryRow("SELECT * FROM users WHERE login_name = ?", params.LoginName).Scan(&user.ID, &user.LoginName, &user.Nickname, &user.PassHash); err != sql.ErrNoRows {
			tx.Rollback()
			if err == nil {
				return resError(c, "duplicated", 409)
			}
			return err
		}

		res, err := tx.Exec("INSERT INTO users (login_name, pass_hash, nickname) VALUES (?, SHA2(?, 256), ?)", params.LoginName, params.Password, params.Nickname)
		if err != nil {
			tx.Rollback()
			return resError(c, "", 0)
		}
		userID, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return resError(c, "", 0)
		}
		if err := tx.Commit(); err != nil {
			return err
		}

		return c.JSON(201, echo.Map{
			"id":       userID,
			"nickname": params.Nickname,
		})
	})
	e.GET("/api/users/:id", func(c echo.Context) error {
		defer measure.Start(
			"main-6").
			Stop()

		//var user User
		//if err := db.QueryRow("SELECT id, nickname FROM users WHERE id = ?", c.Param("id")).Scan(&user.ID, &user.Nickname); err != nil {
		//	return err
		//}

		user, err := getLoginUser(c)
		if err != nil {
			return err
		}
		if c.Param("id") != strconv.Itoa(int(user.ID)) {
			return resError(c, "forbidden", 403)
		}

		//rows, err := db.Query("SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id WHERE r.user_id = ? ORDER BY IFNULL(r.canceled_at, r.reserved_at) DESC LIMIT 5", user.ID)
		rows, err := db.Query("SELECT r.*, e.id, e.title, e.public_fg, e.closed_fg, e.price AS event_price FROM reservations r INNER JOIN events e ON e.id = r.event_id WHERE r.user_id = ?", user.ID)
		if err != nil {
			return err
		}
		defer rows.Close()

		var recentReservations []Reservation
		for rows.Next() {
			var reservation Reservation
			var event Event
			//if err := rows.Scan(&reservation.ID, &reservation.EventID, &reservation.SheetID, &reservation.UserID, &reservation.ReservedAt, &reservation.CanceledAt, &sheet.Rank, &sheet.Num); err != nil {
			if err := rows.Scan(&reservation.ID, &reservation.EventID, &reservation.SheetID, &reservation.UserID, &reservation.ReservedAt, &reservation.CanceledAt, &event.ID, &event.Title, &event.PublicFg, &event.ClosedFg, &event.Price); err != nil {
				return err
			}

			//event, err := getEvent(reservation.EventID, -1)
			//if err != nil {
			//        return err
			//}
			//price := event.Sheets[sheet.Rank].Price

			// sheet情報を取得
			sheet := getSheetsInfo(reservation.SheetID)

			price := event.Price + sheet.Price
			event.Sheets = nil
			event.Total = 0
			event.Remains = 0

			reservation.Event = &event
			reservation.SheetRank = sheet.Rank
			reservation.SheetNum = sheet.Num
			reservation.Price = price
			reservation.ReservedAtUnix = reservation.ReservedAt.Unix()
			if reservation.CanceledAt != nil {
				reservation.CanceledAtUnix = reservation.CanceledAt.Unix()
				reservation.LatestActionAtUnix = reservation.CanceledAtUnix
			} else {
				reservation.LatestActionAtUnix = reservation.ReservedAtUnix
			}
			recentReservations = append(recentReservations, reservation)
		}
		if recentReservations == nil {
			recentReservations = make([]Reservation, 0)
		}

		// recentReservationsをいい感じに並び替える
		sort.Slice(recentReservations, func(i, j int) bool {
			return recentReservations[i].LatestActionAtUnix > recentReservations[j].LatestActionAtUnix
		})
		if len(recentReservations) >= 5 {
			recentReservations = recentReservations[:5]
		}
		for i := range recentReservations {
			recentReservations[i].LatestActionAtUnix = 0
		}

		var totalPrice int
		if err := db.QueryRow("SELECT IFNULL(SUM(e.price + s.price), 0) FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.user_id = ? AND r.canceled_at IS NULL", user.ID).Scan(&totalPrice); err != nil {
			return err
		}

		rows, err = db.Query("SELECT event_id FROM reservations WHERE user_id = ? GROUP BY event_id ORDER BY MAX(IFNULL(canceled_at, reserved_at)) DESC LIMIT 5", user.ID)
		if err != nil {
			return err
		}
		defer rows.Close()

		var recentEvents []*Event
		for rows.Next() {
			var eventID int64
			if err := rows.Scan(&eventID); err != nil {
				return err
			}
			event, err := getEvent(eventID, -1)
			//event, err := getEventWithoutDetail(eventID)
			if err != nil {
				return err
			}
			for k := range event.Sheets {
				event.Sheets[k].Detail = nil
			}
			recentEvents = append(recentEvents, event)
		}
		if recentEvents == nil {
			recentEvents = make([]*Event, 0)
		}

		return c.JSON(200, echo.Map{
			"id":                  user.ID,
			"nickname":            user.Nickname,
			"recent_reservations": recentReservations,
			"total_price":         totalPrice,
			"recent_events":       recentEvents,
		})
	}, loginRequired)
	e.POST("/api/actions/login", func(c echo.Context) error {
		defer measure.Start(
			"main-7").
			Stop()

		var params struct {
			LoginName string `json:"login_name"`
			Password  string `json:"password"`
		}
		c.Bind(&params)

		user := new(User)
		if err := db.QueryRow("SELECT * FROM users WHERE login_name = ?", params.LoginName).Scan(&user.ID, &user.Nickname, &user.LoginName, &user.PassHash); err != nil {
			if err == sql.ErrNoRows {
				return resError(c, "authentication_failed", 401)
			}
			return err
		}

		var passHash string
		if err := db.QueryRow("SELECT SHA2(?, 256)", params.Password).Scan(&passHash); err != nil {
			return err
		}
		if user.PassHash != passHash {
			return resError(c, "authentication_failed", 401)
		}

		sessSetUserID(c, user)
		user, err = getLoginUser(c)
		if err != nil {
			return err
		}
		return c.JSON(200, user)
	})
	e.POST("/api/actions/logout", func(c echo.Context) error {
		defer measure.Start(
			"main-8").
			Stop()

		sessDeleteUserID(c)
		return c.NoContent(204)
	}, loginRequired)
	e.GET("/api/events", func(c echo.Context) error {
		defer measure.Start(
			"main-9").
			Stop()

		events, err := getEvents(true)
		if err != nil {
			return err
		}
		for i, v := range events {
			events[i] = sanitizeEvent(v)
		}
		return c.JSON(200, events)
	})
	e.GET("/api/events/:id", func(c echo.Context) error {
		defer measure.Start(
			"main-10",
		).Stop()

		eventID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return resError(c, "not_found", 404)
		}

		loginUserID := int64(-1)
		if user, err := getLoginUser(c); err == nil {
			loginUserID = user.ID
		}

		event, err := getEvent(eventID, loginUserID)
		if err != nil {
			if err == sql.ErrNoRows {
				return resError(c, "not_found", 404)
			}
			return err
		} else if !event.PublicFg {
			return resError(c, "not_found", 404)
		}
		return c.JSON(200, sanitizeEvent(event))
	})
	// TODO: 次ここ 2020年4月13日
	e.POST("/api/events/:id/actions/reserve", func(c echo.Context) error {
		defer measure.Start(
			"main-11",
		).Stop()

		start := time.Now()

		eventID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return resError(c, "not_found", 404)
		}
		var params struct {
			Rank string `json:"sheet_rank"`
		}
		c.Bind(&params)

		user, err := getLoginUser(c)
		if err != nil {
			return err
		}

		log.Println("ユーザ情報取得")
		log.Println(time.Since(start))
		start = time.Now()

		event, err := getEvent(eventID, user.ID)
		if err != nil {
			if err == sql.ErrNoRows {
				return resError(c, "invalid_event", 404)
			}
			return err
		} else if !event.PublicFg {
			return resError(c, "invalid_event", 404)
		}

		if !validateRank(params.Rank) {
			return resError(c, "invalid_rank", 400)
		}

		log.Println("イベント情報取得")
		log.Println(time.Since(start))
		start = time.Now()

		var sheet Sheet
		var reservationID int64
		for {
			if err := db.QueryRow("SELECT * FROM sheets WHERE id NOT IN (SELECT sheet_id FROM reservations WHERE event_id = ? AND canceled_at IS NULL FOR UPDATE) AND `rank` = ? ORDER BY RAND() LIMIT 1", event.ID, params.Rank).Scan(&sheet.ID, &sheet.Rank, &sheet.Num, &sheet.Price); err != nil {
				//if err := db.QueryRow("SELECT * FROM sheets WHERE id NOT IN (SELECT sheet_id FROM reservations WHERE event_id = ? AND canceled_at IS NULL) AND `rank` = ? ORDER BY RAND() LIMIT 1", event.ID, params.Rank).Scan(&sheet.ID, &sheet.Rank, &sheet.Num, &sheet.Price); err != nil {
				if err == sql.ErrNoRows {
					return resError(c, "sold_out", 409)
				}
				return err
			}

			log.Println("予約席を見つけるクエリ")
			log.Println(time.Since(start))
			start = time.Now()

			// TODO: 予約する席を見つける

			tx, err := db.Begin()
			if err != nil {
				return err
			}

			res, err := tx.Exec("INSERT INTO reservations (event_id, sheet_id, user_id, reserved_at) VALUES (?, ?, ?, ?)", event.ID, sheet.ID, user.ID, time.Now().UTC().Format("2006-01-02 15:04:05.000000"))
			if err != nil {
				tx.Rollback()
				log.Println("re-try: rollback by", err)
				continue
			}

			log.Println("予約情報登録")
			log.Println(time.Since(start))
			start = time.Now()

			reservationID, err = res.LastInsertId()
			if err != nil {
				tx.Rollback()
				log.Println("re-try: rollback by", err)
				continue
			}
			if err := tx.Commit(); err != nil {
				tx.Rollback()
				log.Println("re-try: rollback by", err)
				continue
			}

			break
		}
		return c.JSON(202, echo.Map{
			"id":         reservationID,
			"sheet_rank": params.Rank,
			"sheet_num":  sheet.Num,
		})
	}, loginRequired)
	// TODO: 次ここ 2020/03/30
	e.DELETE("/api/events/:id/sheets/:rank/:num/reservation", func(c echo.Context) error {
		defer measure.Start(
			"main-12",
		).Stop()

		eventID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return resError(c, "not_found", 404)
		}
		rank := c.Param("rank")
		num := c.Param("num")

		user, err := getLoginUser(c)
		if err != nil {
			return err
		}

		event, err := getEvent(eventID, user.ID)
		if err != nil {
			if err == sql.ErrNoRows {
				return resError(c, "invalid_event", 404)
			}
			return err
		} else if !event.PublicFg {
			return resError(c, "invalid_event", 404)
		}

		if !validateRank(rank) {
			return resError(c, "invalid_rank", 404)
		}

		var sheet Sheet
		for _, s := range SheetsMaster {
			n, err := strconv.ParseInt(num, 10, 64)
			if err != nil {
				return resError(c, "", 501)
			}
			if s.Rank == rank && s.Num == n {
				sheet = s
				break
			}
		}
		if sheet.ID == 0 {
			return resError(c, "invalid_sheet", 404)
		}
		// ↑追加して↓削除(dbへのアクセス減らした)
		// この変更で"座席情報取得"が4~6ms→50μs
		//if err := db.QueryRow("SELECT * FROM sheets WHERE `rank` = ? AND num = ?", rank, num).Scan(&sheet.ID, &sheet.Rank, &sheet.Num, &sheet.Price); err != nil {
		//	if err == sql.ErrNoRows {
		//		return resError(c, "invalid_sheet", 404)
		//	}
		//	return err
		//}

		tx, err := db.Begin()
		if err != nil {
			return err
		}

		var reservation Reservation
		if err := tx.QueryRow("SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id HAVING reserved_at = MIN(reserved_at) FOR UPDATE", event.ID, sheet.ID).Scan(&reservation.ID, &reservation.EventID, &reservation.SheetID, &reservation.UserID, &reservation.ReservedAt, &reservation.CanceledAt); err != nil {
			tx.Rollback()
			if err == sql.ErrNoRows {
				return resError(c, "not_reserved", 400)
			}
			return err
		}
		if reservation.UserID != user.ID {
			tx.Rollback()
			return resError(c, "not_permitted", 403)
		}

		if _, err := tx.Exec("UPDATE reservations SET canceled_at = ? WHERE id = ?", time.Now().UTC().Format("2006-01-02 15:04:05.000000"), reservation.ID); err != nil {
			tx.Rollback()
			return resError(c, "rollback", 499)
			//return err
		}

		if err := tx.Commit(); err != nil {
			return resError(c, "commit error", 499)
			//return err
		}

		return c.NoContent(204)
	}, loginRequired)
	// 次ここ修正する 2020/03/23
	e.GET("/admin/", func(c echo.Context) error {
		defer measure.Start(
			"main-13",
		).Stop()

		var events []*Event
		administrator := c.Get("administrator")
		if administrator != nil {
			var err error
			// まずはここを見る
			if events, err = getEvents(true); err != nil {
				return err
			}
		}
		return c.Render(200, "admin.tmpl", echo.Map{
			"events":        events,
			"administrator": administrator,
			"origin":        c.Scheme() + "://" + c.Request().Host,
		})
	}, fillinAdministrator)
	e.POST("/admin/api/actions/login", func(c echo.Context) error {
		defer measure.Start(
			"main-14",
		).Stop()

		var params struct {
			LoginName string `json:"login_name"`
			Password  string `json:"password"`
		}
		c.Bind(&params)

		administrator := new(Administrator)
		if err := db.QueryRow("SELECT * FROM administrators WHERE login_name = ?", params.LoginName).Scan(&administrator.ID, &administrator.LoginName, &administrator.Nickname, &administrator.PassHash); err != nil {
			if err == sql.ErrNoRows {
				return resError(c, "authentication_failed", 401)
			}
			return err
		}

		var passHash string
		if err := db.QueryRow("SELECT SHA2(?, 256)", params.Password).Scan(&passHash); err != nil {
			return err
		}
		if administrator.PassHash != passHash {
			return resError(c, "authentication_failed", 401)
		}

		sessSetAdministratorID(c, administrator.ID)
		administrator, err = getLoginAdministrator(c)
		if err != nil {
			return err
		}
		return c.JSON(200, administrator)
	})
	e.POST("/admin/api/actions/logout", func(c echo.Context) error {
		defer measure.Start(
			"main-15",
		).Stop()

		sessDeleteAdministratorID(c)
		return c.NoContent(204)
	}, adminLoginRequired)
	e.GET("/admin/api/events", func(c echo.Context) error {
		defer measure.Start(
			"main-16",
		).Stop()

		events, err := getEvents(true)
		if err != nil {
			return err
		}
		return c.JSON(200, events)
	}, adminLoginRequired)
	e.POST("/admin/api/events", func(c echo.Context) error {
		defer measure.Start(
			"main-17",
		).Stop()

		var params struct {
			Title  string `json:"title"`
			Public bool   `json:"public"`
			Price  int    `json:"price"`
		}
		c.Bind(&params)

		tx, err := db.Begin()
		if err != nil {
			return err
		}

		res, err := tx.Exec("INSERT INTO events (title, public_fg, closed_fg, price) VALUES (?, ?, 0, ?)", params.Title, params.Public, params.Price)
		if err != nil {
			tx.Rollback()
			return err
		}
		eventID, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}

		event, err := getEvent(eventID, -1)
		if err != nil {
			return err
		}
		return c.JSON(200, event)
	}, adminLoginRequired)
	e.GET("/admin/api/events/:id", func(c echo.Context) error {
		defer measure.Start(
			"main-18",
		).Stop()

		eventID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return resError(c, "not_found", 404)
		}
		event, err := getEvent(eventID, -1)
		if err != nil {
			if err == sql.ErrNoRows {
				return resError(c, "not_found", 404)
			}
			return err
		}
		return c.JSON(200, event)
	}, adminLoginRequired)
	e.POST("/admin/api/events/:id/actions/edit", func(c echo.Context) error {
		defer measure.Start(
			"main-19",
		).Stop()

		eventID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return resError(c, "not_found", 404)
		}

		var params struct {
			Public bool `json:"public"`
			Closed bool `json:"closed"`
		}
		c.Bind(&params)
		if params.Closed {
			params.Public = false
		}

		event, err := getEvent(eventID, -1)
		if err != nil {
			if err == sql.ErrNoRows {
				return resError(c, "not_found", 404)
			}
			return err
		}

		if event.ClosedFg {
			return resError(c, "cannot_edit_closed_event", 400)
		} else if event.PublicFg && params.Closed {
			return resError(c, "cannot_close_public_event", 400)
		}

		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE events SET public_fg = ?, closed_fg = ? WHERE id = ?", params.Public, params.Closed, event.ID); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}

		e, err := getEvent(eventID, -1)
		if err != nil {
			return err
		}
		c.JSON(200, e)
		return nil
	}, adminLoginRequired)
	e.GET("/admin/api/reports/events/:id/sales", func(c echo.Context) error {
		defer measure.Start(
			"main-20",
		).Stop()

		eventID, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			return resError(c, "not_found", 404)
		}

		event, err := getEvent(eventID, -1)
		if err != nil {
			return err
		}

		rows, err := db.Query("SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num, s.price AS sheet_price, e.price AS event_price FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.event_id = ? FOR UPDATE", event.ID)
		if err != nil {
			return err
		}
		defer rows.Close()

		var reports []Report
		for rows.Next() {
			var reservation Reservation
			var sheet Sheet
			if err := rows.Scan(&reservation.ID, &reservation.EventID, &reservation.SheetID, &reservation.UserID, &reservation.ReservedAt, &reservation.CanceledAt, &sheet.Rank, &sheet.Num, &sheet.Price, &event.Price); err != nil {
				return err
			}
			report := Report{
				ReservationID: reservation.ID,
				EventID:       event.ID,
				Rank:          sheet.Rank,
				Num:           sheet.Num,
				UserID:        reservation.UserID,
				SoldAt:        reservation.ReservedAt.Format("2006-01-02T15:04:05.000000Z"),
				Price:         event.Price + sheet.Price,
			}
			if reservation.CanceledAt != nil {
				report.CanceledAt = reservation.CanceledAt.Format("2006-01-02T15:04:05.000000Z")
			}
			reports = append(reports, report)
		}
		return renderReportCSV(c, reports)
	}, adminLoginRequired)
	e.GET("/admin/api/reports/sales", func(c echo.Context) error {
		defer measure.Start(
			"main-21",
		).Stop()

		rows, err := db.Query("select r.*, s.rank as sheet_rank, s.num as sheet_num, s.price as sheet_price, e.id as event_id, e.price as event_price from reservations r inner join sheets s on s.id = r.sheet_id inner join events e on e.id = r.event_id order by reserved_at asc for update")
		if err != nil {
			return err
		}
		defer rows.Close()

		var reports []Report
		for rows.Next() {
			var reservation Reservation
			var sheet Sheet
			var event Event
			if err := rows.Scan(&reservation.ID, &reservation.EventID, &reservation.SheetID, &reservation.UserID, &reservation.ReservedAt, &reservation.CanceledAt, &sheet.Rank, &sheet.Num, &sheet.Price, &event.ID, &event.Price); err != nil {
				return err
			}
			report := Report{
				ReservationID: reservation.ID,
				EventID:       event.ID,
				Rank:          sheet.Rank,
				Num:           sheet.Num,
				UserID:        reservation.UserID,
				SoldAt:        reservation.ReservedAt.Format("2006-01-02T15:04:05.000000Z"),
				Price:         event.Price + sheet.Price,
			}
			if reservation.CanceledAt != nil {
				report.CanceledAt = reservation.CanceledAt.Format("2006-01-02T15:04:05.000000Z")
			}
			reports = append(reports, report)
		}
		return renderReportCSV(c, reports)
	}, adminLoginRequired)
	e.GET("/status", func(c echo.Context) error {
		stats := measure.GetStats()
		stats.SortDesc("sum")

		// print stats in CSV format
		for _, s := range stats {
			fmt.Fprintf(os.Stdout, "%s,%d,%f,%f,%f,%f,%f,%f\n",
				s.Key, s.Count, s.Sum, s.Min, s.Max, s.Avg, s.Rate, s.P95)
		}
		measure.Reset()
		return c.JSON(200, stats)
	})

	e.Start(":8080")
}

type Report struct {
	ReservationID int64
	EventID       int64
	Rank          string
	Num           int64
	UserID        int64
	SoldAt        string
	CanceledAt    string
	Price         int64
}

func renderReportCSV(c echo.Context, reports []Report) error {
	defer measure.Start(
		"renderReportCSV",
	).
		Stop()

	sort.Slice(reports, func(i, j int) bool {
		defer measure.Start(
			"renderReportCSV-1",
		).Stop()

		return strings.Compare(reports[i].SoldAt, reports[j].SoldAt) < 0
	})

	body := bytes.NewBufferString("reservation_id,event_id,rank,num,price,user_id,sold_at,canceled_at\n")
	for _, v := range reports {
		body.WriteString(fmt.Sprintf("%d,%d,%s,%d,%d,%d,%s,%s\n",
			v.ReservationID, v.EventID, v.Rank, v.Num, v.Price, v.UserID, v.SoldAt, v.CanceledAt))
	}

	c.Response().Header().Set("Content-Type", `text/csv; charset=UTF-8`)
	c.Response().Header().Set("Content-Disposition", `attachment; filename="report.csv"`)
	_, err := io.Copy(c.Response(), body)
	return err
}

func resError(c echo.Context, e string, status int) error {
	defer measure.Start(
		"resError",
	).Stop()

	if e == "" {
		e = "unknown"
	}
	if status < 100 {
		status = 500
	}
	return c.JSON(status, map[string]string{"error": e})
}

func getSheetsMaster() ([]Sheet, error) {
	sheetsData, err := db.Query("SELECT * FROM sheets")
	if err != nil {
		return nil, err
	}
	var sheets []Sheet
	for sheetsData.Next() {
		var sheet Sheet
		if err := sheetsData.Scan(&sheet.ID, &sheet.Rank, &sheet.Num, &sheet.Price); err != nil {
			return nil, err
		}
		sheets = append(sheets, sheet)
	}
	return sheets, nil
}

func getSheetsInfo(sheet_id int64) Sheet {
	var sheet Sheet
	for _, s := range SheetsMaster {
		if s.ID == sheet_id {
			sheet = s
			break
		}
	}

	return sheet
}
