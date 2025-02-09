// Copyright 2011 The Go Authors. All rights reserved.
// Modified work copyright 2018 Alex Browne. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import "sort"

// A Type represents a type of Go.
// All types implement the Type interface.
type Type interface {
	// Underlying returns the underlying type of a type.
	Underlying() Type

	// String returns a string representation of a type.
	String() string
}

type GenericType interface {
	Type
	TypeParams() []*TypeParam
	Object() Object
}

type ConcreteType interface {
	Type
	GenericType() GenericType
	TypeMap() map[string]Type
}

type PartialGenericType interface {
	Type
	TypeParams() []*TypeParam
	Object() Object
	GenericType() GenericType
	TypeMap() map[string]Type
}

// BasicKind describes the kind of basic type.
type BasicKind int

const (
	Invalid BasicKind = iota // type is invalid

	// predeclared types
	Bool
	Int
	Int8
	Int16
	Int32
	Int64
	Uint
	Uint8
	Uint16
	Uint32
	Uint64
	Uintptr
	Float32
	Float64
	Complex64
	Complex128
	String
	UnsafePointer

	// types for untyped values
	UntypedBool
	UntypedInt
	UntypedRune
	UntypedFloat
	UntypedComplex
	UntypedString
	UntypedNil

	// aliases
	Byte = Uint8
	Rune = Int32
)

// BasicInfo is a set of flags describing properties of a basic type.
type BasicInfo int

// Properties of basic types.
const (
	IsBoolean BasicInfo = 1 << iota
	IsInteger
	IsUnsigned
	IsFloat
	IsComplex
	IsString
	IsUntyped

	IsOrdered   = IsInteger | IsFloat | IsString
	IsNumeric   = IsInteger | IsFloat | IsComplex
	IsConstType = IsBoolean | IsNumeric | IsString
)

// A Basic represents a basic type.
type Basic struct {
	kind BasicKind
	info BasicInfo
	name string
}

// Kind returns the kind of basic type b.
func (b *Basic) Kind() BasicKind { return b.kind }

// Info returns information about properties of basic type b.
func (b *Basic) Info() BasicInfo { return b.info }

// Name returns the name of basic type b.
func (b *Basic) Name() string { return b.name }

// An Array represents an array type.
type Array struct {
	len  int64
	elem Type
}

// NewArray returns a new array type for the given element type and length.
func NewArray(elem Type, len int64) *Array { return &Array{len, elem} }

// Len returns the length of array a.
func (a *Array) Len() int64 { return a.len }

// Elem returns element type of array a.
func (a *Array) Elem() Type { return a.elem }

// A Slice represents a slice type.
type Slice struct {
	elem Type
}

// NewSlice returns a new slice type for the given element type.
func NewSlice(elem Type) *Slice { return &Slice{elem} }

// Elem returns the element type of slice s.
func (s *Slice) Elem() Type { return s.elem }

// TypeParam is an identifier for a type used in generic data structures and
// functions.
type TypeParam string

// NewTypeParam returns a new type parameter with the given name.
func NewTypeParam(name string) *TypeParam {
	tp := TypeParam(name)
	return &tp
}

// Underlying for type parameters always returns the empty interface. The
// compiler can make no assumptions about the underlying type.
func (tp *TypeParam) Underlying() Type {
	return NewInterface(nil, nil)
}

func (tp TypeParam) String() string {
	return string(tp)
}

// A Struct represents a struct type.
type Struct struct {
	fields []*Var
	tags   []string // field tags; nil if there are no tags
}

// NewStruct returns a new struct with the given fields and corresponding field tags.
// If a field with index i has a tag, tags[i] must be that tag, but len(tags) may be
// only as long as required to hold the tag with the largest index i. Consequently,
// if no field has a tag, tags may be nil.
func NewStruct(fields []*Var, tags []string) *Struct {
	var fset objset
	for _, f := range fields {
		if f.name != "_" && fset.insert(f) != nil {
			panic("multiple fields with the same name")
		}
	}
	if len(tags) > len(fields) {
		panic("more tags than fields")
	}
	return &Struct{fields: fields, tags: tags}
}

// NumFields returns the number of fields in the struct (including blank and anonymous fields).
func (s *Struct) NumFields() int { return len(s.fields) }

// Field returns the i'th field for 0 <= i < NumFields().
func (s *Struct) Field(i int) *Var { return s.fields[i] }

// Tag returns the i'th field tag for 0 <= i < NumFields().
func (s *Struct) Tag(i int) string {
	if i < len(s.tags) {
		return s.tags[i]
	}
	return ""
}

