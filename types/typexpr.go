// Copyright 2013 The Go Authors. All rights reserved.
// Modified work copyright 2018 Alex Browne. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements type-checking of identifiers and type expressions.

package types

import (
	"sort"
	"strconv"

	"github.com/qProust/fo/ast"
	"github.com/qProust/fo/constant"
	"github.com/qProust/fo/token"
)

// ident type-checks identifier e and initializes x with the value or type of e.
// If an error occurred, x.mode is set to invalid.
// For the meaning of def and path, see check.typ, below.
//
func (check *Checker) ident(x *operand, e *ast.Ident, def *Named, path []*TypeName) {
	x.mode = invalid
	x.expr = e

	scope, obj := check.scope.LookupParent(e.Name, check.pos)
	if obj == nil {
		if e.Name == "_" {
			check.errorf(e.Pos(), "cannot use _ as value or type")
		} else {
			check.errorf(e.Pos(), "undeclared name: %s", e.Name)
		}
		return
	}
	check.recordUse(e, obj)

	check.objDecl(obj, def, path)
	typ := obj.Type()
	assert(typ != nil)

	// The object may be dot-imported: If so, remove its package from
	// the map of unused dot imports for the respective file scope.
	// (This code is only needed for dot-imports. Without them,
	// we only have to mark variables, see *Var case below).
	if pkg := obj.Pkg(); pkg != check.pkg && pkg != nil {
		delete(check.unusedDotImports[scope], pkg)
	}

	switch obj := obj.(type) {
	case *PkgName:
		check.errorf(e.Pos(), "use of package %s not in selector", obj.name)
		return

	case *Const:
		check.addDeclDep(obj)
		if typ == Typ[Invalid] {
			return
		}
		if obj == universeIota {
			if check.iota == nil {
				check.errorf(e.Pos(), "cannot use iota outside constant declaration")
				return
			}
			x.val = check.iota
		} else {
			x.val = obj.val
		}
		assert(x.val != nil)
		x.mode = constant_

	case *TypeName:
		x.mode = typexpr
		// check for cycle
		// (it's ok to iterate forward because each named type appears at most once in path)
		for i, prev := range path {
			if prev == obj {
				check.errorf(obj.pos, "illegal cycle in declaration of %s", obj.name)
				// print cycle
				for _, obj := range path[i:] {
					check.errorf(obj.Pos(), "\t%s refers to", obj.Name()) // secondary error, \t indented
				}
				check.errorf(obj.Pos(), "\t%s", obj.Name())
				// maintain x.mode == typexpr despite error
				typ = Typ[Invalid]
				break
			}
		}

	case *Var:
		// It's ok to mark non-local variables, but ignore variables
		// from other packages to avoid potential race conditions with
		// dot-imported variables.
		if obj.pkg == check.pkg {
			obj.used = true
		}
		check.addDeclDep(obj)
		if typ == Typ[Invalid] {
			return
		}
		x.mode = variable

	case *Func:
		check.addDeclDep(obj)
		x.mode = value

	case *Builtin:
		x.id = obj.id
		x.mode = builtin

	case *Nil:
		x.mode = value

	default:
		unreachable()
	}

	x.typ = typ
}

// typExpr type-checks the type expression e and returns its type, or Typ[Invalid].
// If def != nil, e is the type specification for the named type def, declared
// in a type declaration, and def.underlying will be set to the type of e before
// any components of e are type-checked. Path contains the path of named types
// referring to this type.
//
func (check *Checker) typExpr(e ast.Expr, def *Named, path []*TypeName) (T Type) {
	if trace {
		check.trace(e.Pos(), "%s", e)
		check.indent++
		defer func() {
			check.indent--
			check.trace(e.Pos(), "=> %s", T)
		}()
	}

	T = check.typExprInternal(e, def, path)
	assert(isTyped(T))
	check.recordTypeAndValue(e, typexpr, T, nil)

	return
}

