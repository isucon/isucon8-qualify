package bench

import (
	"bench/counter"
	"bench/parameter"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/PuerkitoBio/goquery"
	htmldigest "github.com/karupanerura/go-html-digest"
	"golang.org/x/net/html"
)

func checkHTML(f func(*http.Response, *goquery.Document) error) func(*http.Response, *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		doc, err := goquery.NewDocumentFromReader(body)
		if err != nil {
			return fatalErrorf("ページのHTMLがパースできませんでした")
		}
		return f(res, doc)
	}
}

func checkRedirectStatusCode(res *http.Response, body *bytes.Buffer) error {
	if res.StatusCode == 302 || res.StatusCode == 303 {
		return nil
	}
	return fmt.Errorf("期待していないステータスコード %d Expected 302 or 303", res.StatusCode)
}

func checkJsonErrorResponse(errorCode string) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		bytes := body.Bytes()
		jsonError := JsonError{}
		dec := json.NewDecoder(body)
		err := dec.Decode(&jsonError)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %s %v", string(bytes), err)
		}
		if jsonError.Error != errorCode {
			return fatalErrorf("正しいエラーコードを取得できません %s", jsonError.Error)
		}
		return nil
	}
}

func checkEventList(state *State, eventsBeforeRequest []*Event, events []JsonEvent, eventsAfterResponse []*Event) error {
	eventsMap := map[uint]JsonEvent{}
	for _, e := range events {
		eventsMap[e.ID] = e
	}

	eventsAfterResponseMap := map[uint]*Event{}
	for _, e := range eventsAfterResponse {
		eventsAfterResponseMap[e.ID] = e
	}

	msg := "正しいイベント一覧を取得できません"

	checkRemains := func(
		eventID uint,
		total uint,
		cancelCompletedCountBeforeRequest uint,
		reserveRequestedCountAfterResponse uint,
		remains uint,
		cancelRequestedCountAfterResponse uint,
		reserveCompletedCountBeforeResponse uint) error {
		log.Printf("debug: EventID:%d total:%d+cancelCompletedCountBeforeRequest:%d-reserveRequestedCountAfterResponse:%d <= remains:%d <= total:%d+cancelRequestedCountAfterResponse:%d-reserveCompletedCountBeforeResponse:%d",
			eventID,
			total,
			cancelCompletedCountBeforeRequest,
			reserveRequestedCountAfterResponse,
			remains,
			total,
			cancelRequestedCountAfterResponse,
			reserveCompletedCountBeforeResponse)
		if int32(total)+int32(cancelCompletedCountBeforeRequest)-int32(reserveRequestedCountAfterResponse) <= int32(remains) &&
			int32(remains) <= int32(total)+int32(cancelRequestedCountAfterResponse)-int32(reserveCompletedCountBeforeResponse) {
			return nil
		}
		return &fatalError{}
	}

	for _, eventBeforeRequest := range eventsBeforeRequest {
		e, ok := eventsMap[eventBeforeRequest.ID]
		if !ok {
			log.Printf("debug: checkEventList: should exist (eventID:%d)\n", e.ID)
			return fatalErrorf(msg)
		}
		if e.Title != eventBeforeRequest.Title {
			return fatalErrorf("イベント(id:%d)のタイトルが正しくありません", e.ID)
		}
		if e.Sheets == nil {
			return fatalErrorf("イベント(id:%d)のシート定義が取得できません", e.ID)
		}
		if int(e.Total) != len(DataSet.Sheets) {
			return fatalErrorf("イベント(id:%d)の総座席数が正しくありません", e.ID)
		}
		for _, sheetKind := range DataSet.SheetKinds {
			rank := sheetKind.Rank

			if e.Sheets[rank].Total != sheetKind.Total {
				return fatalErrorf("イベント(id:%d)の%s席の総座席数が正しくありません", e.ID, rank)
			}
			if expected := eventBeforeRequest.Price + sheetKind.Price; e.Sheets[rank].Price != expected {
				return fatalErrorf("イベント(id:%d)の%s席の価格が正しくありません", e.ID, rank)
			}
		}

		eventAfterResponse, ok := eventsAfterResponseMap[e.ID]
		if !ok { // should never happen
			log.Printf("debug: checkEventList: eventAfterResponse did not exist (eventID:%d)\n", e.ID)
			continue
		}

		eventAfterResponse.reservationMtx.RLock()
		err := checkRemains(
			e.ID,
			DataSet.SheetTotal,
			eventBeforeRequest.CancelCompletedCount,
			eventAfterResponse.ReserveRequestedCount,
			e.Remains,
			eventAfterResponse.CancelRequestedCount,
			eventBeforeRequest.ReserveCompletedCount)
		if err != nil {
			err = fatalErrorf("イベント(id:%d)の総残座席数が正しくありません", e.ID)
			goto unlock
		}

		for _, sheetKind := range DataSet.SheetKinds {
			rank := sheetKind.Rank

			err = checkRemains(
				e.ID,
				DataSet.SheetKindMap[rank].Total,
				eventBeforeRequest.CancelCompletedRT.Get(rank),
				eventAfterResponse.ReserveRequestedRT.Get(rank),
				e.Sheets[rank].Remains,
				eventAfterResponse.CancelRequestedRT.Get(rank),
				eventBeforeRequest.ReserveCompletedRT.Get(rank))
			if err != nil {
				log.Printf("warn: eventID=%d remains=%d is not included in count range (%d-%d) \n", e.ID, e.Sheets[rank].Remains, e.Sheets[rank].Total-eventAfterResponse.CancelRequestedRT.Get(rank), e.Sheets[rank].Total-eventBeforeRequest.ReserveCompletedRT.Get(rank))
				err = fatalErrorf("イベント(id:%d)の%s席の残座席数が正しくありません", e.ID, rank)
				break
			}
		}

	unlock:
		eventAfterResponse.reservationMtx.RUnlock()
		if err != nil {
			return err
		}
	}

	return nil
}

func checkJsonFullUserResponse(user *AppUser, check func(*JsonFullUser) error) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		bytes := body.Bytes()
		dec := json.NewDecoder(body)

		var v JsonFullUser
		err := dec.Decode(&v)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %s %v", string(bytes), err)
		}
		if user.ID != v.ID {
			log.Printf("warn: expected id=%d but got id=%d\n", user.ID, v.ID)
			return fatalErrorf("正しいユーザーを取得できません")
		} else if user.Nickname != v.Nickname {
			log.Printf("warn: expected nickname=%s but got nickname=%s (user_id=%d)\n", user.Nickname, v.Nickname, user.ID)
			return fatalErrorf("正しいユーザーを取得できません")
		}

		// basic checks for RecentReservations
		if v.RecentReservations == nil {
			return fatalErrorf("最近予約した席を取得できません")
		}
		if len(v.RecentReservations) > 5 {
			return fatalErrorf("最近予約した席が多すぎます")
		}
		for _, r := range v.RecentReservations {
			if r == nil {
				return fatalErrorf("最近予約した席がnullです")
			}
			if r.Event == nil {
				return fatalErrorf("最近予約した席のイベントがnullです")
			}
		}

		// basic checks for RecentEvents
		if v.RecentEvents == nil {
			return fatalErrorf("最近予約したイベントを取得できません")
		}
		if len(v.RecentEvents) > 5 {
			return fatalErrorf("最近予約したイベントが多すぎます")
		}
		for _, r := range v.RecentEvents {
			if r == nil {
				return fatalErrorf("最近予約したイベントがnullです")
			}
		}

		return check(&v)
	}
}

func loadStaticFile(ctx context.Context, checker *Checker, path string) error {
	return checker.Play(ctx, &CheckAction{
		EnableCache: true,

		Method: "GET",
		Path:   path,
		CheckFunc: func(res *http.Response, body *bytes.Buffer) error {
			// Note. EnableCache時はPlay時に自動でReponseは最後まで読まれる
			if res.StatusCode == http.StatusOK {
				counter.IncKey("staticfile-200")
			} else if res.StatusCode == http.StatusNotModified {
				counter.IncKey("staticfile-304")
			} else {
				return fmt.Errorf("期待していないステータスコード %d", res.StatusCode)
			}
			return nil
		},
	})
}

func goLoadStaticFiles(ctx context.Context, checker *Checker, paths ...string) {
	for _, path := range paths {
		go loadStaticFile(ctx, checker, path)
	}
}

func goLoadAsset(ctx context.Context, checker *Checker) {
	var assetFiles []string
	for _, sf := range StaticFiles {
		assetFiles = append(assetFiles, sf.Path)
	}
	log.Println("debug: goLoadAsset")
	goLoadStaticFiles(ctx, checker, assetFiles...)
}

func LoadCreateUser(ctx context.Context, state *State) error {
	user, checker, newUserPush := state.PopNewUser()
	if user == nil {
		return nil
	}
	checker.ResetCookie()

	err := checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/users",
		ExpectedStatusCode: 201,
		PostJSON: map[string]interface{}{
			"nickname":   user.Nickname,
			"login_name": user.LoginName,
			"password":   user.Password,
		},
		Description: "新規ユーザが作成できること",
		CheckFunc:   checkJsonUserCreateResponse(user),
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/login",
		ExpectedStatusCode: 200,
		PostJSON: map[string]interface{}{
			"login_name": user.LoginName,
			"password":   user.Password,
		},
		Description: "作成したユーザでログインできること",
	})
	if err != nil {
		return err
	}

	user.Status.Online = true
	newUserPush()

	return nil
}