// A Pointer represents a pointer type.
type Pointer struct {
	base Type // element type
}

// NewPointer returns a new pointer type for the given element (base) type.
func NewPointer(elem Type) *Pointer { return &Pointer{base: elem} }

// Elem returns the element type for the given pointer p.
func (p *Pointer) Elem() Type { return p.base }

// A Tuple represents an ordered list of variables; a nil *Tuple is a valid (empty) tuple.
// Tuples are used as components of signatures and to represent the type of multiple
// assignments; they are not first class types of Go.
type Tuple struct {
	vars []*Var
}

// NewTuple returns a new tuple for the given variables.
func NewTuple(x ...*Var) *Tuple {
	if len(x) > 0 {
		return &Tuple{x}
	}
	return nil
}

// Len returns the number variables of tuple t.
func (t *Tuple) Len() int {
	if t != nil {
		return len(t.vars)
	}
	return 0
}

// At returns the i'th variable of tuple t.
func (t *Tuple) At(i int) *Var { return t.vars[i] }

type BaseSignature interface {
	Recv() *Var
	Params() *Tuple
	Results() *Tuple
	Variadic() bool
}

// A Signature represents a (non-builtin) function or method type.
// The receiver is ignored when comparing signatures for identity.
type Signature struct {
	// We need to keep the scope in Signature (rather than passing it around
	// and store it in the Func Object) because when type-checking a function
	// literal we call the general type checker which returns a general Type.
	// We then unpack the *Signature and use the scope for the literal body.
	scope    *Scope // function scope, present for package-local signatures
	recv     *Var   // nil if not a method
	params   *Tuple // (incoming) parameters from left to right; or nil
	results  *Tuple // (outgoing) results from left to right; or nil
	variadic bool   // true if the last parameter's type is of the form ...T (or string, for append built-in only)
}

// NewSignature returns a new function type for the given receiver, parameters,
// and results, either of which may be nil. If variadic is set, the function
// is variadic, it must have at least one parameter, and the last parameter
// must be of unnamed slice type.
func NewSignature(recv *Var, params, results *Tuple, variadic bool) *Signature {
	if variadic {
		n := params.Len()
		if n == 0 {
			panic("types.NewSignature: variadic function must have at least one parameter")
		}
		if _, ok := params.At(n - 1).typ.(*Slice); !ok {
			panic("types.NewSignature: variadic parameter must be of unnamed slice type")
		}
	}

	return &Signature{
		scope:    nil,
		recv:     recv,
		params:   params,
		results:  results,
		variadic: variadic,
	}
}

// Recv returns the receiver of signature s (if a method), or nil if a
// function. It is ignored when comparing signatures for identity.
//
// For an abstract method, Recv returns the enclosing interface either
// as a *Named or an *Interface. Due to embedding, an interface may
// contain methods whose receiver type is a different interface.
func (s *Signature) Recv() *Var { return s.recv }

// Params returns the parameters of signature s, or nil.
func (s *Signature) Params() *Tuple { return s.params }

// Results returns the results of signature s, or nil.
func (s *Signature) Results() *Tuple { return s.results }

// Variadic reports whether the signature s is variadic.
func (s *Signature) Variadic() bool { return s.variadic }

type GenericSignature struct {
	*Signature                  // signature may contain some type parameters in place of types
	typeParams     []*TypeParam // generic type parameters (if any)
	recvTypeParams []*TypeParam // type parameters of the receiver type (if any)
	obj            *Func        // obj points to the corresponding declaration
	// dependents are generic usages inside the function body which inherit
	// type parameters from the function declaration.
	dependents []PartialGenericType
}

func NewGenericSignature(recv *Var, params, results *Tuple, variadic bool, typeParams, recvTypeParams []*TypeParam) *GenericSignature {

	sig := NewSignature(recv, params, results, variadic)

	// TODO(albrow): test this
	var tset map[string]struct{}
	if len(typeParams) > 0 {
		tset = map[string]struct{}{}
		for _, t := range typeParams {
			if _, found := tset[t.String()]; found {
				panic("types.NewSignature: cannot have multiple type parameters with the same name")
			} else {
				tset[t.String()] = struct{}{}
			}
		}
		for _, t := range recvTypeParams {
			if _, found := tset[t.String()]; found {
				panic("types.NewSignature: cannot have multiple type parameters with the same name")
			} else {
				tset[t.String()] = struct{}{}
			}
		}
	}

	return &GenericSignature{
		Signature:      sig,
		typeParams:     typeParams,
		recvTypeParams: recvTypeParams,
	}
}

