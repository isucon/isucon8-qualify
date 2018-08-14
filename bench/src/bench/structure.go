package bench

import (
	"math/rand"
	"sync"
	"time"
)

// {"nickname":"sonots","id":1001};
type JsonUser struct {
	ID       uint   `json:"id"`
	Nickname string `json:"nickname"`
}

type JsonAdministrator struct {
	ID       uint   `json:"id"`
	Nickname string `json:"nickname"`
}

// [{"remains":999,"id":1,"title":"「風邪をひいたなう」しか","sheets":{"S":{"price":8000,"total":50,"remains":49},"A":{"total":150,"price":6000,"remains":150},"C":{"remains":0,"total":0},"c":{"remains":500,"price":3000,"total":500},"B":{"total":300,"price":4000,"remains":300}},"total":1000}];

type JsonSheet struct {
	Price   uint `json:"price"`
	Total   uint `json:"total"`
	Remains uint `json:"remains"`
}

type JsonEvent struct {
	ID      uint                 `json:"id"`
	Title   string               `json:"title"`
	Total   uint                 `json:"total"`
	Remains uint                 `json:"remains"`
	Sheets  map[string]JsonSheet `json:"sheets"`
}

type JsonAdminEvent struct {
	ID      uint                 `json:"id"`
	Title   string               `json:"title"`
	Public  bool                 `json:"public"`
	Price   uint                 `json:"price"`
	Remains uint                 `json:"remains"`
	Sheets  map[string]JsonSheet `json:"sheets"`
}

type JsonReserved struct {
	ReservationID uint   `json:"reservation_id"`
	SheetRank     string `json:"sheet_rank"`
	SheetNum      uint   `json:"sheet_num"`
}

type JsonError struct {
	Error string `json:"error"`
}

type AppUser struct {
	ID        uint
	Nickname  string
	LoginName string
	Password  string

	Status struct {
		Online bool
	}
}

type Administrator struct {
	ID        uint
	Nickname  string
	LoginName string
	Password  string

	Status struct {
		Online bool
	}
}

type Event struct {
	ID        uint
	Title     string
	PublicFg  bool
	Price     uint
	CreatedAt time.Time
}

type SheetKind struct {
	Rank  string
	Total uint
	Price uint
}

type Sheet struct {
	ID    uint
	Rank  string
	Num   uint // ID within a rank
	Price uint
}

type Reservation struct {
	ID        uint
	EventID   uint
	UserID    uint
	SheetRank string
	SheetNum  uint
	// ReservedAt uint // No way to obtain now
}

// Like a transactional log for reserve API.
// A log is removed after we verified that the reserve API request succeeded.
// If a request is timeouted or failed by any reasons, the log remains kept.
type ReserveLog struct {
	EventID   uint
	UserID    uint
	SheetRank string
}

// cancel API version of ReserveLog
type CancelLog struct {
	EventID       uint
	UserID        uint
	SheetRank     string
	ReservationID uint
}

type BenchDataSet struct {
	Users    []*AppUser
	NewUsers []*AppUser

	Administrators []*Administrator

	Events    []*Event
	NewEvents []*Event

	SheetKinds []*SheetKind
	Sheets     []*Sheet
}

// Represents a state of a sheet rank winthin an event
type EventSheetRank struct {
	EventID  uint
	Rank     string
	Total    uint
	Remains  uint
	Reserved map[uint]bool // key: Sheet.Num
}

type State struct {
	mtx sync.Mutex

	users      []*AppUser
	newUsers   []*AppUser
	userMap    map[string]*AppUser
	checkerMap map[*AppUser]*Checker

	admins          []*Administrator
	adminMap        map[string]*Administrator
	adminCheckerMap map[*Administrator]*Checker

	events    []*Event
	newEvents []*Event

	eventSheetRanks        []*EventSheetRank
	privateEventSheetRanks []*EventSheetRank

	reservationsMtx sync.Mutex
	reservations    map[uint]*Reservation // key: reservation id

	reserveLogMtx sync.Mutex
	reserveLogID  uint64                 // 2^64 should be enough
	reserveLog    map[uint64]*ReserveLog // key: reserveLogID
	cancelLogMtx  sync.Mutex
	cancelLogID   uint64                // 2^64 should be enough
	cancelLog     map[uint64]*CancelLog // key: cancelLogID
}

