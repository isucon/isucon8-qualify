package bench

import (
	"math/rand"
	"sync"
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
	SheetRank string `json:"sheet_rank"`
	SheetNum  uint   `json:"sheet_num"`
}

type JsonError struct {
	Error string `json:"error"`
}

type AppUser struct {
	ID        uint
	Nickname  string
	LoginName string
	Password  string
}

type Administrator struct {
	ID        uint
	Nickname  string
	LoginName string
	Password  string
}

type Event struct {
	ID       uint
	Title    string
	PublicFg bool
	Price    uint
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
	EventID    uint
	SheetID    uint
	UserID     uint
	ReservedAt uint
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
type SheetRankState struct {
	EventID  uint
	Rank     string
	Total    uint
	Remains  uint
	Reserved map[uint]bool // key: Sheet.Num
}

type State struct {
	mtx             sync.Mutex
	users           []*AppUser
	newUsers        []*AppUser
	userMap         map[string]*AppUser
	checkerMap      map[*AppUser]*Checker
	admins          []*Administrator
	adminMap        map[string]*Administrator
	adminCheckerMap map[*Administrator]*Checker
	events          []*Event
	newEvents       []*Event

	sheetRankStates        []*SheetRankState
	privateSheetRankStates []*SheetRankState
}

func (s *State) Init() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.users = append(s.users, DataSet.Users...)
	s.newUsers = append(s.newUsers, DataSet.NewUsers...)
	s.userMap = map[string]*AppUser{}
	s.checkerMap = map[*AppUser]*Checker{}
	for _, u := range DataSet.Users {
		s.userMap[u.LoginName] = u
	}

	s.admins = append(s.admins, DataSet.Administrators...)
	s.adminMap = map[string]*Administrator{}
	s.adminCheckerMap = map[*Administrator]*Checker{}
	for _, u := range DataSet.Administrators {
		s.adminMap[u.LoginName] = u
	}

	s.events = append(s.events, DataSet.Events...)
	s.newEvents = append(s.newEvents, DataSet.NewEvents...)

	for _, event := range s.events {
		for _, sheetKind := range DataSet.SheetKinds {
			sheetRankState := &SheetRankState{}
			sheetRankState.EventID = event.ID
			sheetRankState.Rank = sheetKind.Rank
			sheetRankState.Total = sheetKind.Total
			sheetRankState.Remains = sheetKind.Total
			sheetRankState.Reserved = map[uint]bool{}
			if event.PublicFg {
				s.sheetRankStates = append(s.sheetRankStates, sheetRankState)
			} else {
				s.privateSheetRankStates = append(s.privateSheetRankStates, sheetRankState)
			}
		}
	}
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

func (s *State) FindUserByName(login_name string) (*AppUser, bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	u, ok := s.userMap[login_name]
	return u, ok
}

func (s *State) popNewUserLocked() (*AppUser, *Checker, func()) {
	n := len(s.newUsers)
	if n == 0 {
		return nil, nil, nil
	}

	u := s.newUsers[n-1]
	s.newUsers = s.newUsers[:n-1]

	// NOTE: push() function pushes into s.users, does not push back to s.newUsers.
	// You should call push() after you verify that a new user is successfully created.
	return u, s.getCheckerLocked(u), func() {
		// fmt.Printf("newUserPush %d %s %s\n", u.ID, u.LoginName, u.Nickname)
		s.PushUser(u)
	}
}

func (s *State) PopNewUser() (*AppUser, *Checker, func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.popNewUserLocked()
}

func (s *State) PushUser(u *AppUser) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

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
		checker.debugHeaders["X-Administrator-Login-Name"] = u.LoginName
		s.adminCheckerMap[u] = checker
	}

	return checker
}

func (s *State) PopRandomEvent() (*Event, func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	n := len(s.events)
	if n == 0 {
		return nil, nil
	}

	i := rand.Intn(n)
	event := s.events[i]

	s.events[i] = s.events[n-1]
	s.events[n-1] = nil
	s.events = s.events[:n-1]

	return event, func() { s.PushEvent(event) }
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
	// You should call push() after you verify that a new event is successfully created.
	return event, func() {
		// fmt.Printf("newEventPush %d %s %d %t\n", event.ID, event.Title, event.Price, event.PublicFg)
		s.PushEvent(event)
	}
}

func (s *State) PopRandomSheetRankState() (*SheetRankState, func()) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	n := len(s.sheetRankStates)
	if n == 0 {
		return nil, nil
	}

	i := rand.Intn(n)
	rs := s.sheetRankStates[i]

	s.sheetRankStates[i] = s.sheetRankStates[n-1]
	s.sheetRankStates[n-1] = nil
	s.sheetRankStates = s.sheetRankStates[:n-1]

	return rs, func() { s.PushSheetRankState(rs) }
}

func (s *State) PushSheetRankState(sheetRankState *SheetRankState) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.sheetRankStates = append(s.sheetRankStates, sheetRankState)
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