// イベントが公開されるのを待ってトップページをF5連打するユーザがいる
// イベント一覧はログインしていてもしていなくても取れる
func LoadTopPage(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	goLoadAsset(ctx, checker)

	// CheckTopPageでがっつり見る代わりにこっちではチェックを頑張らない
	err := checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/",
		ExpectedStatusCode: 200,
		Description:        "ページが表示されること",
	})
	if err != nil {
		return err
	}

	return nil
}

func LoadAdminTopPage(ctx context.Context, state *State) error {
	admin, checker, push := state.PopRandomAdministrator()
	if admin == nil {
		return nil
	}
	defer push()

	goLoadAsset(ctx, checker)

	err := checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/admin/",
		ExpectedStatusCode: 200,
		Description:        "ページが表示されること",
	})
	if err != nil {
		return err
	}

	return nil
}

func LoadMyPage(ctx context.Context, state *State) error {
	user, userChecker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	err := loginAppUser(ctx, userChecker, user)
	if err != nil {
		return err
	}

	// CheckMyPageでがっつり見る代わりにこっちではチェックを頑張らない
	err = userChecker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/api/users/%d", user.ID),
		ExpectedStatusCode: 200,
		Description:        "ユーザー情報が取得できること",
	})
	if err != nil {
		return err
	}

	return nil
}

// 席は(rank 内で)ランダムに割り当てられるため、良い席に当たるまで予約連打して、キャンセルする悪質ユーザがいる
func LoadReserveCancelSheet(ctx context.Context, state *State) error {
	user, userChecker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	err := loginAppUser(ctx, userChecker, user)
	if err != nil {
		return err
	}

	eventSheet, eventSheetPush, err := popOrCreateEventSheet(ctx, state)
	if err != nil {
		return err
	}
	if eventSheet == nil {
		return nil
	}

	reservation, err := reserveSheet(ctx, state, userChecker, user, eventSheet)
	if reservation == nil && err == nil {
		return nil
	}
	if err != nil {
		return err
	}
	defer eventSheetPush() // NOTE: push only after reserve succeeds

	already_locked, err := cancelSheet(ctx, state, userChecker, user, eventSheet, reservation)
	if err != nil {
		return err
	}
	if already_locked {
		return nil
	}

	return nil
}

func LoadReserveSheet(ctx context.Context, state *State) error {
	user, userChecker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	err := loginAppUser(ctx, userChecker, user)
	if err != nil {
		return err
	}

	eventSheet, eventSheetPush, err := popOrCreateEventSheet(ctx, state)
	if err != nil {
		return err
	}
	if eventSheet == nil {
		return nil
	}

	reservation, err := reserveSheet(ctx, state, userChecker, user, eventSheet)
	if reservation == nil && err == nil {
		return nil
	}
	if err != nil {
		return err
	}
	defer eventSheetPush() // NOTE: push only after reserve succeeds

	return nil
}

// 売り切れたイベントをひたすらF5してキャンセルが出るのを待つユーザがいる
func LoadGetEvent(ctx context.Context, state *State) error {
	// LoadGetEvent() can run concurrently, but CheckCancelReserveSheet() can not
	state.getRandomPublicSoldOutEventRWMtx.RLock()
	defer state.getRandomPublicSoldOutEventRWMtx.RUnlock()

	event := state.GetRandomPublicSoldOutEvent()
	if event == nil {
		log.Printf("warn: LoadGetEvent: no public and sold-out event")
		return nil
	}

	user, checker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	err := loginAppUser(ctx, checker, user)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/api/events/%d", event.ID),
		ExpectedStatusCode: 200,
		Description:        "公開イベントを取得できること",
		CheckFunc:          checkJsonEventResponse(event, nil),
	})
	if err != nil {
		return err
	}

	return nil
}

func CheckGetEvent(ctx context.Context, state *State) error {
	timeBefore := time.Now().Add(-1 * parameter.AllowableDelay)

	user, checker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	var reservation *Reservation
	if rid := user.Status.LastReservation.GetID(timeBefore); rid != 0 {
		reservations := state.GetReservations()
		reservation = reservations[rid]
		if reservation.MaybeCanceled(timeBefore) {
			reservation = nil
		}
	}

	var beforeEvent *Event
	if reservation == nil {
		beforeEvent = CopyEvent(state.GetRandomPublicEvent())
	} else {
		beforeEvent = CopyEvent(state.GetEventByID(reservation.EventID))
		if !beforeEvent.PublicFg {
			beforeEvent = nil
		}
	}
	if beforeEvent == nil {
		return nil
	}

	switch rand.Intn(3) {
	case 0:
		err := loginAppUser(ctx, checker, user)
		if err != nil {
			return err
		}
	case 1:
		err := logoutAppUser(ctx, checker, user)
		if err != nil {
			return err
		}
		// case 2: do nothing
	}

	err := checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/api/events/%d", beforeEvent.ID),
		ExpectedStatusCode: 200,
		Description:        "公開イベントを取得できること",
		CheckFunc: checkJsonEventResponse(beforeEvent, func(event JsonEvent) error {
			afterEvent := state.GetEventByID(beforeEvent.ID)

			err := checkEventList(state, []*Event{beforeEvent}, []JsonEvent{event}, []*Event{afterEvent})
			if err != nil {
				return err
			}

			if reservation == nil {
				return nil
			}

			sheet := event.Sheets[reservation.SheetRank].Details[reservation.SheetNum-1]
			if !sheet.Reserved {
				return fatalErrorf("シート(%s-%d)が予約されていません(id:%d)", reservation.SheetRank, reservation.SheetNum, event.ID)
			}
			if user.Status.Online {
				if !sheet.Mine {
					return fatalErrorf("シート(%s-%d)の保有者がユーザー(id:%d)ではありません(id:%d)", reservation.SheetRank, reservation.SheetNum, user.ID, event.ID)
				}
			} else {
				if sheet.Mine {
					return fatalErrorf("未ログインのユーザーがキャンセルできるシートが存在します(id:%d)", event.ID)
				}
			}

			if sheet.ReservedAt == 0 || !(reservation.ReserveCompletedAt.Unix() == int64(sheet.ReservedAt) || time.Unix(int64(sheet.ReservedAt), 0).Before(reservation.ReserveCompletedAt)) {
				return fatalErrorf("シート(%s-%d)の予約時刻が正しくありません(id:%d)", reservation.SheetRank, reservation.SheetNum, event.ID)
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	return nil
}

func LoadReport(ctx context.Context, state *State) error {
	admin, checker, push := state.PopRandomAdministrator()
	if admin == nil {
		return nil
	}
	defer push()

	err := loginAdministratorWithTimeout(ctx, checker, admin, parameter.PostTestLoginTimeout)
	if err != nil {
		return err
	}

	// We do check at CheckReport
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/admin/api/reports/sales",
		ExpectedStatusCode: 200,
		Description:        "レポートを取得できること",
		Timeout:            parameter.PostTestReportTimeout,
	})
	if err != nil {
		return err
	}

	return nil
}

func LoadEventReport(ctx context.Context, state *State) error {
	admin, checker, push := state.PopRandomAdministrator()
	if admin == nil {
		return nil
	}
	defer push()

	err := loginAdministrator(ctx, checker, admin)
	if err != nil {
		return err
	}

	// We want to let webapp to lock reservations.
	// Since no reserve/cancel occurs for closed events, we ignore closed events.
	event := state.GetRandomPublicEvent()
	if event == nil {
		return nil
	}

	// We do check at CheckEventReport
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/admin/api/reports/events/%d/sales", event.ID),
		ExpectedStatusCode: 200,
		Description:        "レポートを取得できること",
	})
	if err != nil {
		return err
	}

	return nil
}

// Validation

func CheckStaticFiles(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	for _, staticFile := range StaticFiles {
		sf := staticFile
		err := checker.Play(ctx, &CheckAction{
			Method:             "GET",
			Path:               sf.Path,
			ExpectedStatusCode: 200,
			Description:        "静的ファイルが取得できること",
			CheckFunc: func(res *http.Response, body *bytes.Buffer) error {
				hasher := md5.New()
				_, err := io.Copy(hasher, body)
				if err != nil {
					return fatalErrorf("レスポンスボディの取得に失敗 %v", err)
				}
				hash := hex.EncodeToString(hasher.Sum(nil))
				if hash != sf.Hash {
					return fatalErrorf("静的ファイルの内容が正しくありません")
				}
				return nil
			},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func checkJsonUserCreateResponse(user *AppUser) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		bytes := body.Bytes()
		dec := json.NewDecoder(body)
		jsonUser := JsonUser{}
		err := dec.Decode(&jsonUser)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %s %v", string(bytes), err)
		}
		if jsonUser.Nickname != user.Nickname {
			log.Printf("warn: expected nickname=%s but got nickname=%s\n", user.Nickname, jsonUser.Nickname)
			return fatalErrorf("正しいユーザ情報を取得できません")
		}
		// Set auto incremented ID from response
		user.ID = jsonUser.ID
		return nil
	}
}

func checkJsonUserResponse(user *AppUser) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		bytes := body.Bytes()
		dec := json.NewDecoder(body)
		jsonUser := JsonUser{}
		err := dec.Decode(&jsonUser)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %s %v", string(bytes), err)
		}
		if jsonUser.ID != user.ID {
			log.Printf("warn: expected id=%d but got id=%d\n", user.ID, jsonUser.ID)
			return fatalErrorf("正しいユーザ情報を取得できません")
		} else if jsonUser.Nickname != user.Nickname {
			log.Printf("warn: expected nickname=%s but got nickname=%s (user_id=%d)\n", user.Nickname, jsonUser.Nickname, user.ID)
			return fatalErrorf("正しいユーザ情報を取得できません")
		}
		return nil
	}
}

