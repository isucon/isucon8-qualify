package parameter

import (
	"time"
)

// Benchmarker tuning parameters
var (
	// NOTE: DO NOT FORGET TO FIX /initialze OF Web.pm TOGETHER
	InitialNumUsers = 5000
	// NumUsers = 5000 // amount of user.tsv
	// NumAdministrators = 100 // amount of admin.tsv
	InitialNumClosedEvents = 5 // # of reservations = # of events * 1000 * (1 + random canceld reservations)

	GetTimeout            = 10 * time.Second
	PostTimeout           = 3 * time.Second
	DeleteTimeout         = 3 * time.Second
	InitializeTimeout     = 10 * time.Second
	SlowThreshold         = 1000 * time.Millisecond
	MaxCheckerRequest     = 6
	PostTestLoginTimeout  = 20 * time.Second // postTest takes time because of remained requests. This value was tuned to pass initial app
	PostTestReportTimeout = 60 * time.Second

	LoadInitialNumGoroutines = 5.0
	LoadLevelUpRatio         = 1.5
	LoadLevelUpInterval      = time.Second
	LoadStartupTotalWait     = float64(100000) // Microsecond
	CheckEventReportInterval = 5 * time.Second
	CheckReportInterval      = 31 * time.Second
	EveryCheckerInterval     = 3 * time.Second
	AllowableDelay           = time.Second
	WaitOnError              = 500 * time.Millisecond

	Score = func(getCount int64, postCount int64, deleteCount int64, staticCount int64, reserveCount int64, cancelCount int64, topCount int64, getEventCount int64) int64 {
		return 1*(getCount-staticCount-topCount-getEventCount) + 1*(postCount-reserveCount) + 5*(topCount+getEventCount) + 10*(reserveCount+cancelCount) + staticCount/100
	}
)

// Others:
// Tune number of CPUs and amount of memory on servers which benchmarker runs
