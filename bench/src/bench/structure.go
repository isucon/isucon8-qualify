package bench

import (
	"log"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/LK4D4/trylock"
)

// {"nickname":"sonots","id":1001};
type JsonUser struct {
	ID       uint   `json:"id"`
	Nickname string `json:"nickname"`
}

// {"nickname":"sonots","id":1001};
type JsonFullUser struct {
	JsonUser

	TotalPrice         uint                   `json:"total_price"`
	RecentEvents       []*JsonFullEvent       `json:"recent_events"`
	RecentReservations []*JsonFullReservation `json:"recent_reservations"`
}

type JsonAdministrator struct {
	ID       uint   `json:"id"`
	Nickname string `json:"nickname"`
}

// [{"remains":999,"id":1,"title":"「風邪をひいたなう」しか","sheets":{"S":{"price":8000,"total":50,"remains":49},"A":{"total":150,"price":6000,"remains":150},"C":{"remains":0,"total":0},"c":{"remains":500,"price":3000,"total":500},"B":{"total":300,"price":4000,"remains":300}},"total":1000}];

type JsonSheet struct {
	Price   uint              `json:"price"`
	Total   uint              `json:"total"`
	Remains uint              `json:"remains"`
	Details []JsonSheetDetail `json:"detail"`
}

type JsonSheetDetail struct {
	Num        uint `json:"num"`
	Mine       bool `json:"mine"`
	Reserved   bool `json:"reserved"`
	ReservedAt uint `json:"reserved_at"`
}

type JsonEvent struct {
	ID      uint                 `json:"id"`
	Title   string               `json:"title"`
	Total   uint                 `json:"total"`
	Remains uint                 `json:"remains"`
	Sheets  map[string]JsonSheet `json:"sheets"`
}

type JsonFullEvent struct {
	JsonEvent

	Price  uint `json:"price"`
	Public bool `json:"public"`
	Closed bool `json:"closed"`
}

type JsonReservation struct {
	ReservationID uint   `json:"id"`
	SheetRank     string `json:"sheet_rank"`
	SheetNum      uint   `json:"sheet_num"`
}

type JsonFullReservation struct {
	JsonReservation

	Event      *JsonEventInFullReservation `json:"event"`
	Price      uint                        `json:"price"`
	ReservedAt uint                        `json:"reserved_at"`
	CanceledAt uint                        `json:"canceled_at"`
}

type JsonEventInFullReservation struct {
	ID     uint   `json:"id"`
	Title  string `json:"title"`
	Public bool   `json:"public"`
	Closed bool   `json:"closed"`
}

type JsonError struct {
	Error string `json:"error"`
}

type AppUser struct {
	ID        uint
	Nickname  string
	LoginName string
	Password  string

	Status AppUserStatus
}

type AppUserStatus struct {
	Online bool

	PositiveTotalPrice uint
	NegativeTotalPrice uint

	LastReservedEvent      idWithUpdatedAt
	LastMaybeReservedEvent idWithUpdatedAt
	LastReservation        idWithUpdatedAt
	LastMaybeReservation   idWithUpdatedAt
}

func (s *AppUserStatus) TotalPriceString() string {
	if s.PositiveTotalPrice == s.NegativeTotalPrice {
		return strconv.FormatUint(uint64(s.NegativeTotalPrice), 10)
	}

	return strconv.FormatUint(uint64(s.PositiveTotalPrice), 10) + "-" + strconv.FormatUint(uint64(s.NegativeTotalPrice), 10)
}

type idWithUpdatedAt struct {
	id        uint
	updatedAt time.Time
}

func (i *idWithUpdatedAt) SetID(id uint) {
	i.id = id
	i.updatedAt = time.Now()
}

func (i *idWithUpdatedAt) SetIDWithTime(id uint, t time.Time) {
	i.id = id
	i.updatedAt = t
}

