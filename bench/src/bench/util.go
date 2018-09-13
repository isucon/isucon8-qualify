package bench

import (
	"bytes"
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"sync"
)

func assert(flag bool, msgs ...interface{}) {
	if !flag {
		_, filename, line, _ := runtime.Caller(1)
		msgs = append(msgs, filename, " ", line)
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
