# go-html-digest

Make HTML DOM structure digest.

```go
package main

import (
	"fmt"
	"hash"
	"hash/crc32"
	"net/http"

	htmldigest "github.com/karupanerura/go-html-digest"
	"golang.org/x/net/html"
)

func main() {
	crcSum32, err := getCrc32Sum("https://example.com/")
	if err != nil {
		panic(err)
	}

	fmt.Printf("crc32=%d\n", crcSum32) // crc32=885359194
}

func getCrc32Sum(url string) (uint32, error) {
	res, err := http.Get(url)
	if err != nil {
		return 0, err
	}

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

	crcSum32 := uint32(crcSum[0])<<24 | uint32(crcSum[1])<<16 | uint32(crcSum[2])<<8 | uint32(crcSum[3])
	return crcSum32, nil
}
```
