package bench

import (
	"bench/counter"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/PuerkitoBio/goquery"
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
		dec := json.NewDecoder(body)
		jsonError := JsonError{}
		err := dec.Decode(&jsonError)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %v", err)
		}
		if jsonError.Error != errorCode {
			return fatalErrorf("正しいエラーコードを取得できません")
		}
		return nil
	}
}

func loadStaticFile(ctx context.Context, checker *Checker, path string) error {
	return checker.Play(ctx, &CheckAction{
		EnableCache:          true,
		SkipIfCacheAvailable: true,

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

func LoadLogin(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	err := loginAppUser(ctx, checker, user)
	if err != nil {
		return err
	}

	err = logoutAppUser(ctx, checker, user)
	if err != nil {
		return err
	}

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
	// checker.ResetCookie()  // may already login, or not

	events := []JsonEvent{}

	err := checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/",
		ExpectedStatusCode: 200,
		Description:        "ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			selection := doc.Find("#app-wrapper")
			if selection == nil || len(selection.Nodes) == 0 {
				return fatalErrorf("app-wrapperが見つかりません")
			}

			node := selection.Nodes[0]
			for _, attr := range node.Attr {
				if attr.Key == "data-events" {
					err := json.Unmarshal([]byte(attr.Val), &events)
					// TODO(sonots): Validate number of remains, total of events?
					// TODO(sonots): Validate number of remains, total of ranked sheets of events?
					if err != nil {
						return fatalErrorf("イベント一覧のJsonデコードに失敗 %v", err)
					}
					return nil
				}
			}

			return fatalErrorf("app-wrapperにdata-eventsがありません")
		}),
	})
	if err != nil {
		return err
	}
	return nil
}

// 席は(rank 内で)ランダムに割り当てられるため、良い席に当たるまで予約連打して、キャンセルする悪質ユーザがいる
func LoadReserveCancelSheet(ctx context.Context, state *State) error {
	eventSheetRank, eventSheetRankPush := state.PopRandomEventSheetRank()
	if eventSheetRank == nil {
		return nil
	}
	defer eventSheetRankPush()
	if eventSheetRank.Remains <= 0 {
		return nil
	}
	eventID := eventSheetRank.EventID
	rank := eventSheetRank.Rank

	user, userChecker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	err := loginAppUser(ctx, userChecker, user)
	if err != nil {
		return err
	}

	reserved := &JsonReserved{0, rank, 0}
	err = userChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               fmt.Sprintf("/api/events/%d/actions/reserve", eventID),
		ExpectedStatusCode: 202,
		Description:        "席の予約ができること",
		PostJSON: map[string]interface{}{
			"sheet_rank": rank,
		},
		CheckFunc: checkJsonReservedResponse(reserved),
	})
	if err != nil {
		return err
	}
	eventSheetRank.Remains--
	eventSheetRank.Reserved[reserved.SheetNum] = true
	state.AppendReservation(eventID, user.ID, reserved)

	err = userChecker.Play(ctx, &CheckAction{
		Method:             "DELETE",
		Path:               fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, reserved.SheetRank, reserved.SheetNum),
		ExpectedStatusCode: 204,
		Description:        "キャンセルができること",
	})
	if err != nil {
		return err
	}
	eventSheetRank.Remains++
	eventSheetRank.Reserved[reserved.SheetNum] = false
	state.DeleteReservation(reserved.ReservationID)

	return nil
}

// 空きがなくなるとベンチを回し続けられなくなるので、残り20%より先は予約しない
var remainsRatioThreshold = 0.2

func LoadReserveSheet(ctx context.Context, state *State) error {
	eventSheetRank, eventSheetRankPush := state.PopRandomEventSheetRank()
	if eventSheetRank == nil {
		return nil
	}
	defer eventSheetRankPush()
	if float64(eventSheetRank.Remains)/float64(eventSheetRank.Total) <= remainsRatioThreshold {
		return nil
	}
	eventID := eventSheetRank.EventID
	rank := eventSheetRank.Rank

	user, userChecker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	err := loginAppUser(ctx, userChecker, user)
	if err != nil {
		return err
	}

	reserved := &JsonReserved{0, rank, 0}
	err = userChecker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               fmt.Sprintf("/api/events/%d/actions/reserve", eventID),
		ExpectedStatusCode: 202,
		Description:        "席の予約ができること",
		PostJSON: map[string]interface{}{
			"sheet_rank": rank,
		},
		CheckFunc: checkJsonReservedResponse(reserved),
	})
	if err != nil {
		return err
	}
	eventSheetRank.Remains--
	eventSheetRank.Reserved[reserved.SheetNum] = true
	state.AppendReservation(eventID, user.ID, reserved)

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
		dec := json.NewDecoder(body)
		jsonUser := JsonUser{}
		err := dec.Decode(&jsonUser)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %v", err)
		}
		if jsonUser.Nickname != user.Nickname {
			return fatalErrorf("正しいユーザ情報を取得できません")
		}
		// Set auto incremented ID from response
		user.ID = jsonUser.ID
		return nil
	}
}