func (s *State) Init() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.userMap = map[string]*AppUser{}
	s.checkerMap = map[*AppUser]*Checker{}
	for _, u := range DataSet.Users {
		s.pushNewUserLocked(u)
	}
	s.newUsers = append(s.newUsers, DataSet.NewUsers...)

	s.adminMap = map[string]*Administrator{}
	s.adminCheckerMap = map[*Administrator]*Checker{}
	for _, u := range DataSet.Administrators {
		s.pushNewAdministratorLocked(u)
	}

	for _, event := range DataSet.Events {
		s.pushNewEventLocked(event)
	}
	s.newEvents = append(s.newEvents, DataSet.NewEvents...)

	s.reservations = map[uint]*Reservation{}

	s.reserveLogID = 0
	s.reserveLog = map[uint64]*ReserveLog{}
	s.cancelLogID = 0
	s.cancelLog = map[uint64]*CancelLog{}
}

func (s *State) PopRandomUser() (*AppUser, *Checker, func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	n := len(s.users)
	if n == 0 {
		return nil, nil, nil
	}

	i := rand.Intn(n)
	u := s.users[i]

	s.users[i] = s.users[n-1]
	s.users[n-1] = nil
	s.users = s.users[:n-1]

	return u, s.getCheckerLocked(u), func() { s.PushUser(u) }
}

func (s *State) PushUser(u *AppUser) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.users = append(s.users, u)
}

func (s *State) PopNewUser() (*AppUser, *Checker, func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.popNewUserLocked()
}

func (s *State) popNewUserLocked() (*AppUser, *Checker, func()) {
	n := len(s.newUsers)
	if n == 0 {
		return nil, nil, nil
	}

	u := s.newUsers[n-1]
	s.newUsers = s.newUsers[:n-1]

	// NOTE: push() functions pushes into s.users, does not push back to s.newUsers.
	// You should call push() after you verify that a new user is successfully created on the server.
	return u, s.getCheckerLocked(u), func() { s.PushNewUser(u) }
}

func (s *State) PushNewUser(u *AppUser) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.pushNewUserLocked(u)
}

func (s *State) pushNewUserLocked(u *AppUser) {
	// fmt.Printf("newUserPush %d %s %s\n", u.ID, u.LoginName, u.Nickname)
	s.userMap[u.LoginName] = u
	s.users = append(s.users, u)
}

func (s *State) GetChecker(u *AppUser) *Checker {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.getCheckerLocked(u)
}

func (s *State) getCheckerLocked(u *AppUser) *Checker {
	checker, ok := s.checkerMap[u]

	if !ok {
		checker = NewChecker()
		checker.debugHeaders["X-User-Login-Name"] = u.LoginName
		s.checkerMap[u] = checker
	}

	return checker
}

func (s *State) PopRandomAdministrator() (*Administrator, *Checker, func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	n := len(s.admins)
	if n == 0 {
		return nil, nil, nil
	}

	i := rand.Intn(n)
	u := s.admins[i]

	s.admins[i] = s.admins[n-1]
	s.admins[n-1] = nil
	s.admins = s.admins[:n-1]

	return u, s.getAdminCheckerLocked(u), func() { s.PushAdministrator(u) }
}

func (s *State) PushAdministrator(u *Administrator) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.admins = append(s.admins, u)
}

func (s *State) PushNewAdministrator(u *Administrator) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.pushNewAdministratorLocked(u)
}

func (s *State) pushNewAdministratorLocked(u *Administrator) {
	s.adminMap[u.LoginName] = u
	s.admins = append(s.admins, u)
}

func (s *State) GetAdminChecker(u *Administrator) *Checker {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.getAdminCheckerLocked(u)
}

func (s *State) getAdminCheckerLocked(u *Administrator) *Checker {
	checker, ok := s.adminCheckerMap[u]

	if !ok {
		checker = NewChecker()
		checker.debugHeaders["X-Admin-Login-Name"] = u.LoginName
		s.adminCheckerMap[u] = checker
	}

	return checker
}

func (s *State) GetEvents() (events []*Event) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	events = make([]*Event, len(s.events))
	copy(events, s.events)
	return
}

func (s *State) PushEvent(event *Event) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.events = append(s.events, event)
}

func (s *State) PopNewEvent() (*Event, func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	n := len(s.newEvents)
	if n == 0 {
		return nil, nil
	}

	event := s.newEvents[n-1]
	s.newEvents = s.newEvents[:n-1]

	// NOTE: push() function pushes into s.events, does not push back to s.newEvents.
	// You should call push() after you verify that a new event is successfully created on the server.
	return event, func() { s.PushNewEvent(event) }
}

func (s *State) PushNewEvent(event *Event) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.pushNewEventLocked(event)
}