func (i *idWithUpdatedAt) GetID(timeBefore time.Time) uint {
	if i.updatedAt.IsZero() {
		return 0
	}
	if i.updatedAt.After(timeBefore) {
		return 0
	}

	return i.id
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
	ClosedFg  bool
	Price     uint
	CreatedAt time.Time

	reservationMtx        sync.RWMutex
	ReserveRequestedCount uint
	ReserveCompletedCount uint
	CancelRequestedCount  uint
	CancelCompletedCount  uint
	ReserveRequestedRT    ReservationTickets
	ReserveCompletedRT    ReservationTickets
	CancelRequestedRT     ReservationTickets
	CancelCompletedRT     ReservationTickets
}

type ReservationTickets struct {
	S, A, B, C uint
}

// Returns an optimistic sold-out prediction, I mean that,
// returns true even if a few sheets are actually remained when some timeout occurs.
func (e *Event) IsSoldOut() bool {
	return int32(e.ReserveRequestedCount)-int32(e.CancelCompletedCount) >= int32(DataSet.SheetTotal)
}

func (rt *ReservationTickets) getPointer(rank string) *uint {
	switch rank {
	case "S":
		return &rt.S
	case "A":
		return &rt.A
	case "B":
		return &rt.B
	case "C":
		return &rt.C
	}
	assert(false)
	return nil
}