func CheckCreateUser(ctx context.Context, state *State) error {
	user, checker, newUserPush := state.PopNewUser()
	if user == nil {
		return nil
	}
	checker.ResetCookie()

	err := checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/users",
		ExpectedStatusCode: 201,
		PostJSON: map[string]interface{}{
			"nickname":   user.Nickname,
			"login_name": user.LoginName,
			"password":   user.Password,
		},
		Description: "新規ユーザが作成できること",
		CheckFunc:   checkJsonUserCreateResponse(user),
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/login",
		ExpectedStatusCode: 200,
		PostJSON: map[string]interface{}{
			"login_name": user.LoginName,
			"password":   user.Password,
		},
		Description: "作成したユーザでログインできること",
		CheckFunc:   checkJsonUserResponse(user),
	})
	if err != nil {
		return err
	}
	user.Status.Online = true

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/users",
		ExpectedStatusCode: 409,
		PostJSON: map[string]interface{}{
			"nickname":   user.Nickname,
			"login_name": user.LoginName,
			"password":   user.Password,
		},
		Description: "すでに作成済みの場合エラーになること",
		CheckFunc:   checkJsonErrorResponse("duplicated"),
	})
	if err != nil {
		return err
	}

	newUserPush()

	return nil
}

func CheckLogin(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()
	checker.ResetCookie()
	user.Status.Online = false

	err := loginAppUser(ctx, checker, user)
	if err != nil {
		return err
	}

	err = logoutAppUser(ctx, checker, user)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/logout",
		ExpectedStatusCode: 401,
		Description:        "ログアウト済みの場合エラーになること",
		CheckFunc:          checkJsonErrorResponse("login_required"),
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/login",
		ExpectedStatusCode: 401,
		PostJSON: map[string]interface{}{
			"login_name": RandomAlphabetString(32),
			"password":   user.Password,
		},
		Description: "存在しないユーザでログインできないこと",
		CheckFunc:   checkJsonErrorResponse("authentication_failed"),
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/login",
		ExpectedStatusCode: 401,
		PostJSON: map[string]interface{}{
			"login_name": user.LoginName,
			"password":   RandomAlphabetString(32),
		},
		Description: "パスワードが間違っている場合ログインできないこと",
		CheckFunc:   checkJsonErrorResponse("authentication_failed"),
	})
	if err != nil {
		return err
	}

	return nil
}

func CheckTopPage(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	switch rand.Intn(3) {
	case 0:
		err := loginAppUser(ctx, checker, user)
		if err != nil {
			return err
		}
	case 1:
		err := logoutAppUser(ctx, checker, user)
		if err != nil {
			return err
		}
		// case 2: do nothing
	}

	// Assume that public events are not modified (closed or private)
	timeBefore := time.Now().Add(-1 * parameter.AllowableDelay)
	eventsBeforeRequest := FilterEventsToAllowDelay(FilterPublicEvents(state.GetCopiedEvents()), timeBefore)

	err := checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/",
		ExpectedStatusCode: 200,
		Description:        "ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			h := htmldigest.NewHash(func() hash.Hash {
				return crc32.NewIEEE()
			})
			crcSum, err := h.Sum(doc.Nodes[0])
			if err != nil {
				fmt.Fprint(os.Stderr, "HTML: ")
				_ = html.Render(os.Stderr, doc.Nodes[0])
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, err)
				return fatalErrorf("チェックサムの生成に失敗しました (主催者に連絡してください)")
			}
			if crcSum32 := JoinCrc32(crcSum); crcSum32 != ExpectedIndexHash {
				fmt.Fprint(os.Stderr, "HTML: ")
				_ = html.Render(os.Stderr, doc.Nodes[0])
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintf(os.Stderr, "crcSum32=%d\n", crcSum32)
				return fatalErrorf("DOM構造が初期状態と一致しません")
			}

			selection := doc.Find("#app-wrapper")
			if selection == nil || len(selection.Nodes) == 0 {
				return fatalErrorf("app-wrapperが見つかりません")
			}

			var found int
			node := selection.Nodes[0]
			for _, attr := range node.Attr {
				switch attr.Key {
				case "data-events":
					var events []JsonEvent
					err := json.Unmarshal([]byte(attr.Val), &events)
					if err != nil {
						return fatalErrorf("トップページのイベント一覧のJsonデコードに失敗 %s %v", attr.Val, err)
					}

					if len(events) == 0 {
						log.Println("warn: checkEventList: events is empty")
						return fatalErrorf("トップページのイベントの数が正しくありません")
					} else if len(events) < len(eventsBeforeRequest) {
						log.Printf("warn: checkEventList: len(events):%d < len(eventsBeforeRequest):%d\n", len(events), len(eventsBeforeRequest))
						return fatalErrorf("トップページのイベントの数が正しくありません")
					}

					ok := sort.SliceIsSorted(events, func(i, j int) bool {
						return events[i].ID < events[j].ID
					})
					if !ok {
						return fatalErrorf("トップページのイベントの順番が正しくありません")
					}

					eventsAfterResponse := FilterPublicEvents(state.GetEvents())
					err = checkEventList(state, eventsBeforeRequest, events, eventsAfterResponse)
					if err != nil {
						var msg string
						if ferr, ok := err.(*fatalError); ok {
							msg = ferr.msg
						} else {
							msg = err.Error()
						}
						return fatalErrorf("トップページのイベント一覧: %s", msg)
					}

					found++
				case "data-login-user":
					if user.Status.Online {
						var u *JsonUser
						err := json.Unmarshal([]byte(attr.Val), &u)
						if err != nil {
							return fatalErrorf("ログインユーザーのJsonデコードに失敗 %s %v", attr.Val, err)
						}
						if u == nil {
							return fatalErrorf("ログインユーザーがnull")
						}
						if u.ID != user.ID || u.Nickname != user.Nickname {
							return fatalErrorf("ログインユーザーが違います")
						}
					} else {
						if attr.Val != "null" {
							return fatalErrorf("ログインユーザーが非null")
						}
					}

					found++
				}
			}

			if found != 2 {
				return fatalErrorf("app-wrapperにdata-eventsまたはdata-login-userがありません")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	return nil
}

func CheckAdminTopPage(ctx context.Context, state *State) error {
	admin, checker, push := state.PopRandomAdministrator()
	if admin == nil {
		return nil
	}
	defer push()

	err := loginAdministrator(ctx, checker, admin)
	if err != nil {
		return err
	}

	timeBefore := time.Now().Add(-1 * parameter.AllowableDelay)
	eventsBeforeRequest := FilterEventsToAllowDelay(state.GetCopiedEvents(), timeBefore)

	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/admin/",
		ExpectedStatusCode: 200,
		Description:        "ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			h := htmldigest.NewHash(func() hash.Hash {
				return crc32.NewIEEE()
			})
			crcSum, err := h.Sum(doc.Nodes[0])
			if err != nil {
				fmt.Fprint(os.Stderr, "HTML: ")
				_ = html.Render(os.Stderr, doc.Nodes[0])
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, err)
				return fatalErrorf("チェックサムの生成に失敗しました (主催者に連絡してください)")
			}
			if crcSum32 := JoinCrc32(crcSum); crcSum32 != ExpectedAdminHash {
				fmt.Fprint(os.Stderr, "HTML: ")
				_ = html.Render(os.Stderr, doc.Nodes[0])
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintf(os.Stderr, "crcSum32=%d\n", crcSum32)
				return fatalErrorf("DOM構造が初期状態と一致しません")
			}

			selection := doc.Find("#app-wrapper")
			if selection == nil || len(selection.Nodes) == 0 {
				return fatalErrorf("app-wrapperが見つかりません")
			}

			var found int
			node := selection.Nodes[0]
			for _, attr := range node.Attr {
				switch attr.Key {
				case "data-events":
					var events []JsonEvent
					err := json.Unmarshal([]byte(attr.Val), &events)
					if err != nil {
						return fatalErrorf("管理画面のイベント一覧のJsonデコードに失敗 %s %v", attr.Val, err)
					}

					if len(events) == 0 {
						log.Println("warn: checkEventList: events is empty")
						return fatalErrorf("管理画面のイベントの数が正しくありません")
					} else if len(events) < len(eventsBeforeRequest) {
						log.Printf("warn: checkEventList: len(events):%d < len(eventsBeforeRequest):%d\n", len(events), len(eventsBeforeRequest))
						return fatalErrorf("管理画面のイベントの数が正しくありません")
					}

					ok := sort.SliceIsSorted(events, func(i, j int) bool {
						return events[i].ID < events[j].ID
					})
					if !ok {
						return fatalErrorf("管理画面のイベントの順番が正しくありません")
					}

					eventsAfterResponse := state.GetEvents()
					err = checkEventList(state, eventsBeforeRequest, events, eventsAfterResponse)
					if err != nil {
						var msg string
						if ferr, ok := err.(*fatalError); ok {
							msg = ferr.msg
						} else {
							msg = err.Error()
						}
						return fatalErrorf("管理画面のイベント一覧: %s", msg)
					}

					found++
				case "data-administrator":
					var u *JsonAdministrator
					err := json.Unmarshal([]byte(attr.Val), &u)
					if err != nil {
						return fatalErrorf("管理者情報のJsonデコードに失敗 %s %v", attr.Val, err)
					}
					if u == nil {
						return fatalErrorf("管理者情報がnull")
					}
					if u.ID != admin.ID || u.Nickname != admin.Nickname {
						return fatalErrorf("管理者情報が違います")
					}

					found++
				}
			}

			if found != 2 {
				return fatalErrorf("app-wrapperにdata-eventsまたはdata-administratorがありません")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	return nil
}