func (check *Checker) typ(e ast.Expr) Type {
	return check.typExpr(e, nil, nil)
}

// funcType type-checks a function or method type.
func (check *Checker) funcType(sig *Signature, recvPar *ast.FieldList, ftyp *ast.FuncType) {
	scope := NewScope(check.scope, token.NoPos, token.NoPos, "function")
	scope.isFunc = true
	check.recordScope(ftyp, scope)

	recv := check.methodReceiver(scope, recvPar)
	params, variadic := check.collectParams(scope, ftyp.Params, true)
	results, _ := check.collectParams(scope, ftyp.Results, false)

	if recvPar != nil {
		// spec: "The receiver type must be of the form T or *T where T is a type name."
		// (ignore invalid types - error was reported before)
		if t, _ := deref(recv.typ); t != Typ[Invalid] {
			check.recvType(recv, t)
		}
		sig.recv = recv
	}

	sig.scope = scope
	sig.params = NewTuple(params...)
	sig.results = NewTuple(results...)
	sig.variadic = variadic
}

// genericFuncType type-checks a generic function or method type.
func (check *Checker) genericFuncType(sig *GenericSignature, recvPar *ast.FieldList, ftyp *ast.FuncType, tpList *ast.TypeParamDecl) {
	var typeParams []*TypeParam

	// Add receiver type parameters to scope (if any)
	recvTypeParams, tpScope := check.recvTypeParams(recvPar)

	// Add other type parameters to scope (if any)
	if tpScope == nil {
		tpScope = NewScope(check.scope, check.scope.Pos(), check.scope.End(), "function type parameters")
	}
	if tpList != nil {
		for _, ident := range tpList.Names {
			tp := NewTypeParam(ident.Name)
			typeParams = append(typeParams, tp)
			obj := NewTypeName(ident.Pos(), check.pkg, ident.Name, tp)
			scopePos := ident.Pos()
			check.declare(tpScope, ident, obj, scopePos)
		}
	}

	// Set check.scope to the type parameter scope (and unset it when we return)
	origScope := check.scope
	check.scope = tpScope
	defer func() {
		check.scope = origScope
	}()

	scope := NewScope(check.scope, token.NoPos, token.NoPos, "function")
	scope.isFunc = true
	check.recordScope(ftyp, scope)

	recv := check.methodReceiver(scope, recvPar)
	params, variadic := check.collectParams(scope, ftyp.Params, true)
	results, _ := check.collectParams(scope, ftyp.Results, false)

	if recvPar != nil {
		// spec: "The receiver type must be of the form T or *T where T is a type name."
		// (ignore invalid types - error was reported before)
		if t, _ := deref(recv.typ); t != Typ[Invalid] {
			check.recvType(recv, t)
		}
		sig.recv = recv
	}

	sig.scope = scope
	sig.params = NewTuple(params...)
	sig.results = NewTuple(results...)
	sig.variadic = variadic
	if tpList != nil {
		sig.typeParams = typeParams
	}
	sig.recvTypeParams = recvTypeParams
}

func (check *Checker) recvType(recv *Var, typ Type) {
	var err string
	if T, ok := typ.(BaseNamed); ok {
		// spec: "The type denoted by T is called the receiver base type; it must not
		// be a pointer or interface type and it must be declared in the same package
		// as the method."
		if T.Obj().pkg != check.pkg {
			err = "type not defined in this package"
		} else {
			// TODO(gri) This is not correct if the underlying type is unknown yet.
			switch u := T.Underlying().(type) {
			case *Basic:
				// unsafe.Pointer is treated like a regular pointer
				if u.kind == UnsafePointer {
					err = "unsafe.Pointer"
				}
			case *Pointer, *Interface:
				err = "pointer or interface type"
			}
		}
	} else {
		err = "basic or unnamed type"
	}
	if err != "" {
		check.errorf(recv.pos, "invalid receiver %s (%s)", recv.typ, err)
		// ok to continue
	}
}