func (rt *ReservationTickets) Get(rank string) uint {
	return *rt.getPointer(rank)
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

type ReportRecord struct {
	ReservationID uint
	EventID       uint
	SheetRank     string
	SheetNum      uint
	SheetPrice    uint
	UserID        uint
	CanceledAt    time.Time
}

type Reservation struct {
	ID         uint
	EventID    uint
	UserID     uint
	SheetID    uint // Used only in initial reservations. 0 is set for rest because reserve API does not return it
	SheetRank  string
	SheetNum   uint
	Price      uint
	ReservedAt int64 // Used only in initial reservations. 0 is set for rest because reserve API does not return it
	CanceledAt int64 // Used only in initial reservations. 0 is set for rest because reserve API does not return it

	// ReserveRequestedAt time.Time
	ReserveCompletedAt time.Time
	cancelMtx          trylock.Mutex
	CancelRequestedAt  time.Time
	CancelCompletedAt  time.Time
}

func (r Reservation) CancelMtx() trylock.Mutex {
	return r.cancelMtx
}

func (r Reservation) Canceled(timeBefore time.Time) bool {
	return r.MaybeCanceled(timeBefore) && !r.CancelCompletedAt.IsZero() && r.CancelCompletedAt.Before(timeBefore)
}

func (r Reservation) MaybeCanceled(timeBefore time.Time) bool {
	return !r.CancelRequestedAt.IsZero() && r.CancelRequestedAt.Before(timeBefore)
}

func (r Reservation) LastUpdatedAt() time.Time {
	if !r.CancelCompletedAt.IsZero() {
		return r.CancelCompletedAt
	}
	return r.ReserveCompletedAt
}

func (r Reservation) LastMaybeUpdatedAt() time.Time {
	if !r.CancelCompletedAt.IsZero() {
		return r.CancelCompletedAt
	}
	if !r.CancelRequestedAt.IsZero() {
		return r.CancelRequestedAt
	}
	return r.ReserveCompletedAt
}

type BenchDataSet struct {
	Users    []*AppUser
	NewUsers []*AppUser

	Administrators []*Administrator

	Events       []*Event
	ClosedEvents []*Event

	SheetTotal   uint
	SheetKinds   []*SheetKind
	SheetKindMap map[string]*SheetKind
	Sheets       []*Sheet

	Reservations []*Reservation
}

var NonReservedNum = uint(0)

// Represents a sheet within an event
type EventSheet struct {
	EventID uint
	Rank    string
	Num     uint
	Price   uint
}

type State struct {
	mtx                              sync.Mutex
	newEventMtx                      trylock.Mutex
	getRandomPublicSoldOutEventRWMtx sync.RWMutex

	users      []*AppUser
	newUsers   []*AppUser
	userMap    map[string]*AppUser
	checkerMap map[*AppUser]*Checker

	admins          []*Administrator
	adminMap        map[string]*Administrator
	adminCheckerMap map[*Administrator]*Checker

	// NOTE: Do not pop any events from events
	// NOTE: Do not push any events if CreateEvent request fails or timeouts
	events []*Event

	// Pop from eventSheets before reserve request.
	// If reserve succeeds, move into reservedEventSheets.
	// If reserve fails, throw it away instead of pushing back.
	//
	// In the case of cancel,
	// If cancel succeeds, move an event of reservedEventSheets into eventSheets.
	// If cancel fails, keep it in reservedEventSheets.
	// (Actually, reservedEventSheets is not used from anywhere currently)
	//
	// That is, number of eventSheets coulbe be smaller than the server-side, but never be larger.

	// public && closed does not happen
	eventSheets         []*EventSheet // public && !closed
	privateEventSheets  []*EventSheet // !public && !closed
	closedEventSheets   []*EventSheet // !public && closed
	reservedEventSheets []*EventSheet // flag does not matter, all reserved sheets come here

	reservationMtx        sync.Mutex
	reservations          map[uint]*Reservation // key: reservation id
	reserveRequestedCount uint
	reserveCompletedCount uint
	cancelRequestedCount  uint
	cancelCompletedCount  uint

	// TODO(sonots): Remove later if we've completed without using this anymore.
	//
	// Like a transactional log for reserve/cancel API.
	// A log is removed after we verified that the reserve/cancel API request succeeded.
	// If a request is timeouted or failed by any reasons, the log remains kept.
	reserveLogMtx sync.Mutex
	reserveLogID  uint64                  // 2^64 should be enough
	reserveLog    map[uint64]*Reservation // key: reserveLogID
	cancelLogMtx  sync.Mutex
	cancelLogID   uint64                  // 2^64 should be enough
	cancelLog     map[uint64]*Reservation // key: cancelLogID
}

func (s *State) Init() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.userMap = map[string]*AppUser{}
	s.checkerMap = map[*AppUser]*Checker{}
	for _, u := range DataSet.Users {
		s.pushInitialUserLocked(u)
	}
	s.newUsers = append(s.newUsers, DataSet.NewUsers...)

	s.adminMap = map[string]*Administrator{}
	s.adminCheckerMap = map[*Administrator]*Checker{}
	for _, u := range DataSet.Administrators {
		s.pushInitialAdministratorLocked(u)
	}

	createdAt := time.Time{}
	for _, event := range DataSet.Events {
		s.pushNewEventLocked(event, createdAt, "Init")
	}
	for _, event := range DataSet.ClosedEvents {
		s.pushInitialClosedEventLocked(event, createdAt)
	}

	s.reservations = map[uint]*Reservation{}
	for _, reservation := range DataSet.Reservations {
		s.reservations[reservation.ID] = reservation
	}
	s.reserveRequestedCount = uint(len(s.reservations))
	s.reserveCompletedCount = uint(len(s.reservations))
	// NOTE: Need to init cancel counts if initial data contains cancels.

	s.reserveLogID = 0
	s.reserveLog = map[uint64]*Reservation{}
	s.cancelLogID = 0
	s.cancelLog = map[uint64]*Reservation{}
}

func (s *State) PopRandomUser() (*AppUser, *Checker, func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	n := len(s.users)
	if n == 0 {
		log.Println("debug: Empty users")
		return nil, nil, nil
	}

	i := rand.Intn(n)
	u := s.users[i]

	s.users[i] = s.users[n-1]
	s.users[n-1] = nil
	s.users = s.users[:n-1]

	log.Printf("debug: PopRandomUser %d %s %s\n", u.ID, u.LoginName, u.Nickname)
	return u, s.getCheckerLocked(u), func() { s.PushUser(u) }
}

func (s *State) PopUserByID(userID uint) (*AppUser, *Checker, func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	n := len(s.users)
	if n == 0 {
		log.Println("debug: Empty users")
		return nil, nil, nil
	}

	var i int
	var u *AppUser
	for i, u = range s.users {
		if u.ID == userID {
			break
		}
	}

	if u == nil {
		log.Printf("debug: User ID:%d not found\n", userID)
		return nil, nil, nil
	}

	s.users[i] = s.users[n-1]
	s.users[n-1] = nil
	s.users = s.users[:n-1]

	log.Printf("debug: PopUserByID %d %s %s\n", u.ID, u.LoginName, u.Nickname)
	return u, s.getCheckerLocked(u), func() { s.PushUser(u) }
}

