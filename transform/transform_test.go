package transform

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qProust/fo/ast"
	"github.com/qProust/fo/format"
	"github.com/qProust/fo/importer"
	"github.com/qProust/fo/parser"
	"github.com/qProust/fo/token"
	"github.com/qProust/fo/types"
	"github.com/aryann/difflib"
)

func TestTransformStructTypeUnused(t *testing.T) {
	src := `package main

type T[U] struct {}

func f[T](x T) {}

func (T[U]) f0() {}

func (T) f1() {}

func main() { }
`

	expected := `package main

func main() {}
`

	testParseFile(t, src, expected)
}

func TestTransformStructTypeLiterals(t *testing.T) {
	src := `package main

type Box[T] struct {
	val T
}

type Tuple[T, U] struct {
	first T
	second U
}

type Map[T, U] struct {
	m map[T]U
}

func main() {
	var _ = Box[string]{}
	var _ = &Box[int]{}
	var _ = []Box[string]{}
	var _ = [2]Box[int]{}
	var _ = map[string]Box[string]{}

	var _ = Map[string, int]{}

	var _ = Tuple[int, string] {
		first: 2,
		second: "foo",
	}
}
`

	expected := `package main

type (
	Box__int struct {
		val int
	}
	Box__string struct {
		val string
	}
)

type Tuple__int__string struct {
	first  int
	second string
}

type Map__string__int struct {
	m map[string]int
}

func main() {
	var _ = Box__string{}
	var _ = &Box__int{}
	var _ = []Box__string{}
	var _ = [2]Box__int{}
	var _ = map[string]Box__string{}

	var _ = Map__string__int{}

	var _ = Tuple__int__string{
		first:  2,
		second: "foo",
	}
}
`
	testParseFile(t, src, expected)
}

func TestTransformStructTypeSelectorUsage(t *testing.T) {
	src := `package main

import "bytes"

type Box[T] struct{
	val T
}

func main() {
	var _ = Box[bytes.Buffer]{}
}
`

	expected := `package main

import "bytes"

type Box__bytes_Buffer struct {
	val bytes.Buffer
}

func main() {
	var _ = Box__bytes_Buffer{}
}
`
	testParseFile(t, src, expected)
}

func TestTransformStructTypeFuncArgs(t *testing.T) {
	src := `package main

type Either[T, U] struct {
	left T
	right U
}

func getData() Either[int, string] {
	return Either[int, string]{}
}

func handleEither(e Either[error, string]) {
}

func main() { }
`

	expected := `package main

type (
	Either__error__string struct {
		left  error
		right string
	}
	Either__int__string struct {
		left  int
		right string
	}
)

func getData() Either__int__string {
	return Either__int__string{}
}

func handleEither(e Either__error__string) {
}

func main() {}
`

	testParseFile(t, src, expected)
}

func TestTransformStructTypeSwitch(t *testing.T) {
	src := `package main

type Box[T] struct {
	val T
}

func main() {
	var x interface{} = Box[int]{}
	switch x.(type) {
	case Box[int]:
	case Box[string]:
	}
}
`

	expected := `package main

type (
	Box__int struct {
		val int
	}
	Box__string struct {
		val string
	}
)

func main() {
	var x interface{} = Box__int{}
	switch x.(type) {
	case Box__int:
	case Box__string:
	}
}
`

	testParseFile(t, src, expected)
}

func TestTransformStructTypeAssert(t *testing.T) {
	src := `package main

type Box[T] struct {
	val T
}

func main() {
	var x interface{} = Box[int]{}
	_ = x.(Box[int])
	_ = x.(Box[string])
}
`

	expected := `package main

type (
	Box__int struct {
		val int
	}
	Box__string struct {
		val string
	}
)

func main() {
	var x interface{} = Box__int{}
	_ = x.(Box__int)
	_ = x.(Box__string)
}
`

	testParseFile(t, src, expected)
}

func TestTransformFuncDecl(t *testing.T) {
	src := `package main

import "fmt"

func Print[T](t T) {
	fmt.Println(t)
}

func MakeSlice[T]() []T {
	return make([]T, 0)
}

func main() {
	Print[int](5)
	Print[int](42)
	Print[string]("foo")
	MakeSlice[string]()
}
`

	expected := `package main

import "fmt"

func Print__int(t int) {
	fmt.Println(t)
}
func Print__string(t string) {
	fmt.Println(t)
}

func MakeSlice__string() []string {
	return make([]string, 0)
}

func main() {
	Print__int(5)
	Print__int(42)
	Print__string("foo")
	MakeSlice__string()
}
`

	testParseFile(t, src, expected)
}