// typExprInternal drives type checking of types.
// Must only be called by typExpr.
//
func (check *Checker) typExprInternal(e ast.Expr, def *Named, path []*TypeName) Type {
	switch e := e.(type) {
	case *ast.BadExpr:
		// ignore - error reported before

	case *ast.Ident:
		var x operand
		check.ident(&x, e, def, path)

		switch x.mode {
		case typexpr:
			typ := x.typ
			def.setUnderlying(typ)
			return typ
		case invalid:
			// ignore - error reported before
		case novalue:
			check.errorf(x.pos(), "%s used as type", &x)
		default:
			check.errorf(x.pos(), "%s is not a type", &x)
		}

	case *ast.SelectorExpr:
		var x operand
		check.selector(&x, e)

		switch x.mode {
		case typexpr:
			typ := x.typ
			def.setUnderlying(typ)
			return typ
		case invalid:
			// ignore - error reported before
		case novalue:
			check.errorf(x.pos(), "%s used as type", &x)
		default:
			check.errorf(x.pos(), "%s is not a type", &x)
		}

	case *ast.ParenExpr:
		return check.typExpr(e.X, def, path)

	case *ast.ArrayType:
		if e.Len != nil {
			typ := new(Array)
			def.setUnderlying(typ)
			typ.len = check.arrayLength(e.Len)
			typ.elem = check.typExpr(e.Elt, nil, path)
			check.typeArgsRequired(e.Elt.Pos(), typ.elem)
			return typ
		} else {
			typ := new(Slice)
			def.setUnderlying(typ)
			typ.elem = check.typ(e.Elt)
			check.typeArgsRequired(e.Elt.Pos(), typ.elem)
			return typ
		}

	case *ast.TypeArgExpr:
		typ := check.typExpr(e.X, nil, path)
		genType, ok := typ.(GenericType)
		if !ok {
			check.errorf(e.Pos(), "type arguments provided for non-generic type %s", typ)
		} else {
			concreteType := check.concreteType(e, genType)
			def.setUnderlying(concreteType)
			return concreteType
		}

	case *ast.StructType:
		typ := new(Struct)
		def.setUnderlying(typ)
		check.structType(typ, e, path)
		return typ

	case *ast.StarExpr:
		typ := new(Pointer)
		def.setUnderlying(typ)
		typ.base = check.typ(e.X)
		check.typeArgsRequired(e.X.Pos(), typ.base)
		return typ

	case *ast.FuncType:
		typ := new(Signature)
		def.setUnderlying(typ)
		check.funcType(typ, nil, e)
		return typ

	case *ast.InterfaceType:
		typ := new(Interface)
		def.setUnderlying(typ)
		check.interfaceType(typ, e, def, path)
		return typ

	case *ast.MapType:
		typ := new(Map)
		def.setUnderlying(typ)

		typ.key = check.typ(e.Key)
		check.typeArgsRequired(e.Key.Pos(), typ.key)
		typ.elem = check.typ(e.Value)
		check.typeArgsRequired(e.Value.Pos(), typ.elem)

		// spec: "The comparison operators == and != must be fully defined
		// for operands of the key type; thus the key type must not be a
		// function, map, or slice."
		//
		// Delay this check because it requires fully setup types;
		// it is safe to continue in any case (was issue 6667).
		check.delay(func() {
			if !Comparable(typ.key) {
				check.errorf(e.Key.Pos(), "invalid map key type %s", typ.key)
			}
		})

		return typ

	case *ast.ChanType:
		typ := new(Chan)
		def.setUnderlying(typ)

		dir := SendRecv
		switch e.Dir {
		case ast.SEND | ast.RECV:
			// nothing to do
		case ast.SEND:
			dir = SendOnly
		case ast.RECV:
			dir = RecvOnly
		default:
			check.invalidAST(e.Pos(), "unknown channel direction %d", e.Dir)
			// ok to continue
		}

		typ.dir = dir
		typ.elem = check.typ(e.Value)
		check.typeArgsRequired(e.Value.Pos(), typ.elem)
		return typ

	default:
		check.errorf(e.Pos(), "%s is not a type", e)
	}

	typ := Typ[Invalid]
	def.setUnderlying(typ)
	return typ
}