func (s *State) PushUser(u *AppUser) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	log.Printf("debug: PushUser %d %s %s\n", u.ID, u.LoginName, u.Nickname)
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
	log.Printf("debug: newUserPush %d %s %s\n", u.ID, u.LoginName, u.Nickname)
	s.userMap[u.LoginName] = u
	s.users = append(s.users, u)
}

func (s *State) pushInitialUserLocked(u *AppUser) {
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
		log.Println("debug: Empty admins")
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

func (s *State) pushInitialAdministratorLocked(u *Administrator) {
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

func (s *State) CreateNewEvent() (*Event, func(caller string)) {
	event := &Event{
		ID:       0, // auto increment
		Title:    RandomAlphabetString(32),
		PublicFg: true,
		ClosedFg: false,
		Price:    1000 + uint(rand.Intn(10)*1000),
	}

	// NOTE: push() function pushes into s.events, does not push to s.newEvents.
	// You should call push() after you verify that a new event is successfully created on the server.
	return event, func(caller string) { s.PushNewEvent(event, time.Now(), caller) }
}

func (s *State) PushNewEvent(event *Event, createdAt time.Time, caller string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.pushNewEventLocked(event, createdAt, caller)
}

func (s *State) pushNewEventLocked(event *Event, createdAt time.Time, caller string) {
	log.Printf("debug: newEventPush %d %s %d Public:%t Closed:%t (Caller:%s)\n", event.ID, event.Title, event.Price, event.PublicFg, event.ClosedFg, caller)

	event.CreatedAt = createdAt
	s.events = append(s.events, event)

	if event.IsSoldOut() {
		return
	}

	newEventSheets := []*EventSheet{}
	for _, sheetKind := range DataSet.SheetKinds {
		for i := uint(0); i < sheetKind.Total; i++ {
			eventSheet := &EventSheet{
				EventID: event.ID,
				Rank:    sheetKind.Rank,
				Num:     NonReservedNum,
				Price:   event.Price + sheetKind.Price,
			}
			newEventSheets = append(newEventSheets, eventSheet)
		}
	}
	// NOTE: Push new events to front so that PopEventSheet pops a sheet from older ones.
	if event.ClosedFg {
		s.closedEventSheets = append(newEventSheets, s.closedEventSheets...)
	} else if !event.PublicFg {
		s.privateEventSheets = append(newEventSheets, s.privateEventSheets...)
	} else {
		s.eventSheets = append(newEventSheets, s.eventSheets...)
	}
}

// Initial closed events are all reserved and closed
func (s *State) pushInitialClosedEventLocked(event *Event, createdAt time.Time) {
	event.CreatedAt = createdAt
	s.events = append(s.events, event)

	for _, sheetKind := range DataSet.SheetKinds {
		for i := uint(0); i < sheetKind.Total; i++ {
			eventSheet := &EventSheet{event.ID, sheetKind.Rank, i + 1, event.Price + sheetKind.Price}
			s.reservedEventSheets = append(s.reservedEventSheets, eventSheet)
		}
	}
}

func (s *State) FindEventByID(id uint) *Event {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, event := range s.events {
		if event.ID == id {
			return event
		}
	}
	return nil
}

// Returns a shallow copy of s.events
func (s *State) GetEvents() (events []*Event) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	events = make([]*Event, len(s.events))
	copy(events, s.events)
	return
}

// Returns s.events[n]
func (s *State) GetEventByID(eventID uint) *Event {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, e := range s.events {
		if e.ID == eventID {
			return e
		}
	}
	return nil
}

// Returns a deep copy of s.events
func (s *State) GetCopiedEvents() (events []*Event) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	events = make([]*Event, 0, len(s.events))
	for _, e := range s.events {
		event := *e // copy
		events = append(events, &event)
	}

	return
}

