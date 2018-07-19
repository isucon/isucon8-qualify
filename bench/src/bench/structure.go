package bench

import (
	"math/rand"
	"sync"
)

type JsonUser struct {
	ID        int    `json:"id"`
	Nickname  string `json:"nickname"`
	LoginName string `json:"login_name"`
}

type JsonReserve struct {
	SheetRank string `json:"sheet_rank"`
	SheetNum  int    `json:"sheet_num`
}

type BenchDataSet struct {
	Users    []*AppUser
	NewUsers []*AppUser

	Administrators []*Administrator

	Events    []*Event
	NewEvents []*Event
}

type AppUser struct {
	sync.Mutex
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

type Sheet struct {
	ID    uint
	Rank  string
	Num   uint
	Price uint
}

type Reservation struct {
	EventID    uint
	SheetID    uint
	UserID     uint
	ReservedAt uint
}

type Administrator struct {
	Nickname  string
	LoginName string
	Password  string
}

type State struct {
	mtx        sync.Mutex
	users      []*AppUser
	newUsers   []*AppUser
	userMap    map[string]*AppUser
	checkerMap map[*AppUser]*Checker
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
}

// TODO(sonots): Store session and pop user with the session?
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

	return u, s.getCheckerLocked(u), func() { s.PushUser(u) }
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
