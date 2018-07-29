package htmldigest

import (
	"bytes"
	"hash"
	"hash/crc32"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

const (
	exampleHTML = `
<!doctype html>
<html>
  <head>
    <title>Example</title>
    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">body {/* dummy */}</style>
  </head>
  <body><!-- this is a comment -->
    <div style="width:100% /* should be ignored */">
      <h1>Example</h1>
      <p>This an example for testing the htmldigest package.</p>
      <p><a href="/path/to/link-to1" onclick="return false // should be ignored">link1</a></p>
      <p><a href="/path/to/link-to2">link2</a></p>
    </div>
  </body>
</html>
`
	expectedExampleSum   = `[DOCTYPE:html-][:html-[:head-[:title-][:meta-:charset][:meta-:content(text/html; charset=utf-8),:http-equiv(Content-type)][:meta-:content(width=device-width, initial-scale=1),:name(viewport)][:style-:type(text/css)]][:body-[:div-[:h1-][:p-][:p-[:a-:href(/path/to/link-to1)]][:p-[:a-:href(/path/to/link-to2)]]]]]`
	expectedExampleSum32 = 993070481
)

func getSumRawText(rootNode *html.Node) (string, error) {
	buf := &bytes.Buffer{}
	c := &ctx{
		writer:     buf,
		ignoreFunc: nopIgnoreFunc,
	}

	err := c.sum(rootNode)
	if err != nil {
		return "", nil
	}

	return buf.String(), nil
}

func getCrc32Sum(rootNode *html.Node) (uint32, error) {
	h := NewHash(func() hash.Hash {
		return crc32.NewIEEE()
	})

	crcSum, err := h.Sum(rootNode)
	if err != nil {
		return 0, err
	}

	crcSum32 := uint32(crcSum[0])<<24 | uint32(crcSum[1])<<16 | uint32(crcSum[2])<<8 | uint32(crcSum[3])
	return crcSum32, nil
}

func TestRawSum(t *testing.T) {
	t.Run("Simple", func(t *testing.T) {
		rootNode, err := html.Parse(strings.NewReader(exampleHTML))
		if err != nil {
			t.Fatal(err)
		}

		rawSum, err := getSumRawText(rootNode)
		if err != nil {
			t.Fatal(err)
		}
		if rawSum != expectedExampleSum {
			t.Errorf("Unexpected sum: %s", rawSum)
		}
	})
	t.Run("Modified", func(t *testing.T) {
		t.Run("NonBroken", func(t *testing.T) {
			r := strings.NewReplacer(`href=`, `HREF=`, `<h1>`, `<H1 title="hello">`)
			rootNode, err := html.Parse(strings.NewReader(r.Replace(exampleHTML)))
			if err != nil {
				t.Fatal(err)
			}

			rawSum, err := getSumRawText(rootNode)
			if err != nil {
				t.Fatal(err)
			}
			if rawSum != expectedExampleSum {
				t.Errorf("Unexpected sum: %s", rawSum)
			}
		})
		t.Run("Broken", func(t *testing.T) {
			r := strings.NewReplacer(`href="/path/to/link-to2"`, `HREF="/path/to/link-to3"`, `<h1>`, `<H2>`)
			rootNode, err := html.Parse(strings.NewReader(r.Replace(exampleHTML)))
			if err != nil {
				t.Fatal(err)
			}

			rawSum, err := getSumRawText(rootNode)
			if err != nil {
				t.Fatal(err)
			}
			if rawSum == expectedExampleSum {
				t.Errorf("Unexpected sum: %s", rawSum)
			}
			t.Logf("raw: %s", rawSum)
		})
	})
}

func TestDigest(t *testing.T) {
	t.Run("Reverse", func(t *testing.T) {
		rootNode, err := html.Parse(strings.NewReader(exampleHTML))
		if err != nil {
			t.Fatal(err)
		}

		rawSum, err := getSumRawText(rootNode)
		if err != nil {
			t.Fatal(err)
		}

		crcSum32, err := getCrc32Sum(rootNode)
		if err != nil {
			t.Fatal(err)
		}

		if recalced := crc32.ChecksumIEEE([]byte(rawSum)); crcSum32 != recalced {
			t.Errorf("Unexpected sum: %d, expected: %d", crcSum32, recalced)
		}
	})
	t.Run("Simple", func(t *testing.T) {
		rootNode, err := html.Parse(strings.NewReader(exampleHTML))
		if err != nil {
			t.Fatal(err)
		}

		crcSum32, err := getCrc32Sum(rootNode)
		if err != nil {
			t.Fatal(err)
		}
		if crcSum32 != expectedExampleSum32 {
			t.Errorf("Unexpected sum: %d", crcSum32)
		}
	})
	t.Run("Modified", func(t *testing.T) {
		t.Run("NonBroken", func(t *testing.T) {
			r := strings.NewReplacer(`href=`, `HREF=`, `<h1>`, `<H1 title="hello">`)
			rootNode, err := html.Parse(strings.NewReader(r.Replace(exampleHTML)))
			if err != nil {
				t.Fatal(err)
			}

			crcSum32, err := getCrc32Sum(rootNode)
			if err != nil {
				t.Fatal(err)
			}
			if crcSum32 != expectedExampleSum32 {
				t.Errorf("Unexpected sum: %d", crcSum32)
			}
		})
		t.Run("Broken", func(t *testing.T) {
			r := strings.NewReplacer(`href="/path/to/link-to2"`, `HREF="/path/to/link-to3"`, `<h1>`, `<H2>`)
			rootNode, err := html.Parse(strings.NewReader(r.Replace(exampleHTML)))
			if err != nil {
				t.Fatal(err)
			}

			crcSum32, err := getCrc32Sum(rootNode)
			if err != nil {
				t.Fatal(err)
			}
			if crcSum32 == expectedExampleSum32 {
				t.Errorf("Unexpected sum: %d", crcSum32)
			}
			t.Logf("crc32: %d", crcSum32)
		})
	})
}