func CopyEvent(src *Event) *Event {
	if src == nil {
		return nil
	}

	dest := *src // copy
	return &dest
}

func FilterEventsToAllowDelay(src []*Event, timeBefore time.Time) (filtered []*Event) {
	filtered = make([]*Event, 0, len(src))
	for _, e := range src {
		if e.CreatedAt.Before(timeBefore) {
			filtered = append(filtered, e)
		}
	}
	return
}

func FilterPublicEvents(src []*Event) (filtered []*Event) {
	filtered = make([]*Event, 0, len(src))
	for _, e := range src {
		if !e.PublicFg {
			continue
		}
		assert(!e.ClosedFg)
		filtered = append(filtered, e)
	}
	return
}

func FilterSoldOutEvents(src []*Event) (filtered []*Event) {
	filtered = make([]*Event, 0, len(src))
	for _, e := range src {
		if e.IsSoldOut() {
			filtered = append(filtered, e)
		}
	}
	return
}

func (s *State) GetRandomPublicEvent() *Event {
	events := FilterPublicEvents(s.GetEvents())
	if len(events) == 0 {
		return nil
	}
	return events[uint(rand.Intn(len(events)))]
}

func (s *State) GetRandomPublicSoldOutEvent() *Event {
	events := FilterPublicEvents(FilterSoldOutEvents(s.GetEvents()))
	if len(events) == 0 {
		return nil
	}
	return events[uint(rand.Intn(len(events)))]
}

func (s *State) PopEventSheet() (*EventSheet, func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	n := len(s.eventSheets)
	if n == 0 {
		log.Println("debug: Empty eventSheets, will create a new event.")
		return nil, nil
	}

	es := s.eventSheets[n-1]
	s.eventSheets = s.eventSheets[:n-1]

	return es, func() { s.PushEventSheet(es) }
}

func (s *State) PushEventSheet(eventSheet *EventSheet) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if eventSheet.Num == NonReservedNum {
		s.eventSheets = append(s.eventSheets, eventSheet)
	} else {
		s.reservedEventSheets = append(s.reservedEventSheets, eventSheet)
	}
}

func GetRandomSheetRank() string {
	return DataSet.SheetKinds[rand.Intn(len(DataSet.SheetKinds))].Rank
}