func TestTransformStructTypeInherited(t *testing.T) {
	src := `package main

type Tuple[T, U] struct {
	first T
	second U
}

type BoxedTuple[T, U] struct {
	val Tuple[T, U]
}

type BoxedTupleString[T] struct {
	val Tuple[string, T]
}

func main() {
	var _ = BoxedTuple[string, int]{
		val: Tuple[string, int]{
			first: "foo",
			second: 42,
		},
	}
	var _ = BoxedTuple[float64, int]{}
	var _ = BoxedTupleString[bool]{
		val: Tuple[string, bool] {
			first: "abc",
			second: true,
		},
	}
	var _ = BoxedTupleString[uint]{}
}
`

	expected := `package main

type (
	Tuple__float64__int struct {
		first  float64
		second int
	}
	Tuple__string__bool struct {
		first  string
		second bool
	}
	Tuple__string__int struct {
		first  string
		second int
	}
	Tuple__string__uint struct {
		first  string
		second uint
	}
)

type (
	BoxedTuple__float64__int struct {
		val Tuple__float64__int
	}
	BoxedTuple__string__int struct {
		val Tuple__string__int
	}
)

type (
	BoxedTupleString__bool struct {
		val Tuple__string__bool
	}
	BoxedTupleString__uint struct {
		val Tuple__string__uint
	}
)

func main() {
	var _ = BoxedTuple__string__int{
		val: Tuple__string__int{
			first:  "foo",
			second: 42,
		},
	}
	var _ = BoxedTuple__float64__int{}
	var _ = BoxedTupleString__bool{
		val: Tuple__string__bool{
			first:  "abc",
			second: true,
		},
	}
	var _ = BoxedTupleString__uint{}
}
`

	testParseFile(t, src, expected)
}

func TestTransformFuncDeclInherited(t *testing.T) {
	src := `package main

type Tuple[T, U] struct {
	first T
	second U
}

func NewTuple[T, U](first T, second U) Tuple[T, U] {
	return Tuple[T, U]{
		first: first,
		second: second,
	}
}

func NewTupleString[T](first string, second T) Tuple[string, T] {
	return Tuple[string, T] {
		first: first,
		second: second,
	}
}

func main() {
	var _ = NewTuple[bool, int64](true, 42)
	var _ = NewTupleString[float64]("foo", 12.34)
}
`

	expected := `package main

type (
	Tuple__bool__int64 struct {
		first  bool
		second int64
	}
	Tuple__string__float64 struct {
		first  string
		second float64
	}
)

func NewTuple__bool__int64(first bool, second int64) Tuple__bool__int64 {
	return Tuple__bool__int64{
		first:  first,
		second: second,
	}
}

func NewTupleString__float64(first string, second float64) Tuple__string__float64 {
	return Tuple__string__float64{
		first:  first,
		second: second,
	}
}

func main() {
	var _ = NewTuple__bool__int64(true, 42)
	var _ = NewTupleString__float64("foo", 12.34)
}
`

	testParseFile(t, src, expected)
}

// TODO(albrow): Make this test pass.
func TestTransformInerhitedInBody(t *testing.T) {
	src := `package main
	
type A[T] T

func NewA[T]() {
	var _ A[T]
	F[T]()
}

func F[T]() T {
	var x T
	return x
}

func main() {
	NewA[string]()
}
	`

	expected := `package main

type A__string string

func NewA__string() {
	var _ A__string
	F__string()
}

func F__string() string {
	var x string
	return x
}

func main() {
	NewA__string()
}
`
	testParseFile(t, src, expected)
}

