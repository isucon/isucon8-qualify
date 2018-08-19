package main

import (
	"bench"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"go/format"
	"hash"
	"hash/crc32"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	htmldigest "github.com/karupanerura/go-html-digest"
	"golang.org/x/net/html"
)

var (
	benchDir  string
	staticDir string
	baseURL   string
)

func init() {
	flag.StringVar(&benchDir, "benchdir", "./src/bench", "path to bench/src/bench directory")
	flag.StringVar(&staticDir, "staticdir", "../webapp/static", "path to webapp/static directory")
	flag.StringVar(&baseURL, "baseurl", "http://localhost:8080/", "path to base url")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

type hasherTransport struct {
	targetHost string
	t          http.RoundTripper
}

func (ct *hasherTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	defer func() {
		req.URL.Host = host
	}()

	req.URL.Host = ct.targetHost
	res, err := ct.t.RoundTrip(req)
	return res, err
}

type TemplateArg struct {
	StaticFiles       []*StaticFile
	ExpectedIndexHash uint32
	ExpectedAdminHash uint32
}

type StaticFile struct {
	Path string
	Size int64
	Hash string
}

const staticFileTemplate = `
package bench

type StaticFile struct {
	Path string
	Size int64
	Hash string
}

var (
	StaticFiles = []*StaticFile {
{{ range .StaticFiles }} &StaticFile { "{{ .Path }}", {{ .Size }}, "{{ .Hash }}" },
{{ end }}
	}

)

const (
	ExpectedIndexHash = {{ .ExpectedIndexHash }}
	ExpectedAdminHash = {{ .ExpectedAdminHash }}
)
`

func prepareStaticFiles() []*StaticFile {
	var ret []*StaticFile
	err := filepath.Walk(staticDir, func(path string, info os.FileInfo, err error) error {
		must(err)
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".map") {
			return nil
		}

		subPath := path[len(staticDir):]

		f, err := os.Open(path)
		must(err)
		defer f.Close()

		h := md5.New()
		_, err = io.Copy(h, f)
		must(err)

		hash := hex.EncodeToString(h.Sum(nil))

		ret = append(ret, &StaticFile{
			Path: subPath,
			Size: info.Size(),
			Hash: hash,
		})

		return nil
	})
	must(err)

	// canonicalize
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Path < ret[j].Path
	})
	return ret
}

func writeStaticFileGo() {
	const saveName = "staticfile.go"
	files := prepareStaticFiles()

	indexHash, err := getCrc32Sum(baseURL)
	must(err)

	adminHash, err := getCrc32Sum(baseURL + "admin/")
	must(err)

	t := template.Must(template.New(saveName).Parse(staticFileTemplate))

	var buf bytes.Buffer
	t.Execute(&buf, TemplateArg{
		StaticFiles:       files,
		ExpectedIndexHash: indexHash,
		ExpectedAdminHash: adminHash,
	})

	fmt.Print(buf.String())

	data, err := format.Source(buf.Bytes())
	must(err)

	err = ioutil.WriteFile(path.Join(benchDir, saveName), data, 0644)
	must(err)

	log.Println("save", saveName)
}

func getCrc32Sum(rawURL string) (uint32, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, err
	}
	targetHost := u.Host
	u.Host = bench.TorbAppHost

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}

	client := &http.Client{
		Transport: &hasherTransport{
			targetHost: targetHost,
			t:          http.DefaultTransport,
		},
	}

	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	rootNode, err := html.Parse(res.Body)
	if err != nil {
		return 0, err
	}

	h := htmldigest.NewHash(func() hash.Hash {
		return crc32.NewIEEE()
	})

	crcSum, err := h.Sum(rootNode)
	if err != nil {
		return 0, err
	}

	crcSum32 := bench.JoinCrc32(crcSum)
	return crcSum32, nil
}

func main() {
	flag.Parse()
	staticDir, err := filepath.Abs(staticDir)
	must(err)

	if !strings.HasSuffix(staticDir, "/static") {
		log.Fatalln("invalid static dir path")
	}

	benchDir, err := filepath.Abs(benchDir)
	must(err)

	if !strings.HasSuffix(benchDir, "src/bench") {
		log.Fatalln("invalid benchdir path")
	}

	writeStaticFileGo()
}
