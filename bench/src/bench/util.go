package bench

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"sync"
)

func assert(flag bool, msgs ...interface{}) {
	if !flag {
		panic("assertion failed: " + fmt.Sprint(msgs...))
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func trim(s string) string {
	return strings.TrimSpace(s)
}

func reverseUintSlice(s []uint) {
	for l, r := 0, len(s)-1; l < r; l, r = l+1, r-1 {
		s[l], s[r] = s[r], s[l]
	}
}

var alphabet = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandomAlphabetString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(b)
}

var bytesBufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func GetBuffer() *bytes.Buffer {
	return bytesBufferPool.Get().(*bytes.Buffer)
}

func PutBuffer(buf *bytes.Buffer) {
	buf.Reset()
	bytesBufferPool.Put(buf)
}

func JoinCrc32(crcSum []byte) uint32 {
	return uint32(crcSum[0])<<24 | uint32(crcSum[1])<<16 | uint32(crcSum[2])<<8 | uint32(crcSum[3])
}