func GetSheetKindByRank(rank string) *SheetKind {
	for _, sheetKind := range DataSet.SheetKinds {
		if sheetKind.Rank == rank {
			return sheetKind
		}
	}

	return nil
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

func (s *State) FindReservationByID(reservationID uint) *Reservation {
	s.reservationMtx.Lock()
	defer s.reservationMtx.Unlock()

	reservation := s.reservations[reservationID]

	return reservation
}

// Returns a shallow copy of s.reservations
func (s *State) GetReservations() map[uint]*Reservation {
	s.reservationMtx.Lock()
	defer s.reservationMtx.Unlock()

	reservations := make(map[uint]*Reservation, len(s.reservations))
	for id, reservation := range s.reservations {
		reservations[id] = reservation
	}

	return reservations
}

// Returns a deep copy of s.reservations
// NOTE: This could be slow if s.reservations are large, but we assume that
// len(s.reservations) are less than 10,000 even in very fast webapp implementation.
func (s *State) GetCopiedReservations() map[uint]*Reservation {
	s.reservationMtx.Lock()
	defer s.reservationMtx.Unlock()

	t := time.Now()

	reservations := make(map[uint]*Reservation, len(s.reservations))
	for id, r := range s.reservations {
		reservation := *r // copy
		reservations[id] = &reservation
	}

	log.Println("debug: GetCopiedReservations", time.Since(t))

	return reservations
}

// Returns a filtered shallow copy
func (s *State) GetReservationsInEventID(eventID uint) map[uint]*Reservation {
	s.reservationMtx.Lock()
	defer s.reservationMtx.Unlock()

	filtered := make(map[uint]*Reservation, len(s.reservations))
	for id, reservation := range s.reservations {
		if reservation.EventID != eventID {
			continue
		}
		filtered[id] = reservation
	}
	return filtered
}

// Returns a filtered deep copy
func (s *State) GetCopiedReservationsInEventID(eventID uint) map[uint]*Reservation {
	s.reservationMtx.Lock()
	defer s.reservationMtx.Unlock()

	filtered := make(map[uint]*Reservation, len(s.reservations))
	for id, r := range s.reservations {
		if r.EventID != eventID {
			continue
		}
		reservation := *r // copy
		filtered[id] = &reservation
	}
	return filtered
}

func FilterReservationsToAllowDelay(src map[uint]*Reservation, timeBefore time.Time) (filtered map[uint]*Reservation) {
	filtered = make(map[uint]*Reservation, len(src))

	for id, reservation := range src {
		if reservation.ReserveCompletedAt.Before(timeBefore) {
			filtered[id] = reservation
		}
	}
	return
}

func FilterReservationsByUserID(src map[uint]*Reservation, userID uint) (filtered map[uint]*Reservation) {
	filtered = make(map[uint]*Reservation, len(src))

	for id, reservation := range src {
		if reservation.UserID == userID {
			filtered[id] = reservation
		}
	}
	return
}

func (s *State) GetRandomNonCanceledReservationInEventID(eventID uint) *Reservation {
	reservations := s.GetReservationsInEventID(eventID)

	filtered := make([]*Reservation, 0, len(reservations))
	for _, reservation := range reservations {
		if reservation.CancelRequestedAt.IsZero() && reservation.CancelCompletedAt.IsZero() {
			filtered = append(filtered, reservation)
		}
	}

	i := rand.Intn(len(filtered))
	return filtered[i]
}

func (s *State) GetReserveRequestedCount() uint {
	s.reserveLogMtx.Lock()
	defer s.reserveLogMtx.Unlock()

	return s.reserveRequestedCount
}

func (e *Event) GetReserveRequestedCount() uint {
	e.reservationMtx.Lock()
	defer e.reservationMtx.Unlock()

	return e.ReserveRequestedCount
}

func (s *State) BeginReservation(lockedUser *AppUser, reservation *Reservation) (logID uint64) {
	func() {
		s.reservationMtx.Lock()
		defer s.reservationMtx.Unlock()

		s.reserveRequestedCount++
	}()
	func() {
		event := s.FindEventByID(reservation.EventID)
		rank := reservation.SheetRank

		event.reservationMtx.Lock()
		defer event.reservationMtx.Unlock()

		event.ReserveRequestedCount++
		*event.ReserveRequestedRT.getPointer(rank)++
	}()
	{
		lockedUser.Status.PositiveTotalPrice += reservation.Price
		lockedUser.Status.LastMaybeReservedEvent.SetID(reservation.EventID)
	}
	logID = s.appendReserveLog(reservation)
	return
}

func (s *State) CommitReservation(logID uint64, lockedUser *AppUser, reservation *Reservation) error {
	err := func() error {
		s.reservationMtx.Lock()
		defer s.reservationMtx.Unlock()

		if _, ok := s.reservations[reservation.ID]; ok {
			return fatalErrorf("予約IDが重複しています")
		}

		reservation.ReserveCompletedAt = time.Now()
		s.reservations[reservation.ID] = reservation
		s.reserveCompletedCount++
		assert(uint(len(s.reservations)) == s.reserveCompletedCount)
		return nil
	}()
	if err != nil {
		return err
	}
	func() {
		event := s.FindEventByID(reservation.EventID)
		rank := reservation.SheetRank

		event.reservationMtx.Lock()
		defer event.reservationMtx.Unlock()

		event.ReserveCompletedCount++
		*event.ReserveCompletedRT.getPointer(rank)++
	}()
	{
		lockedUser.Status.NegativeTotalPrice += reservation.Price
		lockedUser.Status.LastReservedEvent.SetID(reservation.EventID)
		lockedUser.Status.LastReservation.SetID(reservation.ID)
	}
	s.deleteReserveLog(logID, reservation)
	return nil
}

func (s *State) BeginCancelation(lockedUser *AppUser, reservation *Reservation) (logID uint64) {
	func() {
		s.reservationMtx.Lock()
		defer s.reservationMtx.Unlock()

		reservation.CancelRequestedAt = time.Now()
		s.reservations[reservation.ID] = reservation
		s.cancelRequestedCount++
	}()
	func() {
		event := s.FindEventByID(reservation.EventID)
		rank := reservation.SheetRank

		event.reservationMtx.Lock()
		defer event.reservationMtx.Unlock()

		event.CancelRequestedCount++
		*event.CancelRequestedRT.getPointer(rank)++
	}()
	{
		lockedUser.Status.NegativeTotalPrice -= reservation.Price
		lockedUser.Status.LastMaybeReservedEvent.SetID(reservation.EventID)
		lockedUser.Status.LastMaybeReservation.SetID(reservation.ID)
	}
	logID = s.appendReserveLog(reservation)
	return
}

func (s *State) CommitCancelation(logID uint64, lockedUser *AppUser, reservation *Reservation) {
	func() {
		s.reservationMtx.Lock()
		defer s.reservationMtx.Unlock()

		reservation.CancelCompletedAt = time.Now()
		s.reservations[reservation.ID] = reservation
		s.cancelCompletedCount++
	}()
	func() {
		event := s.FindEventByID(reservation.EventID)
		rank := reservation.SheetRank

		event.reservationMtx.Lock()
		defer event.reservationMtx.Unlock()

		event.CancelCompletedCount++
		*event.CancelCompletedRT.getPointer(rank)++
	}()
	{
		lockedUser.Status.PositiveTotalPrice -= reservation.Price
		lockedUser.Status.LastReservedEvent.SetID(reservation.EventID)
		lockedUser.Status.LastReservation.SetID(reservation.ID)
	}
	s.deleteCancelLog(logID, reservation)
	return
}

func (s *State) appendReserveLog(reservation *Reservation) uint64 {
	s.reserveLogMtx.Lock()
	defer s.reserveLogMtx.Unlock()

	s.reserveLogID++
	s.reserveLog[s.reserveLogID] = reservation

	log.Printf("debug: appendReserveLog LogID:%2d EventID:%2d UserID:%3d SheetRank:%s\n", s.reserveLogID, reservation.EventID, reservation.UserID, reservation.SheetRank)
	return s.reserveLogID
}

func (s *State) deleteReserveLog(reserveLogID uint64, reservation *Reservation) {
	s.reserveLogMtx.Lock()
	defer s.reserveLogMtx.Unlock()

	log.Printf("debug: deleteReserveLog LogID:%2d EventID:%2d UserID:%3d SheetRank:%s SheetNum:%d ReservationID:%d (Reserved)\n", reserveLogID, reservation.EventID, reservation.UserID, reservation.SheetRank, reservation.SheetNum, reservation.ID)
	delete(s.reserveLog, reserveLogID)
}

func (s *State) appendCancelLog(reservation *Reservation) uint64 {
	s.cancelLogMtx.Lock()
	defer s.cancelLogMtx.Unlock()

	s.cancelLogID++
	s.cancelLog[s.cancelLogID] = reservation

	log.Printf("debug: appendCancelLog  LogID:%2d EventID:%2d UserID:%3d SheetRank:%s SheetNum:%d ReservationID:%d\n", s.cancelLogID, reservation.EventID, reservation.UserID, reservation.SheetRank, reservation.SheetNum, reservation.ID)
	return s.cancelLogID
}

func (s *State) deleteCancelLog(cancelLogID uint64, reservation *Reservation) {
	s.cancelLogMtx.Lock()
	defer s.cancelLogMtx.Unlock()

	log.Printf("debug: deleteCancelLog  LogID:%2d EventID:%2d UserID:%3d SheetRank:%s SheetNum:%d ReservationID:%d (Canceled)\n", s.cancelLogID, reservation.EventID, reservation.UserID, reservation.SheetRank, reservation.SheetNum, reservation.ID)
	delete(s.cancelLog, cancelLogID)
}