func (s *State) pushNewEventLocked(event *Event) {
	event.CreatedAt = time.Now()
	// fmt.Printf("newEventPush %d %s %d %t\n", event.ID, event.Title, event.Price, event.PublicFg)
	s.events = append(s.events, event)

	for _, sheetKind := range DataSet.SheetKinds {
		eventSheetRank := &EventSheetRank{}
		eventSheetRank.EventID = event.ID
		eventSheetRank.Rank = sheetKind.Rank
		eventSheetRank.Total = sheetKind.Total
		eventSheetRank.Remains = sheetKind.Total
		eventSheetRank.Reserved = map[uint]bool{}
		if event.PublicFg {
			s.eventSheetRanks = append(s.eventSheetRanks, eventSheetRank)
		} else {
			s.privateEventSheetRanks = append(s.privateEventSheetRanks, eventSheetRank)
		}
	}
}

func (s *State) GetEventSheetRanksByEventID(eventID uint) []*EventSheetRank {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	eventSheetRanks := make([]*EventSheetRank, 0, len(DataSet.SheetKinds))
	for _, eventSheetRank := range s.eventSheetRanks {
		if eventSheetRank.EventID != eventID {
			continue
		}
		eventSheetRanks = append(eventSheetRanks, eventSheetRank)
		if len(eventSheetRanks) == len(DataSet.SheetKinds) {
			break
		}
	}

	return eventSheetRanks
}

func (s *State) PopRandomEventSheetRank() (*EventSheetRank, func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	n := len(s.eventSheetRanks)
	if n == 0 {
		return nil, nil
	}

	i := rand.Intn(n)
	rs := s.eventSheetRanks[i]

	s.eventSheetRanks[i] = s.eventSheetRanks[n-1]
	s.eventSheetRanks[n-1] = nil
	s.eventSheetRanks = s.eventSheetRanks[:n-1]

	return rs, func() { s.PushEventSheetRank(rs) }
}

func (s *State) PushEventSheetRank(eventSheetRank *EventSheetRank) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.eventSheetRanks = append(s.eventSheetRanks, eventSheetRank)
}

func GetRandomSheetRank() string {
	return DataSet.SheetKinds[rand.Intn(len(DataSet.SheetKinds))].Rank
}

func GetRandomSheetNum(sheetRank string) uint {
	total := uint(0)
	for _, sheetKind := range DataSet.SheetKinds {
		if sheetKind.Rank == sheetRank {
			total = sheetKind.Total
		}
	}
	return uint(rand.Intn(int(total)))
}

func FilterPublicEvents(src []*Event) (filtered []*Event) {
	filtered = make([]*Event, 0, len(src))
	for _, e := range src {
		if !e.PublicFg {
			continue
		}

		filtered = append(filtered, e)
	}
	return
}

func (s *State) AppendReservation(eventID uint, userID uint, reserved *JsonReserved) {
	s.reservationsMtx.Lock()
	defer s.reservationsMtx.Unlock()

	reservation := &Reservation{reserved.ReservationID, eventID, userID, reserved.SheetRank, reserved.SheetNum}
	s.reservations[reserved.ReservationID] = reservation
}

func (s *State) DeleteReservation(reservationID uint) {
	s.reservationsMtx.Lock()
	defer s.reservationsMtx.Unlock()

	delete(s.reservations, reservationID)
}

func (s *State) AppendReserveLog(reserveLog *ReserveLog) uint64 {
	s.reserveLogMtx.Lock()
	defer s.reserveLogMtx.Unlock()

	s.reserveLogID += 1
	s.reserveLog[s.reserveLogID] = reserveLog

	// fmt.Printf("AppendReserveLog LogID:%2d EventID:%2d UserID:%3d SheetRank:%s\n", s.reserveLogID, reserveLog.EventID, reserveLog.UserID, reserveLog.SheetRank)
	return s.reserveLogID
}

func (s *State) DeleteReserveLog(reserveLogID uint64) {
	s.reserveLogMtx.Lock()
	defer s.reserveLogMtx.Unlock()

	// fmt.Printf("DeleteReserveLog LogID:%2d\n", reserveLogID)
	delete(s.reserveLog, reserveLogID)
}

func (s *State) AppendCancelLog(cancelLog *CancelLog) uint64 {
	s.cancelLogMtx.Lock()
	defer s.cancelLogMtx.Unlock()

	s.cancelLogID += 1
	s.cancelLog[s.cancelLogID] = cancelLog

	// fmt.Printf("AppendCancelLog  LogID:%2d EventID:%2d UserID:%3d SheetRank:%s ReservationID:%d\n", s.cancelLogID, cancelLog.EventID, cancelLog.UserID, cancelLog.SheetRank, cancelLog.ReservationID)
	return s.cancelLogID
}

func (s *State) DeleteCancelLog(cancelLogID uint64) {
	s.cancelLogMtx.Lock()
	defer s.cancelLogMtx.Unlock()

	// fmt.Printf("DeleteCancelLog  LogID:%2d\n", cancelLogID)
	delete(s.cancelLog, cancelLogID)
}