func TestTransformMethods(t *testing.T) {
	src := `package main

import (
	"fmt"
	"strconv"
)

type A[T] T

func (A[T]) f0() T {
	var x T
	return x
}

func (a A[T]) f1() T {
	return T(a)
}

func (a A[T]) f2[U, V]() (T, U, V) {
	var x U
	var y V
	return T(a), x, y
}

func (*A) f3() {}

type B[T] struct {
	v T
}

func (b B[T]) f0[V](f func(T) V) B[V] {
	return B[V]{
		v: f(b.v),
	}
}

func main() {
	var _ = A[string]("")
	var _ = A[bool](true)

	var x A[uint]
	var a uint
	var b float64
	var c int8
	a, b, c = x.f2[float64, int8]()
	fmt.Println(a, b, c)

	y := B[int]{ v: 42 }
	var _ B[string] = y.f0[string](strconv.Itoa)
}
`

	expected := `package main

import (
	"fmt"
	"strconv"
)

type (
	A__bool   bool
	A__string string
	A__uint   uint
)

func (A__bool) f0() bool {
	var x bool
	return x
}
func (A__string) f0() string {
	var x string
	return x
}
func (A__uint) f0() uint {
	var x uint
	return x
}

func (a A__bool) f1() bool {
	return bool(a)
}
func (a A__string) f1() string {
	return string(a)
}
func (a A__uint) f1() uint {
	return uint(a)
}

func (a A__uint) f2__float64__int8() (uint, float64, int8) {
	var x float64
	var y int8
	return uint(a), x, y
}

func (*A__bool) f3()   {}
func (*A__string) f3() {}
func (*A__uint) f3()   {}

type (
	B__int struct {
		v int
	}
	B__string struct {
		v string
	}
)

func (b B__int) f0__string(f func(int) string) B__string {
	return B__string{
		v: f(b.v),
	}
}

func main() {
	var _ = A__string("")
	var _ = A__bool(true)

	var x A__uint
	var a uint
	var b float64
	var c int8
	a, b, c = x.f2__float64__int8()
	fmt.Println(a, b, c)

	y := B__int{v: 42}
	var _ B__string = y.f0__string(strconv.Itoa)
}
`

	testParseFile(t, src, expected)
}

func TestTransformUnsafeSymbols(t *testing.T) {
	src := `package main

import "bytes"

type A[T] T

func main() {
	var _ A[[]string]
	var _ A[map[string]int]
	var _ A[map[string][]bytes.Buffer]
}
`

	expected := `package main

import "bytes"

type (
	A____string                  []string
	A__map_string___bytes_Buffer map[string][]bytes.Buffer
	A__map_string_int            map[string]int
)

func main() {
	var _ A____string
	var _ A__map_string_int
	var _ A__map_string___bytes_Buffer
}
`

	testParseFile(t, src, expected)
}

// Note: In this case, we expect *two* generated concrete types, one for S and
// one for string
func TestTransformCustomTypes(t *testing.T) {
	src := `package main

type Box[T] struct {
	v T
}

type S string

func main() {
	var _ = Box[S]{
		v: "",
	}
	var _ = Box[string]{
		v: "",
	}
}
`

	expected := `package main

type (
	Box__S struct {
		v S
	}
	Box__string struct {
		v string
	}
)

type S string

func main() {
	var _ = Box__S{
		v: "",
	}
	var _ = Box__string{
		v: "",
	}
}
`

	testParseFile(t, src, expected)
}

// Note: In this case, we expect *one* generated concrete type, because S is
// defined as exactly equivalent to string.
func TestTransformTypeAlias(t *testing.T) {
	src := `package main

type Box[T] struct {
	v T
}

type S = string

func main() {
	var _ = Box[S]{
		v: "",
	}
	var _ = Box[string]{
		v: "",
	}
}
`

	expected := `package main

type Box__string struct {
	v string
}

type S = string

func main() {
	var _ = Box__string{
		v: "",
	}
	var _ = Box__string{
		v: "",
	}
}
`

	testParseFile(t, src, expected)
}

