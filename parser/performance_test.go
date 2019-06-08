// Copyright 2012 The Go Authors. All rights reserved.
// Modified work copyright 2018 Alex Browne. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package parser

import (
	"io/ioutil"
	"testing"

	"github.com/qProust/fo/token"
)

var src = readFile("parser.go")

func readFile(filename string) []byte {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	return data
}

func BenchmarkParse(b *testing.B) {
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		if _, err := ParseFile(token.NewFileSet(), "", src, ParseComments); err != nil {
			b.Fatalf("benchmark failed due to parse error: %s", err)
		}
	}
}