func CheckMyPage(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	err := loginAppUser(ctx, checker, user)
	if err != nil {
		return err
	}

	// Assume that public events are not modified (closed or private)
	timeBefore := time.Now().Add(-1 * parameter.AllowableDelay)
	eventsBeforeRequestOrig := FilterEventsToAllowDelay(state.GetCopiedEvents(), timeBefore)

	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/api/users/%d", user.ID),
		ExpectedStatusCode: 200,
		Description:        "ページが表示されること",
		CheckFunc: checkJsonFullUserResponse(user, func(fullUser *JsonFullUser) error {
			// check total price range
			if !(user.Status.NegativeTotalPrice <= fullUser.TotalPrice || fullUser.TotalPrice <= user.Status.PositiveTotalPrice) {
				log.Printf("warn: miss match user total price expected=%s got=%d userID=%d\n", user.Status.TotalPriceString(), fullUser.TotalPrice, fullUser.ID)
				return fatalErrorf("予約総額が最新の状態ではありません userID=%d", fullUser.ID)
			}

			// check duplicate and filter expected events
			var eventsBeforeRequest []*Event
			var eventsAfterResponse []*Event
			{
				seen := map[uint]struct{}{}
				for _, r := range fullUser.RecentReservations {
					_, exists := seen[r.ReservationID]
					if exists {
						return fatalErrorf("最近予約した席が重複しています userID=%d", fullUser.ID)
					}

					seen[r.ReservationID] = struct{}{}
				}

				seen = map[uint]struct{}{}
				for _, e := range fullUser.RecentEvents {
					_, exists := seen[e.ID]
					if exists {
						return fatalErrorf("最近予約したイベントが重複しています userID=%d", fullUser.ID)
					}

					seen[e.ID] = struct{}{}
				}

				// filter events
				for _, e := range eventsBeforeRequestOrig {
					_, exists := seen[e.ID]
					if !exists {
						continue
					}

					eventsBeforeRequest = append(eventsBeforeRequest, e)
				}
				for _, e := range state.GetEvents() {
					_, exists := seen[e.ID]
					if !exists {
						continue
					}

					eventsAfterResponse = append(eventsAfterResponse, e)
				}
			}

			// check first recent reservation id
			if len(fullUser.RecentReservations) >= 1 {
				r := fullUser.RecentReservations[0]
				if id := user.Status.LastReservation.GetID(timeBefore); id != 0 {
					maybeID := user.Status.LastMaybeReservation.GetID(timeBefore)
					if r.ReservationID != id && r.ReservationID != maybeID {
						log.Printf("warn: miss match user first recent reservation userID=%d\n", fullUser.ID)
						log.Printf("info: r.ReservationID=%d id=%d maybeID=%d\n", r.ReservationID, id, maybeID)
						return fatalErrorf("最近予約した席が最新の状態ではありません userID=%d", fullUser.ID)
					}
				}
			}

			reservationMap := state.GetReservations()
			reservations := []*Reservation{}
			for _, r := range fullUser.RecentReservations {
				// check event details
				if e := state.GetEventByID(r.Event.ID); e == nil {
					return fatalErrorf("最近予約した席のイベント情報(id)が正しくありません userID=%d", fullUser.ID)
				} else if r.Event.Title != e.Title {
					return fatalErrorf("最近予約した席のイベント情報(title)が正しくありません userID=%d", fullUser.ID)
				} else if r.Event.Closed != e.ClosedFg {
					return fatalErrorf("最近予約した席のイベント情報(closed)が正しくありません userID=%d", fullUser.ID)
				} else if r.Event.Public != e.PublicFg {
					return fatalErrorf("最近予約した席のイベント情報(public)が正しくありません userID=%d", fullUser.ID)
				}

				// check sheet details
				reservation, ok := reservationMap[r.ReservationID]
				if !ok {
					// skip
					log.Printf("warn: skip unknown reservation id:%d userID=%d\n", r.ReservationID, fullUser.ID)
					continue
				}
				if r.Event.ID != reservation.EventID {
					log.Printf("info: miss match reservation event id got=%d expected=%d\n", r.Event.ID, reservation.EventID)
					return fatalErrorf("最近予約した席のイベントが正しくありません userID=%d reservationID=%d", fullUser.ID, reservation.ID)
				}
				if r.SheetRank != reservation.SheetRank {
					log.Printf("info: miss match reservation sheet rank got=%d expected=%d\n", r.SheetRank, reservation.SheetRank)
					return fatalErrorf("最近予約した席のランクが正しくありません userID=%d reservationID=%d", fullUser.ID, reservation.ID)
				}
				if r.SheetNum != reservation.SheetNum {
					log.Printf("info: miss match reservation sheet num got=%d expected=%d\n", r.SheetNum, reservation.SheetNum)
					return fatalErrorf("最近予約した席の席番号が正しくありません userID=%d reservationID=%d", fullUser.ID, reservation.ID)
				}
				if r.Price != reservation.Price {
					log.Printf("info: miss match reservation price got=%d expected=%d\n", r.Price, reservation.Price)
					return fatalErrorf("最近予約した席の価格が正しくありません userID=%d reservationID=%d", fullUser.ID, reservation.ID)
				}

				// check reserved at
				if r.ReservedAt == 0 {
					return fatalErrorf("最近予約した席の予約時刻が正しくありません userID=%d reservationID=%d", fullUser.ID, reservation.ID)
				}
				if reservation.ReserveCompletedAt.IsZero() {
					log.Printf("warn: invalid reservation object is got=%#v\n", reservation)
					return nil
				} else if reservedAt := time.Unix(int64(r.ReservedAt), 0); reservation.ReserveCompletedAt.Before(reservedAt) {
					log.Printf("warn: reserved at should be (reservationID:%d) %s < %s\n", reservation.ID, reservedAt, reservation.ReserveCompletedAt)
					return fatalErrorf("最近予約した席の予約時刻が正しくありません userID=%d reservationID=%d", fullUser.ID, reservation.ID)
				}

				// check canceled at
				canceledAt := int64(r.CanceledAt)
				if canceledAt == 0 {
					if !reservation.CancelCompletedAt.IsZero() && reservation.CancelCompletedAt.Before(timeBefore) {
						// should not be canceled
						log.Printf("warn: miss match reservation cancellation status expected=canceled userID=%d reservationID=%d\n", fullUser.ID, reservation.ID)
						return fatalErrorf("最近予約した席のキャンセル状態が正しくありません userID=%d reservationID=%d", fullUser.ID, reservation.ID)
					}
				} else {
					if reservation.CancelRequestedAt.IsZero() {
						log.Printf("warn: miss match reservation cancellation status expected=not-canceled userID=%d reservationID=%d\n", fullUser.ID, reservation.ID)
						return fatalErrorf("最近予約した席のキャンセル時刻が正しくありません userID=%d reservationID=%d", fullUser.ID, reservation.ID)
					}

					cancelRequestedAt := reservation.CancelRequestedAt.Unix()
					if reservation.CancelCompletedAt.IsZero() {
						if !(cancelRequestedAt <= canceledAt) {
							log.Printf("warn: miss match reservation cancellation status expected=not-canceled userID=%d reservationID=%d\n", fullUser.ID, reservation.ID)
							return fatalErrorf("最近予約した席のキャンセル時刻が正しくありません userID=%d reservationID=%d", fullUser.ID, reservation.ID)
						}
					} else {
						cancelCompletedAt := reservation.CancelCompletedAt.Unix()
						if !(cancelRequestedAt <= canceledAt && canceledAt <= cancelCompletedAt) {
							log.Printf("warn: miss match reservation cancellation status expected=not-canceled userID=%d reservationID=%d\n", fullUser.ID, reservation.ID)
							return fatalErrorf("最近予約した席のキャンセル時刻が正しくありません userID=%d reservationID=%d", fullUser.ID, reservation.ID)
						}
					}
				}

				// add reservations
				reservations = append(reservations, reservation)
			}

			// check order
			if len(reservations) >= 2 {
				last := reservations[0]
				for _, r := range reservations[1:] {
					if last.LastMaybeUpdatedAt().Before(r.LastUpdatedAt()) {
						log.Printf("warn: miss match user recent reservation order userID=%d\n", fullUser.ID)
						return fatalErrorf("最近予約した席の順番が正しくありません")
					}
				}
			}

			// check first recent event id
			if len(fullUser.RecentEvents) >= 1 {
				e := fullUser.RecentEvents[0]
				if id := user.Status.LastReservedEvent.GetID(timeBefore); id != 0 {
					maybeID := user.Status.LastMaybeReservedEvent.GetID(timeBefore)
					if e.ID != id && e.ID != maybeID {
						log.Printf("warn: miss match user first recent event userID=%d\n", fullUser.ID)
						log.Printf("info: e.ID=%d id=%d maybeID=%d\n", e.ID, id, maybeID)
						return fatalErrorf("最近予約したイベントが最新の状態ではありません")
					}
				}
			}

			// check event details
			if len(fullUser.RecentEvents) >= 0 {
				events := make([]JsonEvent, len(fullUser.RecentEvents))
				for i, re := range fullUser.RecentEvents {
					// check event status
					if e := state.GetEventByID(re.ID); e == nil {
						return fatalErrorf("最近予約したイベントのイベント情報(id)が正しくありません")
					} else if re.Closed != e.ClosedFg {
						return fatalErrorf("最近予約したイベントのイベント情報(closed)が正しくありません")
					} else if re.Public != e.PublicFg {
						return fatalErrorf("最近予約したイベントのイベント情報(public)が正しくありません")
					}

					events[i] = re.JsonEvent
				}

				err := checkEventList(state, eventsBeforeRequest, events, eventsAfterResponse)
				if err != nil {
					var msg string
					if ferr, ok := err.(*fatalError); ok {
						msg = ferr.msg
					} else {
						msg = err.Error()
					}
					return fatalErrorf("最近予約したイベント一覧(userID=%d): %s", user.ID, msg)
				}
			}

			// check order
			if len(fullUser.RecentEvents) >= 2 {
				eventOrderMap := map[uint]int{}
				for i := len(fullUser.RecentReservations) - 1; i >= 0; i-- {
					r := fullUser.RecentReservations[i]
					eventOrderMap[r.Event.ID] = i
				}

				lastOrder := 0
				for _, e := range fullUser.RecentEvents {
					order, ok := eventOrderMap[e.ID]
					if !ok {
						continue
					}

					if lastOrder > order {
						log.Printf("warn: miss match user recent event order userID=%d (%#v)\n", fullUser.ID, fullUser.RecentEvents)
						log.Printf("info: order=%d lastOrder=%d\n", order, lastOrder)
						return fatalErrorf("最近予約したイベントの順番が正しくありません userID=%d", fullUser.ID)
					}
					lastOrder = order
				}
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	return nil
}

// たまには売り切れイベントをキャンセルさせて、キャッシュしにくくさせる
// キャンセルを待ってイベントページをF5しているユーザもいる想定なのでキャンセルしてあげる
// (簡単のため)キャンセルしたら別のユーザですぐに予約する
// (簡単のため)Check関数にして１並列でしか動かないようにする
// (簡単のため)Check関数にして sold_out 状態に戻らなかったら fail
func CheckCancelReserveSheet(ctx context.Context, state *State) error {
	// LoadGetEvent() can run concurrently, but CheckCancelReserveSheet() can not
	state.getRandomPublicSoldOutEventRWMtx.Lock()
	defer state.getRandomPublicSoldOutEventRWMtx.Unlock()

	event := state.GetRandomPublicSoldOutEvent()
	if event == nil {
		log.Printf("warn: checkCancelReserveSheet: no public and sold-out event")
		return nil
	}
	reservation := state.GetRandomNonCanceledReservationInEventID(event.ID)
	if reservation == nil {
		log.Printf("warn: checkCancelReserveSheet: no reservation which is not canceled in event:%d\n", event.ID)
		return nil
	}

	eventID := event.ID
	rank := reservation.SheetRank

	cancelUser, cacnelChecker, cancelUserPush := state.PopUserByID(reservation.UserID)
	if cancelUser == nil {
		return nil
	}
	defer cancelUserPush()

	err := loginAppUser(ctx, cacnelChecker, cancelUser)
	if err != nil {
		return err
	}

	reserveUser, reserveChecker, reserveUserPush := state.PopRandomUser()
	if reserveUser == nil {
		return nil
	}
	defer reserveUserPush()

	err = loginAppUser(ctx, reserveChecker, reserveUser)
	if err != nil {
		return err
	}

	// For simplicity, s.reservedEventSheets are not modified in this method.
	eventSheet := &EventSheet{eventID, rank, NonReservedNum, event.Price + DataSet.SheetKindMap[rank].Price}

	already_locked, err := cancelSheet(ctx, state, cacnelChecker, cancelUser, eventSheet, reservation)
	if err != nil {
		return err
	}
	if already_locked {
		return nil
	}

	_, err = reserveSheet(ctx, state, reserveChecker, reserveUser, eventSheet)
	if err != nil {
		return err
	}

	// NOTE: Let me skip 409 check. We do not know how many times we should retry because reserve may timeout.
	// Retrying forever makes a problem that benchmarker cannot check further scenarios.
	// err = reserveChecker.Play(ctx, &CheckAction{
	// 	Method:             "POST",
	// 	Path:               fmt.Sprintf("/api/events/%d/actions/reserve", eventID),
	// 	ExpectedStatusCode: 409,
	// 	Description:        "売り切れの場合エラーになること",
	// 	PostJSON: map[string]interface{}{
	// 		"sheet_rank": rank,
	// 	},
	// 	CheckFunc: checkJsonErrorResponse("sold_out"),
	// })
	// if err != nil {
	// 	log.Printf("warn: %s\n", err)
	// 	return err
	// }

	return nil
}

func CheckReserveSheet(ctx context.Context, state *State) error {
	user, userChecker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	err := loginAppUser(ctx, userChecker, user)
	if err != nil {
		return err
	}

	eventSheet, eventSheetPush, err := popOrCreateEventSheet(ctx, state)
	if err != nil {
		return err
	}
	if eventSheet == nil {
		return nil
	}

	eventID := eventSheet.EventID
	rank := eventSheet.Rank

	reservation, err := reserveSheet(ctx, state, userChecker, user, eventSheet)
	if reservation == nil && err == nil {
		return nil
	}
	if err != nil {
		return err
	}
	defer eventSheetPush() // NOTE: push only after reserve succeeds

	already_locked, err := cancelSheet(ctx, state, userChecker, user, eventSheet, reservation)
	if err != nil {
		return err
	}
	if already_locked {
		return nil
	}

	// TODO(sonots): Fix race conditions that following error occurs
	// Response code should be 400, got 403, data: <nil> (DELETE /api/events/11/sheets/C/498/reservation )
	// It is caused if somebody else reserves the canceled sheet before calling the following 2nd cancelation.
	// err = userChecker.Play(ctx, &CheckAction{
	// 	Method:             "DELETE",
	// 	Path:               fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, reservation.SheetRank, reservation.SheetNum),
	// 	ExpectedStatusCode: 400,
	// 	Description:        "すでにキャンセル済みの場合エラーになること",
	// 	CheckFunc:          checkJsonErrorResponse("not_reserved"),
	// })
	// if err != nil {
	// 	return err
	// }

	// TODO(sonots): Need to find a sheet which somebody else reserved.
	// err := userChecker.Play(ctx, &CheckAction{
	// 	Method:      "DELETE",
	// 	Path:        fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, reservation.SheetRank, reservation.SheetNum),
	// 	ExpectedStatusCode: 403,
	// 	Description: "購入していないチケットをキャンセルしようとするとエラーになること",
	//	CheckFunc:          checkJsonErrorResponse("not_permitted"),
	// })
	// if err != nil {
	// 	return err
	// }

	// TODO(sonots): Randomize, but find ID which does not exist.
	unknownEventID := 0
	err = userChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               fmt.Sprintf("/api/events/%d/actions/reserve", unknownEventID),
		ExpectedStatusCode: 404,
		Description:        "存在しないイベントのシートを予約しようとするとエラーになること",
		CheckFunc:          checkJsonErrorResponse("invalid_event"),
		PostJSON: map[string]interface{}{
			"sheet_rank": rank,
		},
	})
	if err != nil {
		return err
	}

	unknownRank := "N"
	err = userChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               fmt.Sprintf("/api/events/%d/actions/reserve", eventID),
		ExpectedStatusCode: 400,
		Description:        "存在しないランクのシートを予約しようとするとエラーになること",
		CheckFunc:          checkJsonErrorResponse("invalid_rank"),
		PostJSON: map[string]interface{}{
			"sheet_rank": unknownRank,
		},
	})
	if err != nil {
		return err
	}

	randomNum := GetRandomSheetNum(rank)
	err = userChecker.Play(ctx, &CheckAction{
		Method:             "DELETE",
		Path:               fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", unknownEventID, rank, randomNum),
		ExpectedStatusCode: 404,
		Description:        "存在しないイベントのシートをキャンセルしようとするとエラーになること",
		CheckFunc:          checkJsonErrorResponse("invalid_event"),
	})
	if err != nil {
		return err
	}

	err = userChecker.Play(ctx, &CheckAction{
		Method:             "DELETE",
		Path:               fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, "D", randomNum),
		ExpectedStatusCode: 404,
		Description:        "存在しないランクのシートをキャンセルしようとするとエラーになること",
		CheckFunc:          checkJsonErrorResponse("invalid_rank"),
	})
	if err != nil {
		return err
	}

	unknownNum := 1 + DataSet.SheetKinds[0].Total + uint(rand.Intn(int(DataSet.SheetKinds[0].Total)))
	err = userChecker.Play(ctx, &CheckAction{
		Method:             "DELETE",
		Path:               fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, DataSet.SheetKinds[0].Rank, unknownNum),
		ExpectedStatusCode: 404,
		Description:        "存在しないシートをキャンセルしようとするとエラーになること",
		CheckFunc:          checkJsonErrorResponse("invalid_sheet"),
	})
	if err != nil {
		return err
	}

	checker := NewChecker()

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               fmt.Sprintf("/api/events/%d/actions/reserve", eventID),
		ExpectedStatusCode: 401,
		Description:        "ログインしていない場合予約ができないこと",
		CheckFunc:          checkJsonErrorResponse("login_required"),
		PostJSON: map[string]interface{}{
			"sheet_rank": rank,
		},
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "DELETE",
		Path:               fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, rank, randomNum),
		ExpectedStatusCode: 401,
		Description:        "ログインしていない場合キャンセルができないこと",
		CheckFunc:          checkJsonErrorResponse("login_required"),
	})
	if err != nil {
		return err
	}

	return nil
}