func (gs *GenericSignature) TypeParams() []*TypeParam {
	return gs.typeParams
}

func (gs *GenericSignature) Object() Object {
	return gs.obj
}

// ConcreteSignature is the corresponding concrete type of a generic Signature
// for which type arguments have been provided.
type ConcreteSignature struct {
	*Signature
	genType *GenericSignature
	typeMap map[string]Type // map of type parameter name to concrete type
}

func (cs *ConcreteSignature) GenericType() GenericType {
	return cs.genType
}

func (cs *ConcreteSignature) TypeMap() map[string]Type {
	return cs.typeMap
}

type PartialGenericSignature struct {
	*Signature
	genType *GenericSignature
	typeMap map[string]Type // map of type parameter name to concrete type
}

func (pgs *PartialGenericSignature) TypeParams() []*TypeParam {
	return pgs.genType.typeParams
}

func (pgs *PartialGenericSignature) Object() Object {
	return pgs.genType.obj
}

func (pgs *PartialGenericSignature) GenericType() GenericType {
	return pgs.genType
}

func (pgs *PartialGenericSignature) TypeMap() map[string]Type {
	return pgs.typeMap
}

// An Interface represents an interface type.
type Interface struct {
	methods   []*Func  // ordered list of explicitly declared methods
	embeddeds []Type   // ordered list of explicitly embedded types

	allMethods []*Func // ordered list of methods declared with or embedded in this interface (TODO(gri): replace with mset)
}

// emptyInterface represents the empty (completed) interface
var emptyInterface = Interface{allMethods: markComplete}

// markComplete is used to mark an empty interface as completely
// set up by setting the allMethods field to a non-nil empty slice.
var markComplete = make([]*Func, 0)

// NewInterface returns a new (incomplete) interface for the given methods and embedded types.
// To compute the method set of the interface, Complete must be called.
func NewInterface(methods []*Func, embeddeds []*Named) *Interface {
	var tnames []Type
	if len(embeddeds) > 0 {
		tnames := make([]Type, len(embeddeds))
		for i, t := range embeddeds {
			tnames[i] = t
		}
	}
	return NewInterface2(methods, tnames)
}

// NewInterface2 returns a new (incomplete) interface for the given methods and embedded types.
// Each embedded type must have an underlying type of interface type.
// NewInterface2 takes ownership of the provided methods and may modify their types by setting
// missing receivers. To compute the method set of the interface, Complete must be called.
func NewInterface2(methods []*Func, embeddeds []Type) *Interface {
	typ := new(Interface)

	if len(methods) == 0 && len(embeddeds) == 0 {
		return typ
	}

	var mset objset
	for _, m := range methods {
		if mset.insert(m) != nil {
			panic("multiple methods with the same name")
		}
		// set receiver
		// TODO(gri) Ideally, we should use a named type here instead of
		// typ, for less verbose printing of interface method signatures.
		m.typ.(*Signature).recv = NewVar(m.pos, m.pkg, "", typ)
	}
	sort.Sort(byUniqueMethodName(methods))

	if len(embeddeds) > 0 {
			for _, t := range embeddeds {
				if !IsInterface(t) {
					panic("embedded type is not an interface")
				}
			}
			sort.Stable(byUniqueTypeName(embeddeds))
	}

	typ.methods = methods
	typ.embeddeds = embeddeds
	return typ
}

func NewInterfaceType(methods []*Func, embeddeds []Type) *Interface {
	typ := new(Interface)

	if len(methods) == 0 && len(embeddeds) == 0 {
		return typ
	}

	var mset objset
	for _, m := range methods {
		if mset.insert(m) != nil {
			panic("multiple methods with the same name")
		}
		// set receiver if we don't have one
		if sig := m.typ.(*Signature); sig.recv == nil {
			sig.recv = NewVar(m.pos, m.pkg, "", typ)
		}
	}
	sort.Sort(byUniqueMethodName(methods))

	if len(embeddeds) > 0 {
		// All embedded types should be interfaces; however, defined types
		// may not yet be fully resolved. Only verify that non-defined types
		// are interfaces. This matches the behavior of the code before the
		// fix for #25301 (issue #25596).
		for _, t := range embeddeds {
			if _, ok := t.(*Named); !ok && !IsInterface(t) {
				panic("embedded type is not an interface")
			}
		}
		sort.Stable(byUniqueTypeName(embeddeds))
	}

	typ.methods = methods
	typ.embeddeds = embeddeds
	return typ
}


// NumExplicitMethods returns the number of explicitly declared methods of interface t.
func (t *Interface) NumExplicitMethods() int { return len(t.methods) }

