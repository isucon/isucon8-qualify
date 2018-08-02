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
	"hash"
	"hash/crc32"
	"io"
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

const (
	expectedIndexHash = 497858079
)

func joinCrc32(crcSum []byte) uint32 {
	return uint32(crcSum[0])<<24 | uint32(crcSum[1])<<16 | uint32(crcSum[2])<<8 | uint32(crcSum[3])
}

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

func checkEventsList(state *State, events []JsonEvent) error {
	ok := sort.SliceIsSorted(events, func(i, j int) bool {
		return events[i].ID < events[j].ID
	})
	if !ok {
		return fatalErrorf("イベントの順番が正しくありません")
	}

	expected := FilterPublicEvents(state.GetEvents())
	if len(events) == 0 {
		return fatalErrorf("イベントの数が正しくありません")
	} else if len(events) < len(expected) {
		// 期待する数に満たない場合は1秒以内の誤差は許容する
		var missed []*Event
		for i := len(expected) - 1; i > 0; i-- {
			if expected[i].ID <= events[len(events)-1].ID {
				break
			}
			missed = expected[i:]
		}

		threshold := time.Now().Add(-1 * time.Second)
		for _, e := range missed {
			if e.CreatedAt.Before(threshold) {
				return fatalErrorf("イベントの数が正しくありません")
			}
		}
	} else if len(events) > len(expected) {
		// XXX: 期待する数を超えて返ってきた場合はIDがより新しいものであることを確認して無視する
		for i := len(events) - 1; i > 0; i-- {
			if events[i].ID <= expected[len(expected)-1].ID {
				break
			}
			events = events[:i]
		}
		if len(events) != len(expected) {
			return fatalErrorf("イベントの数が正しくありません")
		}
	}

	for i, e := range events {
		if i == len(expected) {
			break
		}

		if e.ID != expected[i].ID {
			return fatalErrorf("イベントの順番が正しくありません")
		}
		if e.Title != expected[i].Title {
			return fatalErrorf("イベント(id:%d)のタイトルが正しくありません", e.ID)
		}
		if int(e.Total) != len(DataSet.Sheets) {
			return fatalErrorf("イベント(id:%d)の総座席数が正しくありません", e.ID)
		}

		var remains uint
		for _, eventSheetRank := range state.GetEventSheetRanksByEventID(e.ID) {
			if e.Sheets[eventSheetRank.Rank].Total != eventSheetRank.Total {
				return fatalErrorf("イベント(id:%d)の%s席の総座席数が正しくありません", e.ID, eventSheetRank.Rank)
			}
			// TODO(karupa): check remains
			// if e.Sheets[eventSheetRank.Rank].Remains != eventSheetRank.Remains {
			// 	log.Printf("[DEBUG] Event(%d) %s: expected %d but got %d", e.ID, eventSheetRank.Rank, eventSheetRank.Remains, e.Sheets[eventSheetRank.Rank].Remains)
			// 	return fatalErrorf("イベント(id:%d)の%s席の残座席数が正しくありません", e.ID, eventSheetRank.Rank)
			//}
			// TODO(karupa): check price
			// if e.Sheets[eventSheetRank.Rank].Price != eventSheetRank.Price {
			// 	return fatalErrorf("イベント(id:%d)の%s席の価格が正しくありません", e.ID, eventSheetRank.Rank)
			// }
			remains += eventSheetRank.Remains
		}
		// TODO(karupa): check remains
		// if e.Remains != remains {
		// 	return fatalErrorf("イベント(id:%d)の総残座席数が正しくありません", e.ID)
		// }
	}
	return nil
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

// イベントが公開されるのを待ってトップページをF5連打するユーザがいる
// イベント一覧はログインしていてもしていなくても取れる
func LoadTopPage(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

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

	user, userChecker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	err := loginAppUser(ctx, userChecker, user)
	if err != nil {
		return err
	}

	reserved, err := reserveSheet(ctx, state, userChecker, user.ID, eventSheetRank)
	if err != nil {
		return err
	}

	err = cancelSheet(ctx, state, userChecker, user.ID, eventSheetRank, reserved)
	if err != nil {
		return err
	}

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

	user, userChecker, userPush := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer userPush()

	err := loginAppUser(ctx, userChecker, user)
	if err != nil {
		return err
	}

	_, err = reserveSheet(ctx, state, userChecker, user.ID, eventSheetRank)
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
	}

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
			if crcSum32 := joinCrc32(crcSum); crcSum32 != expectedIndexHash {
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
						return fatalErrorf("イベント一覧のJsonデコードに失敗 %v", err)
					}

					err = checkEventsList(state, events)
					if err != nil {
						return err
					}

					found++
				case "data-login-user":
					if user.Status.Online {
						var u *JsonUser
						err := json.Unmarshal([]byte(attr.Val), &u)
						if err != nil {
							return fatalErrorf("ログインユーザーのJsonデコードに失敗 %v", err)
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
		reserved, err := reserveSheet(ctx, state, userChecker, user.ID, eventSheetRank)
		if err != nil {
			return err
		}

		err = cancelSheet(ctx, state, userChecker, user.ID, eventSheetRank, reserved)
		if err != nil {
			return err
		}

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
		// Set created time and auto incremented ID from response
		event.ID = jsonEvent.ID
		event.CreatedAt = time.Now()
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
	event.CreatedAt = time.Now()

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
		count := 0
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
				return fatalErrorf(msg)
			}
			if reservation.ID != uint(reservationID) ||
				reservation.EventID != uint(eventID) ||
				reservation.UserID != uint(userID) ||
				reservation.SheetRank != sheetRank {
				return fatalErrorf(msg)
			}
			count += 1
		}

		if len(reservations) != count {
			return fatalErrorf(msg)
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

	err := loginAdministrator(ctx, checker, admin)
	if err != nil {
		return err
	}

	state.reservationsMtx.Lock()
	defer state.reservationsMtx.Unlock()

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

func reserveSheet(ctx context.Context, state *State, checker *Checker, userID uint, eventSheetRank *EventSheetRank) (*JsonReserved, error) {
	eventID := eventSheetRank.EventID
	rank := eventSheetRank.Rank

	reserved := &JsonReserved{0, rank, 0}
	err := checker.Play(ctx, &CheckAction{
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
		return nil, err
	}
	eventSheetRank.Remains--
	eventSheetRank.Reserved[reserved.SheetNum] = true
	state.AppendReservation(eventID, userID, reserved)

	return reserved, nil
}

func cancelSheet(ctx context.Context, state *State, checker *Checker, userID uint, eventSheetRank *EventSheetRank, reserved *JsonReserved) error {
	eventID := eventSheetRank.EventID
	rank := eventSheetRank.Rank
	reservationID := reserved.ReservationID
	sheetNum := reserved.SheetNum

	err := checker.Play(ctx, &CheckAction{
		Method:             "DELETE",
		Path:               fmt.Sprintf("/api/events/%d/sheets/%s/%d/reservation", eventID, rank, sheetNum),
		ExpectedStatusCode: 204,
		Description:        "キャンセルができること",
	})
	if err != nil {
		return err
	}
	eventSheetRank.Remains++
	eventSheetRank.Reserved[sheetNum] = false
	state.DeleteReservation(reservationID)

	return nil
}