func checkJsonAdministratorResponse(admin *Administrator) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		bytes := body.Bytes()
		dec := json.NewDecoder(body)
		jsonAdmin := JsonAdministrator{}
		err := dec.Decode(&jsonAdmin)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %s %v", string(bytes), err)
		}
		if jsonAdmin.ID != admin.ID || jsonAdmin.Nickname != admin.Nickname {
			return fatalErrorf("正しい管理者情報を取得できません")
		}
		return nil
	}
}

func CheckAdminLogin(ctx context.Context, state *State) error {
	admin, adminChecker, adminPush := state.PopRandomAdministrator()
	if admin == nil {
		return nil
	}
	defer adminPush()
	adminChecker.ResetCookie()
	admin.Status.Online = false

	user, userChecker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	err := userChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/admin/api/actions/login",
		ExpectedStatusCode: 401,
		PostJSON: map[string]interface{}{
			"login_name": user.LoginName,
			"password":   user.Password,
		},
		Description: "一般ユーザで管理者ログインできないこと",
		CheckFunc:   checkJsonErrorResponse("authentication_failed"),
	})
	if err != nil {
		return err
	}

	err = loginAdministrator(ctx, adminChecker, admin)
	if err != nil {
		return err
	}

	err = logoutAdministrator(ctx, adminChecker, admin)
	if err != nil {
		return err
	}

	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/admin/api/actions/logout",
		ExpectedStatusCode: 401,
		Description:        "ログアウト済みの場合エラーになること",
		CheckFunc:          checkJsonErrorResponse("admin_login_required"),
	})
	if err != nil {
		return err
	}

	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/admin/api/actions/login",
		ExpectedStatusCode: 401,
		PostJSON: map[string]interface{}{
			"login_name": RandomAlphabetString(32),
			"password":   admin.Password,
		},
		Description: "存在しないユーザで管理者ログインできないこと",
		CheckFunc:   checkJsonErrorResponse("authentication_failed"),
	})
	if err != nil {
		return err
	}

	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/admin/api/actions/login",
		ExpectedStatusCode: 401,
		PostJSON: map[string]interface{}{
			"login_name": admin.LoginName,
			"password":   RandomAlphabetString(32),
		},
		Description: "パスワードが間違っている場合管理者ログインできないこと",
		CheckFunc:   checkJsonErrorResponse("authentication_failed"),
	})
	if err != nil {
		return err
	}

	return nil
}

