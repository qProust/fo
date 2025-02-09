// Copyright 2015 The Go Authors. All rights reserved.
// Modified work copyright 2018 Alex Browne. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package importer provides access to export data importers.
package importer

import (
	"go/build"
	"io"
	"runtime"

	"github.com/qProust/fo/token"
	"github.com/qProust/fo/types"

	"github.com/qProust/fo/internal/gccgoimporter"
	"github.com/qProust/fo/internal/gcimporter"
	"github.com/qProust/fo/internal/srcimporter"
)

// A Lookup function returns a reader to access package data for
// a given import path, or an error if no matching package is found.
type Lookup func(path string) (io.ReadCloser, error)

// For returns an Importer for importing from installed packages
// for the compilers "gc" and "gccgo", or for importing directly
// from the source if the compiler argument is "source". In this
// latter case, importing may fail under circumstances where the
// exported API is not entirely defined in pure Go source code
// (if the package API depends on cgo-defined entities, the type
// checker won't have access to those).
//
// If lookup is nil, the default package lookup mechanism for the
// given compiler is used, and the resulting importer attempts
// to resolve relative and absolute import paths to canonical
// import path IDs before finding the imported file.
//
// If lookup is non-nil, then the returned importer calls lookup
// each time it needs to resolve an import path. In this mode
// the importer can only be invoked with canonical import paths
// (not relative or absolute ones); it is assumed that the translation
// to canonical import paths is being done by the client of the
// importer.
func For(compiler string, lookup Lookup) types.Importer {
	switch compiler {
	case "gc":
		return &gcimports{
			packages: make(map[string]*types.Package),
			lookup:   lookup,
		}

	case "gccgo":
		var inst gccgoimporter.GccgoInstallation
		if err := inst.InitFromDriver("gccgo"); err != nil {
			return nil
		}
		return &gccgoimports{
			packages: make(map[string]*types.Package),
			importer: inst.GetImporter(nil, nil),
			lookup:   lookup,
		}

	case "source":
		if lookup != nil {
			panic("source importer for custom import path lookup not supported (issue #13847).")
		}

		return srcimporter.New(&build.Default, token.NewFileSet(), make(map[string]*types.Package))
	}

	// compiler not supported
	return nil
}

// Default returns an Importer for the compiler that built the running binary.
// If available, the result implements types.ImporterFrom.
func Default() types.Importer {
	return For(runtime.Compiler, nil)
}

// gc importer

type gcimports struct {
	packages map[string]*types.Package
	lookup   Lookup
}

func (m *gcimports) Import(path string) (*types.Package, error) {
	return m.ImportFrom(path, "" /* no vendoring */, 0)
}

func (m *gcimports) ImportFrom(path, srcDir string, mode types.ImportMode) (*types.Package, error) {
	if mode != 0 {
		panic("mode must be 0")
	}
	return gcimporter.Import(m.packages, path, srcDir, m.lookup)
}

// gccgo importer

type gccgoimports struct {
	packages map[string]*types.Package
	importer gccgoimporter.Importer
	lookup   Lookup
}

func (m *gccgoimports) Import(path string) (*types.Package, error) {
	return m.ImportFrom(path, "" /* no vendoring */, 0)
}

func (m *gccgoimports) ImportFrom(path, srcDir string, mode types.ImportMode) (*types.Package, error) {
	if mode != 0 {
		panic("mode must be 0")
	}
	return m.importer(m.packages, path, srcDir, m.lookup)
}