// typeOrNil type-checks the type expression (or nil value) e
// and returns the typ of e, or nil.
// If e is neither a type nor nil, typOrNil returns Typ[Invalid].
//
func (check *Checker) typOrNil(e ast.Expr) Type {
	var x operand
	check.rawExpr(&x, e, nil)
	switch x.mode {
	case invalid:
		// ignore - error reported before
	case novalue:
		check.errorf(x.pos(), "%s used as type", &x)
	case typexpr:
		return x.typ
	case value:
		if x.isNil() {
			return nil
		}
		fallthrough
	default:
		check.errorf(x.pos(), "%s is not a type", &x)
	}
	return Typ[Invalid]
}

func (check *Checker) arrayLength(e ast.Expr) int64 {
	var x operand
	check.expr(&x, e)
	if x.mode != constant_ {
		if x.mode != invalid {
			check.errorf(x.pos(), "array length %s must be constant", &x)
		}
		return 0
	}
	if isUntyped(x.typ) || isInteger(x.typ) {
		if val := constant.ToInt(x.val); val.Kind() == constant.Int {
			if representableConst(val, check.conf, Typ[Int], nil) {
				if n, ok := constant.Int64Val(val); ok && n >= 0 {
					return n
				}
				check.errorf(x.pos(), "invalid array length %s", &x)
				return 0
			}
		}
	}
	check.errorf(x.pos(), "array length %s must be integer", &x)
	return 0
}

func (check *Checker) recvTypeParams(recvPar *ast.FieldList) ([]*TypeParam, *Scope) {
	if recvPar == nil {
		return nil, nil
	} else if len(recvPar.List) != 1 {
		return nil, nil
	}

	recv := recvPar.List[0]
	var typeParams []*TypeParam
	var tpScope *Scope
	typ := recv.Type
	if x, ok := recv.Type.(*ast.StarExpr); ok {
		typ = x.X
	}
	if x, ok := typ.(*ast.TypeArgExpr); ok {
		tpScope = NewScope(check.scope, check.scope.Pos(), check.scope.End(), "function type parameters")
		for _, expr := range x.Types {
			ident, ok := expr.(*ast.Ident)
			if !ok {
				check.error(expr.Pos(), "type parameters in method receiver must be identifiers")
			}
			// Check if the type parameter is an already defined type.
			if _, foundObj := check.scope.LookupParent(ident.Name, ident.Pos()); foundObj != nil {
				if _, ok := foundObj.Type().(*TypeParam); !ok {
					check.error(ident.Pos(), "type parameters in method receiver cannot be concrete types")
				}
			}
			tp := NewTypeParam(ident.Name)
			typeParams = append(typeParams, tp)
			obj := NewTypeName(ident.Pos(), check.pkg, ident.Name, tp)
			scopePos := ident.Pos()
			check.declare(tpScope, ident, obj, scopePos)
		}
	}

	return typeParams, tpScope
}