func TestTransformImportGo(t *testing.T) {
	src := `package main

import (
	"github.com/qProust/fo/ast"
)

type List[T] []T

func NewList[T] () List[T] {
	return List[T]{}
}

func (l List[T]) Head() T {
	if len(l) > 0 {
		return l[0]
	}
	var x T
	return x
}

func (l List[T]) Append(v T) List[T] {
	var result List[T] = make([]T, len(l))
	result = append(result, v)
	return result
}

func main() {
	list := NewList[*ast.Ident]()
	list = list.Append(ast.NewIdent(""))
	var _ *ast.Ident = list.Head()

	var _ = NewList[[]ast.Ident]()
	var _ = NewList[[5]ast.Ident]()
	var _ = NewList[map[string]ast.Ident]()
	var _ = NewList[chan ast.Ident]()
}
`

	expected := `package main

import (
	"github.com/qProust/fo/ast"
)

type (
	List___5_ast_Ident         [][5]ast.Ident
	List____ast_Ident          [][]ast.Ident
	List___ast_Ident           []*ast.Ident
	List__ast_Ident            []ast.Ident
	List__map_string_ast_Ident []map[string]ast.Ident
)

func NewList___5_ast_Ident() List___5_ast_Ident {
	return List___5_ast_Ident{}
}
func NewList____ast_Ident() List____ast_Ident {
	return List____ast_Ident{}
}
func NewList___ast_Ident() List___ast_Ident {
	return List___ast_Ident{}
}
func NewList__ast_Ident() List__ast_Ident {
	return List__ast_Ident{}
}
func NewList__map_string_ast_Ident() List__map_string_ast_Ident {
	return List__map_string_ast_Ident{}
}

func (l List__ast_Ident) Head() ast.Ident {
	if len(l) > 0 {
		return l[0]
	}
	var x ast.Ident
	return x
}
func (l List___ast_Ident) Head() *ast.Ident {
	if len(l) > 0 {
		return l[0]
	}
	var x *ast.Ident
	return x
}
func (l List___5_ast_Ident) Head() [5]ast.Ident {
	if len(l) > 0 {
		return l[0]
	}
	var x [5]ast.Ident
	return x
}
func (l List____ast_Ident) Head() []ast.Ident {
	if len(l) > 0 {
		return l[0]
	}
	var x []ast.Ident
	return x
}
func (l List__map_string_ast_Ident) Head() map[string]ast.Ident {
	if len(l) > 0 {
		return l[0]
	}
	var x map[string]ast.Ident
	return x
}

func (l List__ast_Ident) Append(v ast.Ident) List__ast_Ident {
	var result List__ast_Ident = make([]ast.Ident, len(l))
	result = append(result, v)
	return result
}
func (l List___ast_Ident) Append(v *ast.Ident) List___ast_Ident {
	var result List___ast_Ident = make([]*ast.Ident, len(l))
	result = append(result, v)
	return result
}
func (l List___5_ast_Ident) Append(v [5]ast.Ident) List___5_ast_Ident {
	var result List___5_ast_Ident = make([][5]ast.Ident, len(l))
	result = append(result, v)
	return result
}
func (l List____ast_Ident) Append(v []ast.Ident) List____ast_Ident {
	var result List____ast_Ident = make([][]ast.Ident, len(l))
	result = append(result, v)
	return result
}
func (l List__map_string_ast_Ident) Append(v map[string]ast.Ident) List__map_string_ast_Ident {
	var result List__map_string_ast_Ident = make([]map[string]ast.Ident, len(l))
	result = append(result, v)
	return result
}

func main() {
	list := NewList___ast_Ident()
	list = list.Append(ast.NewIdent(""))
	var _ *ast.Ident = list.Head()

	var _ = NewList____ast_Ident()
	var _ = NewList___5_ast_Ident()
	var _ = NewList__map_string_ast_Ident()
	var _ = NewList__chan_ast_Ident()
}
`

	testParseFile(t, src, expected)
}