// ExplicitMethod returns the i'th explicitly declared method of interface t for 0 <= i < t.NumExplicitMethods().
// The methods are ordered by their unique Id.
func (t *Interface) ExplicitMethod(i int) *Func { return t.methods[i] }

// NumEmbeddeds returns the number of embedded types in interface t.
func (t *Interface) NumEmbeddeds() int { return len(t.embeddeds) }

// Embedded returns the i'th embedded defined (*Named) type of interface t for 0 <= i < t.NumEmbeddeds().
// The result is nil if the i'th embedded type is not a defined type.
//
// Deprecated: Use EmbeddedType which is not restricted to defiend (*Named) types.
func (t *Interface) Embedded(i int) *Named { tname, _ := t.embeddeds[i].(*Named); return tname }

// EmbeddedType returns the i'th embedded type of interface t for 0 <= i < t.NumEmbeddeds().
func (t *Interface) EmbeddedType(i int) Type { return t.embeddeds[i] }

// NumMethods returns the total number of methods of interface t.
func (t *Interface) NumMethods() int { return len(t.allMethods) }

// Method returns the i'th method of interface t for 0 <= i < t.NumMethods().
// The methods are ordered by their unique Id.
func (t *Interface) Method(i int) *Func { return t.allMethods[i] }

// Empty returns true if t is the empty interface.
func (t *Interface) Empty() bool { return len(t.allMethods) == 0 }

// Complete computes the interface's method set. It must be called by users of
// NewInterface after the interface's embedded types are fully defined and
// before using the interface type in any way other than to form other types.
// Complete returns the receiver.
func (t *Interface) Complete() *Interface {
	if t.allMethods != nil {
		return t
	}

	var allMethods []*Func
	if t.embeddeds == nil {
		if t.methods == nil {
			allMethods = make([]*Func, 0, 1)
		} else {
			allMethods = t.methods
		}
	} else {
		allMethods = append(allMethods, t.methods...)
		for _, et := range t.embeddeds {
			it := et.Underlying().(*Interface)
			it.Complete()
			for _, tm := range it.allMethods {
				// Make a copy of the method and adjust its receiver type.
				newm := *tm
				newmtyp := *tm.typ.(*Signature)
				newm.typ = &newmtyp
				newmtyp.recv = NewVar(newm.pos, newm.pkg, "", t)
				allMethods = append(allMethods, &newm)
			}
		}
		sort.Sort(byUniqueMethodName(allMethods))
	}
	t.allMethods = allMethods

	return t
}

// A Map represents a map type.
type Map struct {
	key, elem Type
}

// NewMap returns a new map for the given key and element types.
func NewMap(key, elem Type) *Map {
	return &Map{key, elem}
}

// Key returns the key type of map m.
func (m *Map) Key() Type { return m.key }

// Elem returns the element type of map m.
func (m *Map) Elem() Type { return m.elem }

// A Chan represents a channel type.
type Chan struct {
	dir  ChanDir
	elem Type
}

// A ChanDir value indicates a channel direction.
type ChanDir int

// The direction of a channel is indicated by one of these constants.
const (
	SendRecv ChanDir = iota
	SendOnly
	RecvOnly
)

// NewChan returns a new channel type for the given direction and element type.
func NewChan(dir ChanDir, elem Type) *Chan {
	return &Chan{dir, elem}
}

// Dir returns the direction of channel c.
func (c *Chan) Dir() ChanDir { return c.dir }

// Elem returns the element type of channel c.
func (c *Chan) Elem() Type { return c.elem }

type BaseNamed interface {
	Type
	Obj() *TypeName
	NumMethods() int
	Method(int) *Func
}

// A Named represents a named type.
type Named struct {
	obj        *TypeName // corresponding declared object
	underlying Type      // possibly a *Named during setup; never a *Named once set up completely
	methods    []*Func   // methods declared for this type (not the method set of this type)
}

// NewNamed returns a new named type for the given type name, underlying type, and associated methods.
// If the given type name obj doesn't have a type yet, its type is set to the returned named type.
// The underlying type must not be a *Named.
func NewNamed(obj *TypeName, underlying Type, methods []*Func) *Named {
	if _, ok := underlying.(*Named); ok {
		panic("types.NewNamed: underlying type must not be *Named")
	}

	typ := &Named{obj: obj, underlying: underlying, methods: methods}
	if obj.typ == nil {
		obj.typ = typ
	}
	return typ
}

// Obj returns the type name for the named type t.
func (t *Named) Obj() *TypeName { return t.obj }