func (check *Checker) methodReceiver(scope *Scope, list *ast.FieldList) *Var {
	if list == nil {
		return nil
	}

	var recv *ast.Field
	switch len(list.List) {
	case 0:
		check.error(list.Pos(), "method is missing receiver")
		return NewParam(0, nil, "", Typ[Invalid]) // ignore recv below
	default:
		// more than one receiver
		check.error(list.List[len(list.List)-1].Pos(), "method must have exactly one receiver")
		fallthrough // continue with first receiver
	case 1:
		recv = list.List[0]
	}

	ftype := recv.Type
	isStar := false
	switch t := ftype.(type) {
	case *ast.Ellipsis:
		ftype = t.Elt
		check.invalidAST(recv.Pos(), "... not permitted")
		// ignore ... and continue
	case *ast.StarExpr:
		// Special case for StarExpr in receivers. It is okay for the type args to
		// be omitted.
		isStar = true
		ftype = t.X
	}
	typ := check.typ(ftype)
	if isStar {
		// convert back to a pointer receiver type.
		ptrType := new(Pointer)
		ptrType.base = typ
		typ = ptrType
	}

	var recvVar *Var
	switch len(recv.Names) {
	case 0:
		recvVar = NewParam(recv.Type.Pos(), check.pkg, "", typ)
		check.recordImplicit(recv, recvVar)
	default:
		// more than one name
		check.invalidAST(recv.Names[len(recv.Names)-1].Pos(), "method must have exactly one receiver")
		fallthrough // continue with first receiver
	case 1:
		name := recv.Names[0]
		if name.Name == "" {
			check.invalidAST(name.Pos(), "anonymous parameter")
			// ok to continue
		}
		recvVar = NewParam(name.Pos(), check.pkg, name.Name, typ)
		check.declare(scope, name, recvVar, scope.pos)
	}

	return recvVar
}

func (check *Checker) collectParams(scope *Scope, list *ast.FieldList, variadicOk bool) (params []*Var, variadic bool) {
	if list == nil {
		return
	}

	var named, anonymous bool
	for i, field := range list.List {
		ftype := field.Type
		if t, _ := ftype.(*ast.Ellipsis); t != nil {
			ftype = t.Elt
			if variadicOk && i == len(list.List)-1 {
				variadic = true
			} else {
				check.invalidAST(field.Pos(), "... not permitted")
				// ignore ... and continue
			}
		}
		typ := check.typ(ftype)
		check.typeArgsRequired(ftype.Pos(), typ)
		// The parser ensures that f.Tag is nil and we don't
		// care if a constructed AST contains a non-nil tag.
		if len(field.Names) > 0 {
			// named parameter
			for _, name := range field.Names {
				if name.Name == "" {
					check.invalidAST(name.Pos(), "anonymous parameter")
					// ok to continue
				}
				par := NewParam(name.Pos(), check.pkg, name.Name, typ)
				check.declare(scope, name, par, scope.pos)
				params = append(params, par)
			}
			named = true
		} else {
			// anonymous parameter
			par := NewParam(ftype.Pos(), check.pkg, "", typ)
			check.recordImplicit(field, par)
			params = append(params, par)
			anonymous = true
		}
	}

	if named && anonymous {
		check.invalidAST(list.Pos(), "list contains both named and anonymous parameters")
		// ok to continue
	}

	// For a variadic function, change the last parameter's type from T to []T.
	if variadic && len(params) > 0 {
		last := params[len(params)-1]
		last.typ = &Slice{elem: last.typ}
	}

	return
}

func (check *Checker) declareInSet(oset *objset, pos token.Pos, obj Object) bool {
	if alt := oset.insert(obj); alt != nil {
		check.errorf(pos, "%s redeclared", obj.Name())
		check.reportAltDecl(alt)
		return false
	}
	return true
}