func checkJsonFullEventCreateResponse(event *Event) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		bytes := body.Bytes()
		dec := json.NewDecoder(body)
		jsonEvent := JsonFullEvent{}
		err := dec.Decode(&jsonEvent)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %s %v", string(bytes), err)
		}
		if jsonEvent.Title != event.Title || jsonEvent.Price != event.Price || jsonEvent.Public != event.PublicFg || jsonEvent.Closed != event.ClosedFg {
			return fatalErrorf("正しいイベントを取得できません")
		}
		// Set created time and auto incremented ID from response
		event.ID = jsonEvent.ID
		event.CreatedAt = time.Now()
		return nil
	}
}

func checkJsonFullEventResponse(event *Event) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		bytes := body.Bytes()
		dec := json.NewDecoder(body)
		jsonEvent := JsonFullEvent{}
		err := dec.Decode(&jsonEvent)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %s %v", string(bytes), err)
		}
		if jsonEvent.ID != event.ID || jsonEvent.Title != event.Title || jsonEvent.Price != event.Price || jsonEvent.Public != event.PublicFg {
			return fatalErrorf("正しいイベントを取得できません")
		}
		return nil
	}
}

func checkJsonEventResponse(event *Event, cb func(JsonEvent) error) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		bytes := body.Bytes()

		dec := json.NewDecoder(body)
		jsonEvent := JsonEvent{}
		err := dec.Decode(&jsonEvent)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %s %v", string(bytes), err)
		}

		// basic checks
		if jsonEvent.ID != event.ID || jsonEvent.Title != event.Title || len(jsonEvent.Sheets) != len(DataSet.SheetKinds) {
			return fatalErrorf("正しいイベント(id:%d)を取得できません", event.ID)
		}
		if jsonEvent.Sheets == nil {
			return fatalErrorf("イベント(id:%d)のシート定義が取得できません", event.ID)
		}
		for rank, sheets := range jsonEvent.Sheets {
			sheetKind := DataSet.SheetKindMap[rank]
			if sheets.Details == nil || int(sheetKind.Total) != len(sheets.Details) {
				return fatalErrorf("イベント(id:%d)のシートの詳細情報が取得できません", event.ID)
			}

			reservedCount := 0
			for i, sheet := range sheets.Details {
				if int(sheet.Num) != i+1 {
					return fatalErrorf("イベント(id:%d)のシートの順番が違います", event.ID)
				}
				if sheet.Reserved {
					reservedCount++
				}
			}
			if reservedCount != int(sheets.Total-sheets.Remains) {
				return fatalErrorf("イベント(id:%d)のシートの予約状況が矛盾しています", event.ID)
			}
		}

		if cb != nil {
			return cb(jsonEvent)
		}
		return nil
	}
}

func eventPostJSON(event *Event) map[string]interface{} {
	return map[string]interface{}{
		"title":  event.Title,
		"public": event.PublicFg,
		"price":  event.Price,
	}
}

func eventEditJSON(event *Event) map[string]bool {
	return map[string]bool{
		"public": event.PublicFg,
		"closed": event.ClosedFg,
	}
}

func CheckCreateEvent(ctx context.Context, state *State) error {
	checker := NewChecker()

	admin, adminChecker, adminPush := state.PopRandomAdministrator()
	if admin == nil {
		return nil
	}
	defer adminPush()

	user, userChecker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	err := loginAdministrator(ctx, adminChecker, admin)
	if err != nil {
		return err
	}

	err = loginAppUser(ctx, userChecker, user)
	if err != nil {
		return err
	}

	event, newEventPush := state.CreateNewEvent()

	err = userChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/admin/api/events",
		ExpectedStatusCode: 401,
		Description:        "一般ユーザがイベントを作成できないこと",
		PostJSON:           eventPostJSON(event),
		CheckFunc:          checkJsonErrorResponse("admin_login_required"),
	})
	if err != nil {
		return err
	}

	// Create as a private event
	event.PublicFg = false

	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/admin/api/events",
		ExpectedStatusCode: 200,
		Description:        "管理者がイベントを作成できること",
		PostJSON:           eventPostJSON(event),
		CheckFunc:          checkJsonFullEventCreateResponse(event),
	})
	if err != nil {
		return err
	}
	newEventPush("CheckCreateEvent")

	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/api/events/%d", event.ID),
		ExpectedStatusCode: 404,
		Description:        "非公開イベントを取得できないこと",
		CheckFunc:          checkJsonErrorResponse("not_found"),
	})
	if err != nil {
		return err
	}

	err = userChecker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/admin/api/events/%d", event.ID),
		ExpectedStatusCode: 401,
		Description:        "一般ユーザが管理者APIでイベントを取得できないこと",
		CheckFunc:          checkJsonErrorResponse("admin_login_required"),
	})
	if err != nil {
		return err
	}

	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/admin/api/events/%d", event.ID),
		ExpectedStatusCode: 200,
		Description:        "管理者が非公開イベントを取得できること",
		CheckFunc:          checkJsonFullEventResponse(event),
	})
	if err != nil {
		return err
	}

	err = userChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               fmt.Sprintf("/admin/api/events/%d/actions/edit", event.ID),
		ExpectedStatusCode: 401,
		Description:        "一般ユーザがイベントを編集できないこと",
		PostJSON:           eventPostJSON(event),
		CheckFunc:          checkJsonErrorResponse("admin_login_required"),
	})
	if err != nil {
		return err
	}

	// Publish an event
	event.PublicFg = true

	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               fmt.Sprintf("/admin/api/events/%d/actions/edit", event.ID),
		ExpectedStatusCode: 200,
		Description:        "管理者がイベントを編集できること",
		PostJSON:           eventEditJSON(event),
		CheckFunc:          checkJsonFullEventResponse(event),
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/api/events/%d", event.ID),
		ExpectedStatusCode: 200,
		Description:        "公開イベントを取得できること",
		CheckFunc:          checkJsonEventResponse(event, nil),
	})
	if err != nil {
		return err
	}

	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/admin/api/events/%d", event.ID),
		ExpectedStatusCode: 200,
		Description:        "管理者が公開イベントを取得できること",
		CheckFunc:          checkJsonFullEventResponse(event),
	})
	if err != nil {
		return err
	}

	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/admin/api/events/%d", event.ID+1),
		ExpectedStatusCode: 404,
		Description:        "イベントが存在しない場合取得に失敗すること",
		CheckFunc:          checkJsonErrorResponse("not_found"),
	})
	if err != nil {
		return err
	}

	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/api/events/%d", event.ID+1),
		ExpectedStatusCode: 404,
		Description:        "イベントが存在しない場合取得に失敗すること",
		CheckFunc:          checkJsonErrorResponse("not_found"),
	})
	if err != nil {
		return err
	}

	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               fmt.Sprintf("/admin/api/events/%d/actions/edit", event.ID+1),
		ExpectedStatusCode: 404,
		Description:        "イベントが存在しない場合編集に失敗すること",
		PostJSON:           eventPostJSON(event),
		CheckFunc:          checkJsonErrorResponse("not_found"),
	})
	if err != nil {
		return err
	}

	return nil
}

func checkReportHeader(reader *csv.Reader) error {
	// reservation_id,event_id,rank,num,price,user_id,sold_at,canceled_at
	row, err := reader.Read()
	if err == io.EOF ||
		len(row) != 8 ||
		row[0] != "reservation_id" ||
		row[1] != "event_id" ||
		row[2] != "rank" ||
		row[3] != "num" ||
		row[4] != "price" ||
		row[5] != "user_id" ||
		row[6] != "sold_at" ||
		row[7] != "canceled_at" {
		return fatalErrorf("正しいCSVヘッダを取得できません")
	}
	return nil
}