func checkJsonUserResponse(user *AppUser) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		dec := json.NewDecoder(body)
		jsonUser := JsonUser{}
		err := dec.Decode(&jsonUser)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %v", err)
		}
		if jsonUser.ID != user.ID || jsonUser.Nickname != user.Nickname {
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
	// 1. ヘッダー部分の確認
	//   ログイン済みの場合ユーザー名が表示されていること
	//   ログインしていない場合ユーザー名が表示されていないこと
	// 2. DOM 構造が変わっていないこと
	// 3. イベント一覧が一定期間以内に更新されていること
	//   何秒許容するかは要検討
	// 4. イベント一覧の残席数が正しく更新されていること（要検討）
	return nil
}

func checkJsonReservedResponse(reserved *JsonReserved) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		dec := json.NewDecoder(body)
		resReserved := JsonReserved{}
		err := dec.Decode(&resReserved)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %v", err)
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

	eventSheetRank, eventSheetRankPush := state.PopRandomEventSheetRank()
	if eventSheetRank == nil {
		return nil
	}
	defer eventSheetRankPush()
	eventID := eventSheetRank.EventID
	rank := eventSheetRank.Rank

	if eventSheetRank.Remains <= 0 {
		err = userChecker.Play(ctx, &CheckAction{
			Method:             "POST",
			Path:               fmt.Sprintf("/api/events/%d/actions/reserve", eventID),
			ExpectedStatusCode: 409,
			Description:        "売り切れの場合エラーになること",
			CheckFunc:          checkJsonErrorResponse("sold_out"),
			PostJSON: map[string]interface{}{
				"sheet_rank": rank,
			},
		})
		if err != nil {
			return err
		}

	} else {
		reserved := &JsonReserved{0, rank, 0}
		err = userChecker.Play(ctx, &CheckAction{
			Method:             "POST",
			Path:               fmt.Sprintf("/api/events/%d/actions/reserve", eventID),
			ExpectedStatusCode: 202,
			Description:        "席の予約ができること",
			PostJSON: map[string]interface{}{
				"sheet_rank": rank,
			},
			CheckFunc: checkJsonReservedResponse(reserved),
		})
		if err != nil {
			return err
		}
		eventSheetRank.Remains--
		eventSheetRank.Reserved[reserved.SheetNum] = true
		state.AppendReservation(eventID, user.ID, reserved)

		err = userChecker.Play(ctx, &CheckAction{
			Method:             "DELETE",
			Path:               fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, reserved.SheetRank, reserved.SheetNum),
			ExpectedStatusCode: 204,
			Description:        "キャンセルができること",
		})
		if err != nil {
			return err
		}
		eventSheetRank.Remains++
		eventSheetRank.Reserved[reserved.SheetNum] = false
		state.DeleteReservation(reserved.ReservationID)

		err = userChecker.Play(ctx, &CheckAction{
			Method:             "DELETE",
			Path:               fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, reserved.SheetRank, reserved.SheetNum),
			ExpectedStatusCode: 400,
			Description:        "すでにキャンセル済みの場合エラーになること",
			CheckFunc:          checkJsonErrorResponse("not_reserved"),
		})
		if err != nil {
			return err
		}

		// TODO(sonots): Need to find a sheet which somebody else reserved.
		// err := userChecker.Play(ctx, &CheckAction{
		// 	Method:      "DELETE",
		// 	Path:        fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, reserved.SheetRank, reserved.SheetNum),
		// 	ExpectedStatusCode: 403,
		// 	Description: "購入していないチケットをキャンセルしようとするとエラーになること",
		//	CheckFunc:          checkJsonErrorResponse("not_permitted"),
		// })
		// if err != nil {
		// 	return err
		// }
	}

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
		ExpectedStatusCode: 409,
		Description:        "存在しないランクのシートを予約しようとするとエラーになること",
		CheckFunc:          checkJsonErrorResponse("sold_out"), // TOOD(sonots): FIX ME
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

	// TODO(sonots): Randomize, but find ID which does not exist.
	unknownNum := 0
	err = userChecker.Play(ctx, &CheckAction{
		Method:             "DELETE",
		Path:               fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, unknownRank, unknownNum),
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
		dec := json.NewDecoder(body)
		jsonAdmin := JsonAdministrator{}
		err := dec.Decode(&jsonAdmin)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %v", err)
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

func checkJsonAdminEventCreateResponse(event *Event) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		dec := json.NewDecoder(body)
		jsonEvent := JsonAdminEvent{}
		err := dec.Decode(&jsonEvent)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %v", err)
		}
		if jsonEvent.Title != event.Title || jsonEvent.Price != event.Price || jsonEvent.Public != event.PublicFg {
			return fatalErrorf("正しいイベントを取得できません")
		}
		// Set auto incremented ID from response
		event.ID = jsonEvent.ID
		return nil
	}
}