func (check *Checker) interfaceType(iface *Interface, ityp *ast.InterfaceType, def *Named, path []*TypeName) {
	// empty interface: common case
	if ityp.Methods == nil {
		return
	}

	// The parser ensures that field tags are nil and we don't
	// care if a constructed AST contains non-nil tags.

	// use named receiver type if available (for better error messages)
	var recvTyp Type = iface
	if def != nil {
		recvTyp = def
	}

	// Phase 1: Collect explicitly declared methods, the corresponding
	//          signature (AST) expressions, and the list of embedded
	//          type (AST) expressions. Do not resolve signatures or
	//          embedded types yet to avoid cycles referring to this
	//          interface.

	var (
		mset       objset
		signatures []ast.Expr // list of corresponding method signatures
		embedded   []ast.Expr // list of embedded types
	)
	for _, f := range ityp.Methods.List {
		if len(f.Names) > 0 {
			// The parser ensures that there's only one method
			// and we don't care if a constructed AST has more.
			name := f.Names[0]
			pos := name.Pos()
			// spec: "As with all method sets, in an interface type,
			// each method must have a unique non-blank name."
			if name.Name == "_" {
				check.errorf(pos, "invalid method name _")
				continue
			}
			// Don't type-check signature yet - use an
			// empty signature now and update it later.
			// Since we know the receiver, set it up now
			// (required to avoid crash in ptrRecv; see
			// e.g. test case for issue 6638).
			// TODO(gri) Consider marking methods signatures
			// as incomplete, for better error messages. See
			// also the T4 and T5 tests in testdata/cycles2.src.
			sig := new(Signature)
			sig.recv = NewVar(pos, check.pkg, "", recvTyp)
			m := NewFunc(pos, check.pkg, name.Name, sig)
			if check.declareInSet(&mset, pos, m) {
				iface.methods = append(iface.methods, m)
				iface.allMethods = append(iface.allMethods, m)
				signatures = append(signatures, f.Type)
				check.recordDef(name, m)
			}
		} else {
			// embedded type
			embedded = append(embedded, f.Type)
		}
	}

	// Phase 2: Resolve embedded interfaces. Because an interface must not
	//          embed itself (directly or indirectly), each embedded interface
	//          can be fully resolved without depending on any method of this
	//          interface (if there is a cycle or another error, the embedded
	//          type resolves to an invalid type and is ignored).
	//          In particular, the list of methods for each embedded interface
	//          must be complete (it cannot depend on this interface), and so
	//          those methods can be added to the list of all methods of this
	//          interface.

	for _, e := range embedded {
		pos := e.Pos()
		typ := check.typExpr(e, nil, path)
		check.typeArgsRequired(e.Pos(), typ)
		// Determine underlying embedded (possibly incomplete) type
		// by following its forward chain.
		named, _ := typ.(*Named)
		under := underlying(named)
		embed, _ := under.(*Interface)
		if embed == nil {
			if typ != Typ[Invalid] {
				check.errorf(pos, "%s is not an interface", typ)
			}
			continue
		}
		iface.embeddeds = append(iface.embeddeds, named)
		// collect embedded methods
		if embed.allMethods == nil {
			check.errorf(pos, "internal error: incomplete embedded interface %s (issue #18395)", named)
		}
		for _, m := range embed.allMethods {
			if check.declareInSet(&mset, pos, m) {
				iface.allMethods = append(iface.allMethods, m)
			}
		}
	}

	// Phase 3: At this point all methods have been collected for this interface.
	//          It is now safe to type-check the signatures of all explicitly
	//          declared methods, even if they refer to this interface via a cycle
	//          and embed the methods of this interface in a parameter of interface
	//          type.

	for i, m := range iface.methods {
		expr := signatures[i]
		typ := check.typ(expr)
		check.typeArgsRequired(expr.Pos(), typ)
		sig, _ := typ.(*Signature)
		if sig == nil {
			if typ != Typ[Invalid] {
				check.invalidAST(expr.Pos(), "%s is not a method signature", typ)
			}
			continue // keep method with empty method signature
		}
		// update signature, but keep recv that was set up before
		old := m.typ.(*Signature)
		sig.recv = old.recv
		*old = *sig // update signature (don't replace it!)
	}

	// TODO(gri) The list of explicit methods is only sorted for now to
	// produce the same Interface as NewInterface. We may be able to
	// claim source order in the future. Revisit.
	sort.Sort(byUniqueMethodName(iface.methods))

	// TODO(gri) The list of embedded types is only sorted for now to
	// produce the same Interface as NewInterface. We may be able to
	// claim source order in the future. Revisit.
	sort.Sort(byUniqueTypeName(iface.embeddeds))

	if iface.allMethods == nil {
		iface.allMethods = make([]*Func, 0) // mark interface as complete
	} else {
		sort.Sort(byUniqueMethodName(iface.allMethods))
	}
}

