package htmldigest

import (
	"errors"
	"hash"
	"io"
	"sort"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

// Hash is digest calculator
type Hash struct {
	HashProvider func() hash.Hash
	IgnoreFunc   func(*html.Node) bool
}

// NewHash creates new digest calculator
func NewHash(provider func() hash.Hash) *Hash {
	return &Hash{
		HashProvider: provider,
	}
}

func nopIgnoreFunc(*html.Node) bool {
	return false
}

// Sum makes new digest for HTML from hasher.
func (h *Hash) Sum(rootNode *html.Node) ([]byte, error) {
	hs := h.HashProvider()
	c := &ctx{
		writer:     hs,
		ignoreFunc: h.IgnoreFunc,
	}
	if c.ignoreFunc == nil {
		c.ignoreFunc = nopIgnoreFunc
	}

	err := c.sum(rootNode)
	if err != nil {
		return nil, err
	}

	d := hs.Sum(nil)
	return d, nil
}

type ctx struct {
	writer     io.Writer
	ignoreFunc func(*html.Node) bool
}

func (cx *ctx) write(s string) error {
	_, err := io.WriteString(cx.writer, s)
	return err
}

func (cx *ctx) sum(n *html.Node) error {
	if cx.ignoreFunc(n) {
		return nil
	}

	switch n.Type {
	case html.ErrorNode:
		return errors.New("htmldigest: cannot digest an ErrorNode node")
	case html.TextNode, html.CommentNode:
		// ignore it
		return nil
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := cx.sum(c); err != nil {
				return err
			}
		}
		return nil
	case html.ElementNode:
		err := cx.write("[" + strings.ToLower(n.Namespace) + ":" + n.DataAtom.String() + "-")
		if err != nil {
			return err
		}

		if len(n.Attr) > 0 {
			err = cx.sumAttr(n.Attr)
			if err != nil {
				return err
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := cx.sum(c); err != nil {
				return err
			}
		}

		return cx.write("]")
	case html.DoctypeNode:
		err := cx.write("[DOCTYPE:" + strings.ToLower(n.Data) + "-")
		if err != nil {
			return err
		}

		err = cx.sumAttrFull(n.Attr)
		if err != nil {
			return err
		}

		return cx.write("]")
	default:
		return errors.New("htmldigest: unknown node type")
	}
}

var (
	keysBufPool = sync.Pool{
		New: func() interface{} {
			return make([]string, 0, 32)
		},
	}
	attrsBufPool = sync.Pool{
		New: func() interface{} {
			return make([]html.Attribute, 0, 32)
		},
	}
)

func (cx *ctx) sumAttr(attrs []html.Attribute) error {
	keys := keysBufPool.Get().([]string)
	defer keysBufPool.Put(keys)
	keys = keys[:0]

	for _, attr := range attrs {
		k := strings.ToLower(attr.Namespace + ":" + attr.Key)
		switch k {
		case ":id", ":name", ":xmlns", "xmlns:r", ":content", ":rel", ":type", ":http-equiv":
			keys = append(keys, k+"("+attr.Val+")")
		case ":class":
			classes := strings.Split(attr.Val, " ")
			sort.Strings(classes)
			keys = append(keys, k+"("+strings.Join(classes, ",")+")")
		case ":href", ":src", ":action":
			keys = append(keys, k+"("+attr.Val+")")
		case ":style", ":title", ":accesskey", ":hidden", ":tabindex":
			// ignore it (because these are UI attrs)
			continue // for loop
		default:
			if strings.HasPrefix(k, ":on") && handlerAtrrs.Replace(k[1:]) == "" {
				// ignore it
				continue // for loop
			}

			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	err := cx.write(strings.Join(keys, ","))
	return err
}

func (cx *ctx) sumAttrFull(srcAttrs []html.Attribute) error {
	attrs := attrsBufPool.Get().([]html.Attribute)
	defer attrsBufPool.Put(attrs)
	if len(srcAttrs) > cap(attrs) {
		attrs = make([]html.Attribute, len(srcAttrs))
	} else {
		attrs = attrs[:len(srcAttrs)]
	}
	copy(attrs, srcAttrs)

	sort.Slice(attrs, func(i, j int) bool {
		return attrs[i].Namespace < attrs[j].Namespace || attrs[i].Key < attrs[j].Key
	})
	for i, attr := range attrs {
		sep := ","
		if i == len(attrs)-1 {
			sep = ""
		}

		k := strings.ToLower(attr.Namespace + ":" + attr.Key)
		switch k {
		case ":class":
			classes := strings.Split(attr.Val, " ")
			sort.Strings(classes)
			err := cx.write(k + "(" + strings.Join(classes, ",") + ")" + sep)
			if err != nil {
				return err
			}
		case ":value":
			// ignore value (because it's not good for)
			err := cx.write(k + sep)
			if err != nil {
				return err
			}
		case ":style", ":title", ":accesskey", ":hidden", ":tabindex":
			// ignore it (because these are UI attrs)
			continue // for loop
		default:
			if strings.HasPrefix(k, ":on") && handlerAtrrs.Replace(k[1:]) == "" {
				// ignore it
				continue // for loop
			}

			err := cx.write(k + "(" + attr.Val + ")" + sep)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
