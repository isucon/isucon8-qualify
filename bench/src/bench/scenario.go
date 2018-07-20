package bench

import (
	"bench/counter"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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

func LoadSignUp(ctx context.Context, state *State) error {
	user, checker, push := state.PopNewUser()
	if user == nil {
		return nil
	}

	err := checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/users",
		ExpectedStatusCode: 201,
		PostData: map[string]string{
			"nickname":   user.Nickname,
			"login_name": user.LoginName,
			"password":   user.Password,
		},
		Description: "新規ユーザが作成できること",
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/login",
		ExpectedStatusCode: 200,
		PostData: map[string]string{
			"login_name": user.LoginName,
			"password":   user.Password,
		},
		Description: "作成したユーザでログインできること",
	})
	if err != nil {
		return err
	}

	push()

	return nil
}

func LoadSignIn(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	act := &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/login",
		ExpectedStatusCode: 200,
		Description:        "ログインできること",
		PostData: map[string]string{
			"login_name": user.LoginName,
			"password":   user.Password,
		},
	}

	err := checker.Play(ctx, act)
	if err != nil {
		return err
	}

	act = &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/logout",
		ExpectedStatusCode: 204,
		Description:        "ログアウトできること",
	}

	err = checker.Play(ctx, act)
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

// 席は(rank 内で)ランダムに割り当てられるため、良い席に当たるまで予約連打して、キャンセルするユーザがいる
func LoadReserve(ctx context.Context, state *State) error {
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

func CheckLogin(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	err := checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/login",
		ExpectedStatusCode: 200,
		PostData: map[string]string{
			"login_name": user.LoginName,
			"password":   user.Password,
		},
		Description: "存在するユーザでログインできること",
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/logout",
		ExpectedStatusCode: 204,
		Description:        "ログアウトできること",
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/api/actions/login",
		ExpectedStatusCode: 401,
		PostData: map[string]string{
			"login_name": RandomAlphabetString(32),
			"password":   RandomAlphabetString(32),
		},
		Description: "存在しないユーザでログインできないこと",
	})
	if err != nil {
		return err
	}

	return nil
}