func getReportRecords(s *State, reader *csv.Reader) (map[uint]*ReportRecord, error) {
	// reservation_id,event_id,rank,num,price,user_id,sold_at,canceled_at
	// 1,1,S,36,8000,1002,2018-08-17T04:55:30Z,2018-08-17T04:58:31Z
	// 2,1,S,36,8000,1002,2018-08-17T04:55:32Z,
	// 3,1,B,149,4000,1002,2018-08-17T04:55:33Z,
	// 4,1,C,317,3000,1002,2018-08-17T04:55:34Z,
	// 5,1,B,27,4000,1002,2018-08-17T04:55:36Z,
	// 6,3,A,15,6000,1002,2018-08-17T04:55:38Z,
	// 7,3,S,10,8000,1002,2018-08-17T04:55:41Z,2018-08-17T04:58:29Z

	records := map[uint]*ReportRecord{}

	line := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		line++

		msg := "正しいCSVレポートを取得できません"

		if len(row) != 8 {
			return nil, fatalErrorf(msg)
		}

		reservationID, err := strconv.Atoi(row[0])
		if err != nil {
			log.Printf("debug: invalid reservationID (line:%d) error:%v\n", line, err)
			return nil, fatalErrorf(msg)
		}
		eventID, err := strconv.Atoi(row[1])
		if err != nil {
			log.Printf("debug: invalid eventID (line:%d) error:%v\n", line, err)
			return nil, fatalErrorf(msg)
		}
		sheetRank := row[2]

		sheetNum, err := strconv.Atoi(row[3])
		if err != nil {
			log.Printf("debug: invalid sheetNum (line:%d) error:%v\n", line, err)
			return nil, fatalErrorf(msg)
		}

		sheetPrice, err := strconv.Atoi(row[4])
		if err != nil {
			log.Printf("debug: invalid price (line:%d) error:%v\n", line, err)
			return nil, fatalErrorf(msg)
		}

		userID, err := strconv.Atoi(row[5])
		if err != nil {
			log.Printf("debug: invalid userID (line:%d) error:%v\n", line, err)
			return nil, fatalErrorf(msg)
		}

		_, err = time.Parse(time.RFC3339, row[6])
		if err != nil {
			log.Printf("debug: invalid soldAt (line:%d) error:%v\n", line, err)
			return nil, fatalErrorf(msg)
		}

		var canceledAt time.Time
		if row[7] != "" {
			canceledAt, err = time.Parse(time.RFC3339, row[7])
			if err != nil {
				log.Printf("debug: invalid canceledAt (line:%d) error:%v\n", line, err)
				return nil, fatalErrorf(msg)
			}
		}

		record := &ReportRecord{
			ReservationID: uint(reservationID),
			EventID:       uint(eventID),
			SheetRank:     sheetRank,
			SheetNum:      uint(sheetNum),
			SheetPrice:    uint(sheetPrice),
			UserID:        uint(userID),
			CanceledAt:    canceledAt,
		}

		records[record.ReservationID] = record
	}

	return records, nil
}

func checkReportRecord(s *State, records map[uint]*ReportRecord, timeBefore time.Time,
	reservationsBeforeRequest map[uint]*Reservation) error {

	for reservationID, reservationBeforeRequest := range reservationsBeforeRequest {
		// All elements in reservationsBeforeRequest must exist in records
		record, ok := records[reservationID]
		if !ok {
			log.Printf("debug: should exist (reservationID:%d)\n", reservationID)
			return fatalErrorf("レポートに予約id:%dの行が存在しません", reservationID)
		}

		event := s.FindEventByID(record.EventID)
		if event == nil {
			log.Printf("debug: event id=%d is not found (reservationID:%d)\n", record.EventID, reservationID)
			return fatalErrorf("レポート(予約id:%d)のイベントidが正しくありません", reservationID)
		}
		if expected := event.Price + GetSheetKindByRank(record.SheetRank).Price; record.SheetPrice != expected {
			log.Printf("debug: price:%d is not expected:%d (reservationID:%d)\n", record.SheetPrice, expected, reservationID)
			return fatalErrorf("レポート(予約id:%d)のシート価格が正しくありません", reservationID)
		}

		if reservationBeforeRequest.EventID != record.EventID {
			log.Printf("debug: event id=%d is not expected:%d (reservationID:%d)\n", record.EventID, reservationBeforeRequest.EventID, reservationID)
			return fatalErrorf("レポート(予約id:%d)のイベントidが正しくありません", reservationID)
		}
		if reservationBeforeRequest.UserID != record.UserID {
			log.Printf("debug: user id=%d is not expected:%d (reservationID:%d)\n", record.UserID, reservationBeforeRequest.UserID, reservationID)
			return fatalErrorf("レポート(予約id:%d)のユーザidが正しくありません", reservationID)
		}
		if reservationBeforeRequest.SheetRank != record.SheetRank {
			log.Printf("debug: sheet rank=%s is not expected:%s (reservationID:%d)\n", record.SheetRank, reservationBeforeRequest.SheetRank, reservationID)
			return fatalErrorf("レポート(予約id:%d)のシートランクが正しくありません", reservationID)
		}
		if reservationBeforeRequest.SheetNum != record.SheetNum {
			log.Printf("debug: sheet num=%d is not expected:%d (reservationID:%d)\n", record.SheetNum, reservationBeforeRequest.SheetNum, reservationID)
			return fatalErrorf("レポート(予約id:%d)のシート番号が正しくありません", reservationID)
		}

		if reservationBeforeRequest.Canceled(timeBefore) {
			if record.CanceledAt.IsZero() {
				log.Printf("debug: should have canceledAt (reservationID:%d)\n", reservationID)
				return fatalErrorf("レポート(予約id:%d)のキャンセル時刻が正しくありません", reservationID)
			}
		} else if reservationBeforeRequest.MaybeCanceled(timeBefore) {
			if record.CanceledAt.IsZero() {
				log.Printf("warn: should have canceledAt (reservationID:%d) but ignored (race condition)\n", reservationID)
			}
		}
	}

	return nil
}

func checkReportCount(
	reserveCompletedCountBeforeRequest int,
	reportCount int,
	reserveRequestedCountAfterResponse uint) error {
	log.Printf("debug: reserveCompletedCountBeforeRequest:%d <= reportCount:%d <= reserveRequestedCountAfterResponse:%d\n",
		reserveCompletedCountBeforeRequest,
		reportCount,
		reserveRequestedCountAfterResponse)
	if reserveCompletedCountBeforeRequest <= reportCount &&
		reportCount <= int(reserveRequestedCountAfterResponse) {
		return nil
	}
	return fatalErrorf("レポートの数が正しくありません")
}

func checkReportResponse(s *State, timeBefore time.Time, reservationsBeforeRequest map[uint]*Reservation) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		reserveRequestedCountAfterResponse := s.GetReserveRequestedCount()

		log.Println("debug:", body)
		reader := csv.NewReader(body)

		err := checkReportHeader(reader)
		if err != nil {
			return err
		}

		records, err := getReportRecords(s, reader)
		if err != nil {
			return err
		}

		err = checkReportRecord(s, records, timeBefore, reservationsBeforeRequest)
		if err != nil {
			return err
		}

		err = checkReportCount(len(reservationsBeforeRequest), len(records), reserveRequestedCountAfterResponse)
		if err != nil {
			return err
		}

		return nil
	}
}

func checkEventReportResponse(s *State, event *Event, timeBefore time.Time, reservationsBeforeRequest map[uint]*Reservation) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		reserveRequestedCountAfterResponse := event.GetReserveRequestedCount()

		log.Printf("debug: checkEventReport %d\n", event.ID)
		log.Println("debug:", body)
		reader := csv.NewReader(body)

		err := checkReportHeader(reader)
		if err != nil {
			return err
		}

		records, err := getReportRecords(s, reader)
		if err != nil {
			return err
		}

		msg := "正しいレポートを取得できません"
		for _, record := range records {
			if record.EventID != event.ID {
				log.Printf("debug: event id=%d does not match with id=%d (reservationID:%d)\n", record.EventID, event.ID, record.ReservationID)
				return fatalErrorf(msg)
			}
		}

		err = checkReportRecord(s, records, timeBefore, reservationsBeforeRequest)
		if err != nil {
			return err
		}

		err = checkReportCount(len(reservationsBeforeRequest), len(records), reserveRequestedCountAfterResponse)
		if err != nil {
			return err
		}

		return nil
	}
}

func CheckReport(ctx context.Context, state *State) error {
	admin, checker, push := state.PopRandomAdministrator()
	if admin == nil {
		return nil
	}
	defer push()

	err := loginAdministratorWithTimeout(ctx, checker, admin, parameter.PostTestLoginTimeout)
	if err != nil {
		return err
	}

	timeBefore := time.Now().Add(-1 * parameter.AllowableDelay)
	reservationsBeforeRequest := FilterReservationsToAllowDelay(state.GetCopiedReservations(), timeBefore)

	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/admin/api/reports/sales",
		ExpectedStatusCode: 200,
		Description:        "レポートを正しく取得できること",
		CheckFunc:          checkReportResponse(state, timeBefore, reservationsBeforeRequest),
		Timeout:            parameter.PostTestReportTimeout,
	})
	if err != nil {
		return err
	}

	return nil
}

