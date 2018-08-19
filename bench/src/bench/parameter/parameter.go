package parameter

import (
	"time"
)

// Benchmarker tuning parameters
var (
	// NOTE: DO NOT FORGET TO FIX /initialze OF Web.pm TOGETHER
	InitialNumUsers = 1000
	// NumUsers = 5000 // amount of user.tsv
	// NumAdministrators = 100 // amount of admin.tsv
	InitialNumClosedEvents = 0 // # of reservations = # of events * 1000

	GetTimeout        = 10 * time.Second
	PostTimeout       = 3 * time.Second
	InitializeTimeout = 10 * time.Second
	SlowThreshold     = 1000 * time.Millisecond
	MaxCheckerRequest = 6
	// TODO(sonots):  Current initial app takes 13 sec to login at postTest on my env. Tune somehow.
	PostTestLoginTimeout  = 20 * time.Second
	PostTestReportTimeout = 60 * time.Second

	LoadInitialNumGoroutines  = 5.0
	LoadLevelUpRatio          = 1.5
	LoadLevelUpInterval       = time.Second
	CheckReportTickerInterval = 5 * time.Second
	AllowableDelay            = time.Second
	WaitOnError               = 500 * time.Millisecond

	Score = func(getCount int64, postCount int64, deleteCount int64, s304Count int64, reserveCount int64, cancelCount int64, topCount int64) int64 {
		return 1*(getCount-s304Count-topCount) + 1*(postCount-reserveCount) + 3*(topCount+reserveCount+cancelCount) + s304Count/100
	}

	// TODO(sonots): parameters of workermode.go
)

// Others:
// Tune number of CPUs and amount of memory on servers which benchmarker runs
