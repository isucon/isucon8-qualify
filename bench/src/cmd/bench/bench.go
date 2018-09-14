package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"bench"
	"bench/counter"
	"bench/parameter"

	"github.com/comail/colog"
)

var (
	benchDuration    time.Duration = time.Minute
	preTestOnly      bool
	noLevelup        bool
	checkFuncs       []benchFunc // also preTestFuncs
	everyCheckFuncs  []benchFunc
	loadFuncs        []benchFunc
	loadLevelUpFuncs []benchFunc
	postTestFuncs    []benchFunc
	loadLogs         []string

	pprofPort int = 16060
)

type benchFunc struct {
	Name string
	Func func(ctx context.Context, state *bench.State) error
}

func addCheckFunc(f benchFunc) {
	checkFuncs = append(checkFuncs, f)
}

func addEveryCheckFunc(f benchFunc) {
	everyCheckFuncs = append(everyCheckFuncs, f)
}

func addLoadFunc(weight int, f benchFunc) {
	for i := 0; i < weight; i++ {
		loadFuncs = append(loadFuncs, f)
	}
}

func addLoadAndLevelUpFunc(weight int, f benchFunc) {
	for i := 0; i < weight; i++ {
		loadFuncs = append(loadFuncs, f)
		loadLevelUpFuncs = append(loadLevelUpFuncs, f)
	}
}

func addPostTestFunc(f benchFunc) {
	postTestFuncs = append(postTestFuncs, f)
}