// byUniqueTypeName named type lists can be sorted by their unique type names.
type byUniqueTypeName []Type

func (a byUniqueTypeName) Len() int           { return len(a) }
func (a byUniqueTypeName) Less(i, j int) bool { return sortName(a[i]) < sortName(a[j]) }
func (a byUniqueTypeName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func sortName(t Type) string {
	if named, _ := t.(*Named); named != nil {
		return named.obj.Id()
	}
	return ""
}

// byUniqueMethodName method lists can be sorted by their unique method names.
type byUniqueMethodName []*Func

func (a byUniqueMethodName) Len() int           { return len(a) }
func (a byUniqueMethodName) Less(i, j int) bool { return a[i].Id() < a[j].Id() }
func (a byUniqueMethodName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func (check *Checker) tag(t *ast.BasicLit) string {
	if t != nil {
		if t.Kind == token.STRING {
			if val, err := strconv.Unquote(t.Value); err == nil {
				return val
			}
		}
		check.invalidAST(t.Pos(), "incorrect tag syntax: %q", t.Value)
	}
	return ""
}

func (check *Checker) structType(styp *Struct, e *ast.StructType, path []*TypeName) {
	list := e.Fields
	if list == nil {
		return
	}

	// struct fields and tags
	var fields []*Var
	var tags []string

	// for double-declaration checks
	var fset objset

	// current field typ and tag
	var typ Type
	var tag string
	add := func(ident *ast.Ident, anonymous bool, pos token.Pos) {
		if tag != "" && tags == nil {
			tags = make([]string, len(fields))
		}
		if tags != nil {
			tags = append(tags, tag)
		}

		name := ident.Name
		fld := NewField(pos, check.pkg, name, typ, anonymous)
		// spec: "Within a struct, non-blank field names must be unique."
		if name == "_" || check.declareInSet(&fset, pos, fld) {
			fields = append(fields, fld)
			check.recordDef(ident, fld)
		}
	}

	for _, f := range list.List {
		typ = check.typExpr(f.Type, nil, path)
		check.typeArgsRequired(f.Type.Pos(), typ)
		tag = check.tag(f.Tag)
		if len(f.Names) > 0 {
			// named fields
			for _, name := range f.Names {
				add(name, false, name.Pos())
			}
		} else {
			// anonymous field
			// spec: "An embedded type must be specified as a type name T or as a pointer
			// to a non-interface type name *T, and T itself may not be a pointer type."
			pos := f.Type.Pos()
			name := anonymousFieldIdent(f.Type)
			if name == nil {
				check.invalidAST(pos, "anonymous field type %s has no name", f.Type)
				continue
			}
			t, isPtr := deref(typ)
			// Because we have a name, typ must be of the form T or *T, where T is the name
			// of a (named or alias) type, and t (= deref(typ)) must be the type of T.
			switch t := t.Underlying().(type) {
			case *Basic:
				if t == Typ[Invalid] {
					// error was reported before
					continue
				}

				// unsafe.Pointer is treated like a regular pointer
				if t.kind == UnsafePointer {
					check.errorf(pos, "anonymous field type cannot be unsafe.Pointer")
					continue
				}

			case *Pointer:
				check.errorf(pos, "anonymous field type cannot be a pointer")
				continue

			case *Interface:
				if isPtr {
					check.errorf(pos, "anonymous field type cannot be a pointer to an interface")
					continue
				}
			}
			add(name, true, pos)
		}
	}

	styp.fields = fields
	styp.tags = tags
}

func anonymousFieldIdent(e ast.Expr) *ast.Ident {
	switch e := e.(type) {
	case *ast.Ident:
		return e
	case *ast.StarExpr:
		// *T is valid, but **T is not
		if _, ok := e.X.(*ast.StarExpr); !ok {
			return anonymousFieldIdent(e.X)
		}
	case *ast.SelectorExpr:
		return e.Sel
	}
	return nil // invalid anonymous field
}