// See https://github.com/albrow/fo/issues/3 and
// https://github.com/albrow/fo/issues/15
func TestTransformRecursive(t *testing.T) {
	src := `package main

type A[T] struct {
	a *A[T]
	v T
}

func (a *A) init() {
	a.a = a
}

type B[T, U] struct {
	b *B[U, T]
	t T
	u U
}

type C[T] struct {
	d *D[T]
	v T
}

type D[T] struct {
	c *C[T]
	v T
}

func E[T]() T {
	return E[T]()
}

func F[T, U]() (T, U) {
	return F[T, U]()
}

func G[T]() T {
	return H[T]()
}

func H[T]() T {
	return G[T]()
}

func main() {
	a := A[string]{
		v: "foo",
	}
	a.init()
	var _ string = a.a.a.a.a.a.a.a.a.v

	var _ = B[string, int]{
		t: "foo",
		u: 42,
	}
	c := C[bool]{
		v: true,
	}
	d := D[bool]{
		c: &c,
		v: false,
	}
	c.d = &d
	var _ bool = c.d.c.d.c.d.c.d.c.d.c.d.v

	var _ uint8 = E[uint8]()
	var f0 float64
	var f1 complex64
	f0, f1 = F[float64, complex64]()
	print(f0)
	print(f1)

	var _ string = H[string]()
	var _ []int = G[[]int]()
}
`

	expected := `package main

type A__string struct {
	a *A__string
	v string
}

func (a *A__string) init() {
	a.a = a
}

type (
	B__int__string struct {
		b *B__string__int
		t int
		u string
	}
	B__string__int struct {
		b *B__int__string
		t string
		u int
	}
)

type C__bool struct {
	d *D__bool
	v bool
}

type D__bool struct {
	c *C__bool
	v bool
}

func E__uint8() uint8 {
	return E__uint8()
}

func F__float64__complex64() (float64, complex64) {
	return F__float64__complex64()
}

func G____int() []int {
	return H____int()
}
func G__string() string {
	return H__string()
}

func H____int() []int {
	return G____int()
}
func H__string() string {
	return G__string()
}

func main() {
	a := A__string{
		v: "foo",
	}
	a.init()
	var _ string = a.a.a.a.a.a.a.a.a.v

	var _ = B__string__int{
		t: "foo",
		u: 42,
	}
	c := C__bool{
		v: true,
	}
	d := D__bool{
		c: &c,
		v: false,
	}
	c.d = &d
	var _ bool = c.d.c.d.c.d.c.d.c.d.c.d.v

	var _ uint8 = E__uint8()
	var f0 float64
	var f1 complex64
	f0, f1 = F__float64__complex64()
	print(f0)
	print(f1)

	var _ string = H__string()
	var _ []int = G____int()
}
`

	testParseFile(t, src, expected)
}

func TestTransformSafeStringCollisions(t *testing.T) {
	src := `package main

type Box[T] struct{
	val T
}

func main() {
	var _ = Box[**string]{}
	var _ = Box[[]string]{}
	var _ = Box[****string]{}
	var _ = Box[[]**string]{}
	var _ = Box[**[]string]{}
	var _ = Box[[][]string]{}
}
`

	expected := `package main

type (
	Box______string struct {
		val ****string
	}
	Box______string_0 struct {
		val []**string
	}
	Box______string_1 struct {
		val **[]string
	}
	Box______string_2 struct {
		val [][]string
	}
	Box____string struct {
		val []string
	}
	Box____string_0 struct {
		val **string
	}
)

func main() {
	var _ = Box____string_0{}
	var _ = Box____string{}
	var _ = Box______string{}
	var _ = Box______string_0{}
	var _ = Box______string_1{}
	var _ = Box______string_2{}
}
`
	testParseFile(t, src, expected)
}

func testParseFile(t *testing.T, src string, expected string) {
	t.Helper()
	fset := token.NewFileSet()
	orig, err := parser.ParseFile(fset, "transform_test", src, 0)
	if err != nil {
		t.Fatalf("ParseFile returned error: %s", err.Error())
	}
	conf := types.Config{}
	conf.Importer = importer.Default()
	info := &types.Info{
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Uses:       map[*ast.Ident]types.Object{},
	}
	pkg, err := conf.Check("transformtest", fset, []*ast.File{orig}, info)
	if err != nil {
		t.Fatalf("conf.Check returned error: %s", err.Error())
	}
	trans := &Transformer{
		Fset: fset,
		Pkg:  pkg,
		Info: info,
	}
	transformed, err := trans.File(orig)
	if err != nil {
		t.Fatalf("Transform returned error: %s", err.Error())
	}
	output := bytes.NewBuffer(nil)
	if err := format.Node(output, fset, transformed); err != nil {
		t.Fatalf("format.Node returned error: %s", err.Error())
	}
	if output.String() != expected {
		diff := difflib.Diff(strings.Split(expected, "\n"), strings.Split(output.String(), "\n"))
		diffStrings := ""
		for _, d := range diff {
			diffStrings += d.String() + "\n"
		}
		t.Fatalf(
			"output of Transform did not match expected\n\n%s",
			diffStrings,
		)
	}
}