func requestInitialize(targetHost string) error {
	u, _ := url.Parse("/initialize")
	u.Scheme = "http"
	u.Host = targetHost

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", bench.UserAgent)
	req.Host = bench.TorbAppHost

	client := &http.Client{
		Timeout: bench.InitializeTimeout,
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()
	_, err = io.Copy(ioutil.Discard, res.Body)
	if err != nil {
		return err
	}

	if !(200 <= res.StatusCode && res.StatusCode < 300) {
		return fmt.Errorf("Unexpected status code: %d", res.StatusCode)
	}

	return nil
}

// 負荷を掛ける前にアプリが最低限動作しているかをチェックする
// エラーが発生したら負荷をかけずに終了する
func preTest(ctx context.Context, state *bench.State) error {
	funcs := make([]benchFunc, len(checkFuncs)+len(everyCheckFuncs))
	copy(funcs, checkFuncs)
	copy(funcs[len(checkFuncs):], everyCheckFuncs)
	for _, checkFunc := range funcs {
		t := time.Now()
		err := checkFunc.Func(ctx, state)
		log.Println("preTest:", checkFunc.Name, time.Since(t))
		if err != nil {
			return err
		}
	}

	return nil
}

func postTest(ctx context.Context, state *bench.State) error {
	for _, postTestFunc := range postTestFuncs {
		t := time.Now()
		err := postTestFunc.Func(ctx, state)
		log.Println("postTest:", postTestFunc.Name, time.Since(t))
		if err != nil {
			return err
		}
	}

	return nil
}

func checkMain(ctx context.Context, state *bench.State) error {
	// Inserts CheckEventReport and CheckReport on every the specified interval
	checkEventReportTicker := time.NewTicker(parameter.CheckEventReportInterval)
	defer checkEventReportTicker.Stop()
	checkReportTicker := time.NewTicker(parameter.CheckReportInterval)
	defer checkReportTicker.Stop()
	everyCheckerTicker := time.NewTicker(parameter.EveryCheckerInterval)
	defer everyCheckerTicker.Stop()

	randCheckFuncIndices := []int{}
	popRandomPermCheckFunc := func() benchFunc {
		n := len(randCheckFuncIndices)
		if n == 0 {
			randCheckFuncIndices = rand.Perm(len(checkFuncs))
			n = len(randCheckFuncIndices)
		}
		i := randCheckFuncIndices[n-1]
		randCheckFuncIndices = randCheckFuncIndices[:n-1]
		return checkFuncs[i]
	}

	for {
		select {
		case <-checkEventReportTicker.C:
			if ctx.Err() != nil {
				return nil
			}
			t := time.Now()
			err := bench.CheckEventReport(ctx, state)
			log.Println("checkMain(checkEventReport): CheckEventReport", time.Since(t))

			// fatalError以外は見逃してあげる
			if err != nil && bench.IsFatal(err) {
				return err
			}
		case <-checkReportTicker.C:
			if ctx.Err() != nil {
				return nil
			}
			t := time.Now()
			err := bench.CheckReport(ctx, state)
			log.Println("checkMain(checkReport): CheckReport", time.Since(t))

			// fatalError以外は見逃してあげる
			if err != nil && bench.IsFatal(err) {
				return err
			}
		case <-everyCheckerTicker.C:
			for _, checkFunc := range everyCheckFuncs {
				t := time.Now()
				err := checkFunc.Func(ctx, state)
				log.Println("checkMain(every):", checkFunc.Name, time.Since(t))

				// fatalError以外は見逃してあげる
				if err != nil && bench.IsFatal(err) {
					return err
				}

				if err != nil {
					// バリデーションシナリオを悪用してスコアブーストさせないためエラーのときは少し待つ
					time.Sleep(parameter.WaitOnError)
				}
			}
		case <-ctx.Done():
			// benchmarker timeout
			return nil
		default:
			if ctx.Err() != nil {
				return nil
			}

			// Sequentially runs the check functions in randomly permuted order
			checkFunc := popRandomPermCheckFunc()
			t := time.Now()
			err := checkFunc.Func(ctx, state)
			log.Println("checkMain:", checkFunc.Name, time.Since(t))

			// fatalError以外は見逃してあげる
			if err != nil && bench.IsFatal(err) {
				return err
			}

			if err != nil {
				// バリデーションシナリオを悪用してスコアブーストさせないためエラーのときは少し待つ
				time.Sleep(parameter.WaitOnError)
			}
		}
	}
}

func goLoadFuncs(ctx context.Context, state *bench.State, n int) {
	sumWait := (n - 1) * n / 2
	waits := rand.Perm(n)

	var sumDelay time.Duration
	for i := 0; i < n; i++ {
		// add delay not to fire all goroutines at same time
		delay := time.Duration(float64(waits[i])/float64(sumWait)*parameter.LoadStartupTotalWait) * time.Microsecond
		time.Sleep(delay)
		sumDelay += delay

		go func() {
			for {
				if ctx.Err() != nil {
					return
				}

				loadFunc := loadFuncs[rand.Intn(len(loadFuncs))]
				t := time.Now()
				err := loadFunc.Func(ctx, state)
				log.Println("debug: loadFunc:", loadFunc.Name, time.Since(t))

				if err != nil {
					// バリデーションシナリオを悪用してスコアブーストさせないためエラーのときは少し待つ
					time.Sleep(parameter.WaitOnError)
				}

				// no fail
			}
		}()
	}
	log.Println("debug: goLoadLevelUpFuncs wait totally", sumDelay)
}

func goLoadLevelUpFuncs(ctx context.Context, state *bench.State, n int) {
	sumWait := (n - 1) * n / 2
	waits := rand.Perm(n)

	var sumDelay time.Duration
	for i := 0; i < n; i++ {
		// add delay not to fire all goroutines at same time
		delay := time.Duration(float64(waits[i])/float64(sumWait)*parameter.LoadStartupTotalWait) * time.Microsecond
		time.Sleep(delay)
		sumDelay += delay

		go func() {
			for {
				if ctx.Err() != nil {
					return
				}

				loadFunc := loadLevelUpFuncs[rand.Intn(len(loadLevelUpFuncs))]
				t := time.Now()
				err := loadFunc.Func(ctx, state)
				log.Println("debug: levelUpFunc:", loadFunc.Name, time.Since(t))

				if err != nil {
					// バリデーションシナリオを悪用してスコアブーストさせないためエラーのときは少し待つ
					time.Sleep(parameter.WaitOnError)
				}

				// no fail
			}
		}()
	}
	log.Println("debug: goLoadLevelUpFuncs wait totally", sumDelay)
}

func loadMain(ctx context.Context, state *bench.State) {
	levelUpRatio := parameter.LoadLevelUpRatio
	numGoroutines := parameter.LoadInitialNumGoroutines

	goLoadFuncs(ctx, state, int(numGoroutines))

	levelUpTicker := time.NewTicker(parameter.LoadLevelUpInterval)
	defer levelUpTicker.Stop()

	for {
		select {
		case <-levelUpTicker.C:
			log.Printf("debug: loadLevel:%d numGoroutines:%d runtime.NumGoroutines():%d\n", counter.GetKey("load-level-up"), int(numGoroutines), runtime.NumGoroutine())
			if noLevelup {
				continue
			}

			e, et := bench.GetLastCheckerError()
			hasRecentErr := e != nil && time.Since(et) < 5*time.Second

			path, st := bench.GetLastSlowPath()
			hasRecentSlowPath := path != "" && time.Since(st) < 5*time.Second

			now := time.Now().Format("01/02 15:04:05")

			if hasRecentErr {
				loadLogs = append(loadLogs, fmt.Sprintf("%v エラーが発生したため負荷レベルを上げられませんでした。%v", now, e))
				log.Println("Cannot increase Load Level. Reason: RecentErr", e, "Before", time.Since(et))
			} else if hasRecentSlowPath {
				loadLogs = append(loadLogs, fmt.Sprintf("%v レスポンスが遅いため負荷レベルを上げられませんでした。%v", now, path))
				log.Println("Cannot increase Load Level. Reason: SlowPath", path, "Before", time.Since(st))
			} else {
				loadLogs = append(loadLogs, fmt.Sprintf("%v 負荷レベルが上昇しました。", now))
				counter.IncKey("load-level-up")
				nextNumGoroutines := numGoroutines * levelUpRatio
				log.Println("Increase Load Level", counter.GetKey("load-level-up"))
				goLoadLevelUpFuncs(ctx, state, int(nextNumGoroutines-numGoroutines))
				numGoroutines = nextNumGoroutines
			}
		case <-ctx.Done():
			// ベンチ終了、このタイミングでエラーの収集をやめる。
			bench.GuardCheckerError(true)
			return
		}
	}
}

func printCounterSummary() {
	m := map[string]int64{}

	for key, count := range counter.GetMap() {
		if strings.HasPrefix(key, "GET|/api/events/") {
			key = "GET|/api/events/*"
		} else if strings.HasPrefix(key, "POST|/api/events/") {
			key = "POST|/api/events/*/actions/reserve"
		} else if strings.HasPrefix(key, "DELETE|/api/events/") {
			key = "DELETE|/api/events/*/sheets/*/*/reservation"
		} else if strings.HasPrefix(key, "GET|/admin/api/events/") {
			key = "GET|/admin/api/events/*"
		} else if strings.HasPrefix(key, "GET|/api/users/") {
			key = "GET|/api/users/*"
		} else if strings.HasPrefix(key, "POST|/admin/api/events/") {
			key = "POST|/admin/api/events/*/actions/edit"
		} else if strings.HasPrefix(key, "GET|/admin/api/reports/events/") {
			key = "GET|/admin/api/reports/events/*/sales"
		}

		m[key] += count
	}

	type p struct {
		Key   string
		Value int64
	}
	var s []p

	for key, count := range m {
		s = append(s, p{key, count})
	}

	sort.Slice(s, func(i, j int) bool { return s[i].Value > s[j].Value })

	log.Println("----- Request counts -----")
	for _, kv := range s {
		if strings.HasPrefix(kv.Key, "GET|") || strings.HasPrefix(kv.Key, "POST|") || strings.HasPrefix(kv.Key, "DELETE|") {
			log.Println(kv.Key, kv.Value)
		}
	}
	log.Println("----- Other counts ------")
	for _, kv := range s {
		if strings.HasPrefix(kv.Key, "GET|") || strings.HasPrefix(kv.Key, "POST|") || strings.HasPrefix(kv.Key, "DELETE|") {
		} else {
			log.Println(kv.Key, kv.Value)
		}
	}
	log.Println("-------------------------")
}

func startBenchmark(remoteAddrs []string) *BenchResult {
	addLoadFunc(10, benchFunc{"LoadCreateUser", bench.LoadCreateUser})
	addLoadFunc(10, benchFunc{"LoadMyPage", bench.LoadMyPage})
	addLoadFunc(10, benchFunc{"LoadEventReport", bench.LoadEventReport})
	addLoadFunc(10, benchFunc{"LoadAdminTopPage", bench.LoadAdminTopPage})
	addLoadFunc(1, benchFunc{"LoadReport", bench.LoadReport})
	addLoadAndLevelUpFunc(30, benchFunc{"LoadTopPage", bench.LoadTopPage})
	addLoadAndLevelUpFunc(10, benchFunc{"LoadReserveCancelSheet", bench.LoadReserveCancelSheet})
	addLoadAndLevelUpFunc(20, benchFunc{"LoadReserveSheet", bench.LoadReserveSheet})
	addLoadAndLevelUpFunc(30, benchFunc{"LoadGetEvent", bench.LoadGetEvent})

	addCheckFunc(benchFunc{"CheckStaticFiles", bench.CheckStaticFiles})
	addCheckFunc(benchFunc{"CheckCreateUser", bench.CheckCreateUser})
	addCheckFunc(benchFunc{"CheckLogin", bench.CheckLogin})
	addCheckFunc(benchFunc{"CheckTopPage", bench.CheckTopPage})
	addCheckFunc(benchFunc{"CheckAdminTopPage", bench.CheckAdminTopPage})
	addCheckFunc(benchFunc{"CheckReserveSheet", bench.CheckReserveSheet})
	addCheckFunc(benchFunc{"CheckAdminLogin", bench.CheckAdminLogin})
	addCheckFunc(benchFunc{"CheckCreateEvent", bench.CheckCreateEvent})
	addCheckFunc(benchFunc{"CheckMyPage", bench.CheckMyPage})
	addCheckFunc(benchFunc{"CheckCancelReserveSheet", bench.CheckCancelReserveSheet})
	addCheckFunc(benchFunc{"CheckGetEvent", bench.CheckGetEvent})

	addEveryCheckFunc(benchFunc{"CheckSheetReservationEntropy", bench.CheckSheetReservationEntropy})

	addPostTestFunc(benchFunc{"CheckReport", bench.CheckReport})

	result := new(BenchResult)
	result.StartTime = time.Now()
	defer func() {
		result.EndTime = time.Now()
	}()

	getErrorsString := func() []string {
		var errors []string
		for _, err := range bench.GetCheckerErrors() {
			errors = append(errors, err.Error())
		}
		return errors
	}

	state := new(bench.State)

	log.Println("State.Init()")
	state.Init()
	log.Println("State.Init() Done")

	log.Println("requestInitialize()")
	err := requestInitialize(bench.GetRandomTargetHost())
	if err != nil {
		result.Score = 0
		result.Errors = getErrorsString()
		result.Message = fmt.Sprint("/initialize へのリクエストに失敗しました。", err)
		return result
	}
	log.Println("requestInitialize() Done")

	ctx, cancel := context.WithTimeout(context.Background(), benchDuration)
	defer cancel()

	log.Println("preTest()")
	err = preTest(ctx, state)
	if err != nil {
		result.Score = 0
		result.Errors = getErrorsString()
		result.Message = fmt.Sprint("負荷走行前のバリデーションに失敗しました。", err)
		return result
	}
	log.Println("preTest() Done")

	if preTestOnly {
		result.Score = 0
		result.Errors = getErrorsString()
		result.Message = fmt.Sprint("preTest passed.")
		return result
	}

	go loadMain(ctx, state)
	log.Println("checkMain()")
	err = checkMain(ctx, state)
	if err != nil {
		result.Score = 0
		result.Errors = getErrorsString()
		result.Message = fmt.Sprint("負荷走行中のバリデーションに失敗しました。", err)
		return result
	}
	log.Println("checkMain() Done")

	time.Sleep(parameter.AllowableDelay)

	// If backlog, the queue length for completely established sockets waiting to be accepted,
	// are too large or not configured well, postTest may timeout because of the remained requests.
	log.Println("postTest()")
	err = postTest(context.Background(), state)
	if err != nil {
		result.Score = 0
		result.Errors = getErrorsString()
		result.Message = fmt.Sprint("負荷走行後のバリデーションに失敗しました。", err)
		return result
	}
	log.Println("postTest() Done")

	printCounterSummary()

	getEventCount := counter.SumPrefix("GET|/api/events/")
	reserveCount := counter.SumPrefix("POST|/api/events/")
	cancelCount := counter.SumPrefix("DELETE|/api/events/")
	topCount := counter.SumEqual("GET|/")

	getCount := counter.SumPrefix(`GET|/`)
	postCount := counter.SumPrefix(`POST|/`)
	deleteCount := counter.SumPrefix(`DELETE|/`) // == cancelCount
	staticCount := counter.GetKey("staticfile-304") + counter.GetKey("staticfile-200")

	score := parameter.Score(getCount, postCount, deleteCount, staticCount, reserveCount, cancelCount, topCount, getEventCount)

	log.Println("get", getCount)
	log.Println("post", postCount)
	log.Println("delete", deleteCount)
	log.Println("static", staticCount)
	log.Println("top", topCount)
	log.Println("reserve", reserveCount)
	log.Println("cancel", cancelCount)
	log.Println("get_event", getEventCount)
	log.Println("score", score)

	result.LoadLevel = int(counter.GetKey("load-level-up"))
	result.Pass = true
	result.Score = score
	result.Errors = getErrorsString()
	result.Message = "ok"
	return result
}

func main() {
	rand.Seed(time.Now().UnixNano())

	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	log.SetPrefix("[isu8q-bench] ")
	colog.Register()
	colog.SetDefaultLevel(colog.LInfo)
	colog.SetMinLevel(colog.LInfo)

	var (
		workermode bool
		portalUrl  string
		dataPath   string
		remotes    string
		output     string
		jobid      string
		tempdir    string
		test       bool
		debugMode  bool
		debugLog   bool
		nolevelup  bool
		duration   time.Duration
	)

	flag.BoolVar(&workermode, "workermode", false, "workermode")
	flag.StringVar(&portalUrl, "portal", "http://localhost:8888", "portal site url (only used at workermode)")
	flag.StringVar(&dataPath, "data", "./data", "path to data directory")
	flag.StringVar(&remotes, "remotes", "localhost:8080", "remote addrs to benchmark")
	flag.StringVar(&output, "output", "", "path to write result json")
	flag.StringVar(&jobid, "jobid", "", "job id")
	flag.StringVar(&tempdir, "tempdir", "", "path to temp dir")
	flag.BoolVar(&test, "test", false, "run pretest only")
	flag.BoolVar(&debugMode, "debug-mode", false, "add debugging info into request header")
	flag.BoolVar(&debugLog, "debug-log", false, "print debug log")
	flag.DurationVar(&duration, "duration", time.Minute, "benchamrk duration")
	flag.BoolVar(&nolevelup, "nolevelup", false, "dont increase load level")
	flag.Parse()

	if debugLog {
		colog.SetMinLevel(colog.LDebug)
	}
	bench.DebugMode = debugMode
	bench.DataPath = dataPath
	bench.PrepareDataSet()

	preTestOnly = test
	noLevelup = nolevelup
	benchDuration = duration

	if workermode {
		runWorkerMode(tempdir, portalUrl)
		return
	}

	go func() {
		log.Println(http.ListenAndServe(fmt.Sprintf(":%d", pprofPort), nil))
	}()

	remoteAddrs := strings.Split(remotes, ",")
	if 0 == len(remoteAddrs) {
		log.Fatalln("invalid remotes")
	}
	log.Println("Remotes", remoteAddrs)

	bench.SetTargetHosts(remoteAddrs)

	result := startBenchmark(remoteAddrs)
	result.IPAddrs = remotes
	result.JobID = jobid
	result.Logs = loadLogs

	b, err := json.Marshal(result)
	if err != nil {
		log.Fatalln(err)
	}

	log.Println(string(b))

	if output != "" {
		err := ioutil.WriteFile(output, b, 0644)
		if err != nil {
			log.Fatalln(err)
		}
		log.Println("result json saved to ", output)
	}

	if !result.Pass {
		os.Exit(1)
	}
}