// NumMethods returns the number of explicit methods whose receiver is named type t.
func (t *Named) NumMethods() int { return len(t.methods) }

// Method returns the i'th method of named type t for 0 <= i < t.NumMethods().
func (t *Named) Method(i int) *Func { return t.methods[i] }

// SetUnderlying sets the underlying type and marks t as complete.
func (t *Named) SetUnderlying(underlying Type) {
	if underlying == nil {
		panic("types.Named.SetUnderlying: underlying type must not be nil")
	}
	if _, ok := underlying.(*Named); ok {
		panic("types.Named.SetUnderlying: underlying type must not be *Named")
	}
	t.underlying = underlying
}

// AddMethod adds method m unless it is already in the method list.
func (t *Named) AddMethod(m *Func) {
	if i, _ := lookupMethod(t.methods, m.pkg, m.name); i < 0 {
		t.methods = append(t.methods, m)
	}
}

type GenericNamed struct {
	*Named
	typeParams []*TypeParam
}

func NewGenericNamed(obj *TypeName, underlying Type, methods []*Func, typeParams []*TypeParam) *GenericNamed {
	named := NewNamed(obj, underlying, methods)

	// TODO(albrow): test this
	var tset map[string]struct{}
	if len(typeParams) > 0 {
		tset = map[string]struct{}{}
		for _, t := range typeParams {
			if _, found := tset[t.String()]; found {
				panic("types.NewNamed: cannot have multiple type parameters with the same name")
			} else {
				tset[t.String()] = struct{}{}
			}
		}
	}

	return &GenericNamed{
		Named:      named,
		typeParams: typeParams,
	}
}

func (gn *GenericNamed) TypeParams() []*TypeParam {
	return gn.typeParams
}

func (gn *GenericNamed) Object() Object {
	return gn.obj
}

// GenericType() GenericType
// TypeArgs() map[string]Type

// ConcreteNamed is the corresponding concrete type of a generic Named type for
// which type arguments have been provided.
type ConcreteNamed struct {
	*Named
	genType *GenericNamed
	typeMap map[string]Type // map of type parameter name to concrete type
}

func (cn *ConcreteNamed) GenericType() GenericType {
	return cn.genType
}

func (cn *ConcreteNamed) TypeMap() map[string]Type {
	return cn.typeMap
}

type PartialGenericNamed struct {
	*Named
	genType *GenericNamed
	typeMap map[string]Type
}

func (pgn *PartialGenericNamed) TypeParams() []*TypeParam {
	return pgn.genType.typeParams
}

func (pgn *PartialGenericNamed) Object() Object {
	return pgn.genType.obj
}

func (pgn *PartialGenericNamed) GenericType() GenericType {
	return pgn.genType
}

func (pgn *PartialGenericNamed) TypeMap() map[string]Type {
	return pgn.typeMap
}

// Implementations for Type methods.

func (t *Basic) Underlying() Type     { return t }
func (t *Array) Underlying() Type     { return t }
func (t *Slice) Underlying() Type     { return t }
func (t *Struct) Underlying() Type    { return t }
func (t *Pointer) Underlying() Type   { return t }
func (t *Tuple) Underlying() Type     { return t }
func (t *Signature) Underlying() Type { return t }
func (t *Interface) Underlying() Type { return t }
func (t *Map) Underlying() Type       { return t }
func (t *Chan) Underlying() Type      { return t }
func (t *Named) Underlying() Type     { return t.underlying }

func (t *Basic) String() string                   { return TypeString(t, nil) }
func (t *Array) String() string                   { return TypeString(t, nil) }
func (t *Slice) String() string                   { return TypeString(t, nil) }
func (t *Struct) String() string                  { return TypeString(t, nil) }
func (t *Pointer) String() string                 { return TypeString(t, nil) }
func (t *Tuple) String() string                   { return TypeString(t, nil) }
func (t *Signature) String() string               { return TypeString(t, nil) }
func (t *GenericSignature) String() string        { return TypeString(t, nil) }
func (t *PartialGenericSignature) String() string { return TypeString(t, nil) }
func (t *ConcreteSignature) String() string       { return TypeString(t, nil) }
func (t *Interface) String() string               { return TypeString(t, nil) }
func (t *Map) String() string                     { return TypeString(t, nil) }
func (t *Chan) String() string                    { return TypeString(t, nil) }
func (t *Named) String() string                   { return TypeString(t, nil) }
func (t *GenericNamed) String() string            { return TypeString(t, nil) }
func (t *PartialGenericNamed) String() string     { return TypeString(t, nil) }
func (t *ConcreteNamed) String() string           { return TypeString(t, nil) }