func CheckEventReport(ctx context.Context, state *State) error {
	admin, checker, push := state.PopRandomAdministrator()
	if admin == nil {
		return nil
	}
	defer push()

	err := loginAdministrator(ctx, checker, admin)
	if err != nil {
		return err
	}

	// We want to let webapp to lock reservations.
	// Since no reserve/cancel occurs for closed events, we ignore closed events.
	// Notice that webapp locks to update reservations (cancel),
	// but it does not lock to create reservations (reserve).
	event := state.GetRandomPublicEvent()
	if event == nil {
		return nil
	}

	timeBefore := time.Now().Add(-1 * parameter.AllowableDelay)
	reservationsBeforeRequest := FilterReservationsToAllowDelay(state.GetCopiedReservationsInEventID(event.ID), timeBefore)

	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/admin/api/reports/events/%d/sales", event.ID),
		ExpectedStatusCode: 200,
		Description:        "レポートを正しく取得できること",
		CheckFunc:          checkEventReportResponse(state, event, timeBefore, reservationsBeforeRequest),
	})
	if err != nil {
		return err
	}

	return nil
}

func CheckSheetReservationEntropy(ctx context.Context, state *State) error {
	var event *Event
	var now time.Time
	var source map[uint]*Reservation

	// fetch source
	seen := map[uint]bool{}
	for retry := 0; retry < 5; retry++ {
		event = state.GetRandomPublicEvent()
		if event == nil {
			return nil
		}
		if seen[event.ID] {
			continue
		}
		seen[event.ID] = true

		now = time.Now()
		source = state.GetReservationsInEventID(event.ID)

		// NOTE(karupa): skip smallest or biggest source
		if l := len(source); !(10 < l && l < 600) {
			continue
		}

		break
	}

	// prepare
	reservationsMap := map[string][]*Reservation{}
	for _, reservation := range source {
		if reservation.MaybeCanceled(now) {
			continue
		}
		reservationsMap[reservation.SheetRank] = append(reservationsMap[reservation.SheetRank], reservation)
	}
	for _, reservations := range reservationsMap {
		sort.Slice(reservations, func(i, j int) bool {
			return reservations[i].ID < reservations[j].ID
		})
	}

	// calculate score
	// note(karupa): 等差数列であれば差分の和が一定以下の数になるはず/一定数以上
	scoreMap := map[string]uint{}
	for rank, reservations := range reservationsMap {
		if len(reservations) < 2 {
			continue
		}

		var score uint
		var before = float64(reservations[0].SheetNum)
		for _, reservation := range reservations[1:] {
			after := float64(reservation.SheetNum)
			score += uint(math.Abs(before - after))
			before = after
		}
		scoreMap[rank] = score / uint(len(reservations))
	}

	// check score
	ok := true
	for rank, score := range scoreMap {
		if score < 4 {
			log.Printf("error: fatal entropy score %s=%d (event_id:%d)\n", rank, score, event.ID)
			ok = false
		} else if score < 8 {
			log.Printf("warn: small entropy score %s=%d (event_id:%d)\n", rank, score, event.ID)
		} else if score < 16 {
			log.Printf("info: small entropy score %s=%d (event_id:%d)\n", rank, score, event.ID)
		} else {
			log.Printf("debug: normal entropy score %s=%d (event_id:%d)\n", rank, score, event.ID)
		}
	}
	if !ok {
		return fatalErrorf("予約順がランダムではありません: event_id:%d", event.ID)
	}

	return nil
}

func loginAdministrator(ctx context.Context, checker *Checker, admin *Administrator) error {
	return loginAdministratorWithTimeout(ctx, checker, admin, 0)
}

func loginAdministratorWithTimeout(ctx context.Context, checker *Checker, admin *Administrator, timeout time.Duration) error {
	if admin.Status.Online {
		return nil
	}

	err := checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/admin/api/actions/login",
		ExpectedStatusCode: 200,
		Description:        "管理者でログインできること",
		PostJSON: map[string]interface{}{
			"login_name": admin.LoginName,
			"password":   admin.Password,
		},
		CheckFunc: checkJsonAdministratorResponse(admin),
		Timeout:   timeout, // 0 to use default timeout
	})
	if err != nil {
		return err
	}

	admin.Status.Online = true
	return nil
}

func logoutAdministrator(ctx context.Context, checker *Checker, admin *Administrator) error {
	if !admin.Status.Online {
		return nil
	}

	err := checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/admin/api/actions/logout",
		ExpectedStatusCode: 204,
		Description:        "管理者でログアウトできること",
	})
	if err != nil {
		return err
	}

	admin.Status.Online = false
	return nil
}

func loginAppUser(ctx context.Context, checker *Checker, user *AppUser) error {
	if user.Status.Online {
		return nil
	}

	err := checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/login",
		ExpectedStatusCode: 200,
		Description:        "一般ユーザでログインできること",
		PostJSON: map[string]interface{}{
			"login_name": user.LoginName,
			"password":   user.Password,
		},
		CheckFunc: checkJsonUserResponse(user),
	})
	if err != nil {
		return err
	}

	user.Status.Online = true
	return nil
}

func logoutAppUser(ctx context.Context, checker *Checker, user *AppUser) error {
	if !user.Status.Online {
		return nil
	}

	err := checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/logout",
		ExpectedStatusCode: 204,
		Description:        "一般ユーザでログアウトできること",
	})
	if err != nil {
		return err
	}

	user.Status.Online = false
	return nil
}

func popOrCreateEventSheet(ctx context.Context, state *State) (*EventSheet, func(), error) {
	eventSheet, eventSheetPush := state.PopEventSheet()
	if eventSheet != nil {
		return eventSheet, eventSheetPush, nil
	}

	// Create a new event if no sheet is available

	ok := state.newEventMtx.TryLock()
	if ok {
		defer state.newEventMtx.Unlock()
	} else {
		log.Println("debug: Somebody else is trying to create a new event. Exit.")
		// NOTE: We immediately return rather than waiting somebody else finishes to create a new event
		// because probably the waiting strategy makes benchmarker work faster.
		return nil, nil, nil
	}

	admin, adminChecker, adminPush := state.PopRandomAdministrator()
	if admin == nil {
		return nil, nil, nil
	}
	defer adminPush()

	err := loginAdministrator(ctx, adminChecker, admin)
	if err != nil {
		return nil, nil, err
	}

	event, newEventPush := state.CreateNewEvent()
	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/admin/api/events",
		ExpectedStatusCode: 200,
		Description:        "管理者がイベントを作成できること",
		PostJSON:           eventPostJSON(event),
		CheckFunc:          checkJsonFullEventCreateResponse(event),
	})
	if err != nil {
		return nil, nil, err
	}
	newEventPush("popOrCreateEventSheet")

	eventSheet, eventSheetPush = state.PopEventSheet()
	return eventSheet, eventSheetPush, nil
}

func checkJsonReservationResponse(reserved *JsonReservation) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		bytes := body.Bytes()
		dec := json.NewDecoder(body)
		resReserved := JsonReservation{}
		err := dec.Decode(&resReserved)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %s %v", string(bytes), err)
		}
		if resReserved.SheetRank != reserved.SheetRank {
			return fatalErrorf("正しい予約情報を取得できません")
		}
		// Set reserved ID and Sheet Number from response
		reserved.ReservationID = resReserved.ReservationID
		reserved.SheetNum = resReserved.SheetNum
		return nil
	}
}

func reserveSheet(ctx context.Context, state *State, checker *Checker, user *AppUser, eventSheet *EventSheet) (*Reservation, error) {
	eventID := eventSheet.EventID
	rank := eventSheet.Rank

	reserved := &JsonReservation{ReservationID: 0, SheetRank: rank, SheetNum: 0}
	reservation := &Reservation{ID: 0, EventID: eventID, UserID: user.ID, SheetRank: rank, Price: eventSheet.Price, SheetNum: 0}
	logID := state.BeginReservation(user, reservation)

	err := checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               fmt.Sprintf("/api/events/%d/actions/reserve", eventID),
		ExpectedStatusCode: 202,
		Description:        "席の予約ができること",
		PostJSON: map[string]interface{}{
			"sheet_rank": rank,
		},
		CheckFunc: checkJsonReservationResponse(reserved),
	})
	if err != nil {
		user.Status.PositiveTotalPrice += eventSheet.Price
		return nil, err
	}

	reservation.ID = reserved.ReservationID
	reservation.SheetNum = reserved.SheetNum
	err = state.CommitReservation(logID, user, reservation)
	if err != nil {
		return nil, err
	}
	eventSheet.Num = reserved.SheetNum

	log.Printf("debug: reserve userID:%d(total-price:%s) eventID:%d reservedID:%d(%s-%d) price:%d\n", user.ID, user.Status.TotalPriceString(), eventID, reserved.ReservationID, reserved.SheetRank, reserved.SheetNum, eventSheet.Price)
	return reservation, nil
}

func cancelSheet(ctx context.Context, state *State, checker *Checker, user *AppUser, eventSheet *EventSheet, reservation *Reservation) (already_locked bool, err error) {
	// If somebody is canceling, nobody else should not cancel because, otherwise, double cancelation occurs.
	// To achieve it, we use trylock instead of mutex.Lock()
	mtx := reservation.CancelMtx()
	ok := mtx.TryLock()
	if ok {
		defer mtx.Unlock()
	} else {
		log.Printf("debug: reservation:%d is already locked to cancel\n", reservation.ID)
		return true, nil
	}

	eventID := reservation.EventID
	rank := reservation.SheetRank
	sheetNum := reservation.SheetNum

	logID := state.BeginCancelation(user, reservation)

	err = checker.Play(ctx, &CheckAction{
		Method:             "DELETE",
		Path:               fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, rank, sheetNum),
		ExpectedStatusCode: 204,
		Description:        "キャンセルができること",
	})
	if err != nil {
		return false, err
	}

	state.CommitCancelation(logID, user, reservation)
	eventSheet.Num = NonReservedNum

	return false, nil
}