func checkJsonAdminEventResponse(event *Event) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		dec := json.NewDecoder(body)
		jsonEvent := JsonAdminEvent{}
		err := dec.Decode(&jsonEvent)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %v", err)
		}
		if jsonEvent.ID != event.ID || jsonEvent.Title != event.Title || jsonEvent.Price != event.Price || jsonEvent.Public != event.PublicFg {
			return fatalErrorf("正しいイベントを取得できません")
		}
		return nil
	}
}

func checkJsonEventResponse(event *Event) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		dec := json.NewDecoder(body)
		jsonEvent := JsonEvent{}
		err := dec.Decode(&jsonEvent)
		if err != nil {
			return fatalErrorf("Jsonのデコードに失敗 %v", err)
		}
		if jsonEvent.ID != event.ID || jsonEvent.Title != event.Title {
			return fatalErrorf("正しいイベントを取得できません")
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

	event, newEventPush := state.PopNewEvent()
	if event == nil {
		return nil
	}

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
		CheckFunc:          checkJsonAdminEventCreateResponse(event),
	})
	if err != nil {
		return err
	}

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
		CheckFunc:          checkJsonAdminEventResponse(event),
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
		CheckFunc:          checkJsonAdminEventResponse(event),
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/api/events/%d", event.ID),
		ExpectedStatusCode: 200,
		Description:        "公開イベントを取得できること",
		CheckFunc:          checkJsonEventResponse(event),
	})
	if err != nil {
		return err
	}

	err = adminChecker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/admin/api/events/%d", event.ID),
		ExpectedStatusCode: 200,
		Description:        "管理者が公開イベントを取得できること",
		CheckFunc:          checkJsonAdminEventResponse(event),
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

	newEventPush()

	return nil
}

func checkReportResponse(reservations map[uint]*Reservation) func(res *http.Response, body *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		// reservation_id,event_id,user_id,rank,price,sold_at
		// 5,1,830,A,6000,2018-08-02T05:04:07Z
		// 7,1,854,S,8000,2018-08-02T05:04:10Z
		// 8,1,484,A,6000,2018-08-02T05:04:10Z
		// 9,1,377,B,4000,2018-08-02T05:04:12Z

		r := csv.NewReader(body)
		record, err := r.Read()
		if err == io.EOF ||
			len(record) != 6 ||
			record[0] != "reservation_id" ||
			record[1] != "event_id" ||
			record[2] != "user_id" ||
			record[3] != "rank" ||
			record[4] != "price" ||
			record[5] != "sold_at" {
			return fatalErrorf("正しいCSVヘッダを取得できません")
		}

		msg := "正しいレポートを取得できません"
		for {
			record, err := r.Read()
			if err == io.EOF {
				break
			}
			reservationID, err := strconv.Atoi(record[0])
			if err != nil {
				return fatalErrorf(msg)
			}
			eventID, err := strconv.Atoi(record[1])
			if err != nil {
				return fatalErrorf(msg)
			}
			userID, err := strconv.Atoi(record[2])
			if err != nil {
				return fatalErrorf(msg)
			}
			sheetRank := record[3]

			reservation, ok := reservations[uint(reservationID)]
			if !ok {
				// Golang context forcely stops benchmarker if benchDuration is passed.
				// However, some requests would already been issued to webapps, thus,
				// the report would include some reservations which we did not complete and missed.
				// Ignore such reservations.
				continue
			}
			if reservation.ID != uint(reservationID) ||
				reservation.EventID != uint(eventID) ||
				reservation.UserID != uint(userID) ||
				reservation.SheetRank != sheetRank {
				return fatalErrorf(msg)
			}
		}

		// Count also does not match by same reasons that context forcely stops
		// if len(reservations) != count {
		// 	return fatalErrorf(msg)
		// }

		return nil
	}
}

func CheckReport(ctx context.Context, state *State) error {
	admin, checker, push := state.PopRandomAdministrator()
	if admin == nil {
		return nil
	}
	defer push()

	err := loginAdministrator(ctx, checker, admin)
	if err != nil {
		return err
	}

	state.reservationMtx.Lock()
	defer state.reservationMtx.Unlock()

	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/admin/api/reports/sales",
		ExpectedStatusCode: 200,
		Description:        "レポートを正しく取得できること",
		CheckFunc:          checkReportResponse(state.reservations),
	})
	if err != nil {
		return err
	}

	return nil
}

func loginAdministrator(ctx context.Context, checker *Checker, admin *Administrator) error {
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
