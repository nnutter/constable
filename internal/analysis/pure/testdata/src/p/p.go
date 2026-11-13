package p

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"
)

var counter int

var globalSlice []int

var globalMap = map[string]int{}

var globalPtr *int

var globalStruct struct{ n int }

func UnannotatedGlobalWrite() {
	counter = 1
}

//constable:pure
func ReadsPointer(p *int) int {
	return *p
}

//constable:pure
func MutatesPointer(p *int) {
	*p = 1 // want "//constable:pure function mutates pointer parameter p"
}

//constable:pure
func MutatesSlice(s []int) {
	s[0] = 1 // want "//constable:pure function mutates parameter s"
}

//constable:pure
func MutatesMap(m map[string]int) {
	m["x"] = 1 // want "//constable:pure function mutates parameter m"
}

//constable:pure
func DeletesMapEntry(m map[string]int) {
	delete(m, "x") // want "//constable:pure function deletes from map parameter m"
}

type Counter struct{ n int }

//constable:pure
func (c *Counter) Inc() {
	c.n++ // want "//constable:pure method mutates receiver c"
}

type Count struct{ n int }

//constable:pure
func (c Count) Increment() Count {
	c.n++
	return c
}

//constable:pure
func WritesGlobalAssign() {
	counter = 1 // want "//constable:pure function writes global counter"
}

//constable:pure
func WritesGlobalInc() {
	counter++ // want "//constable:pure function writes global counter"
}

//constable:pure
func WritesGlobalSliceElem() {
	globalSlice[0] = 1 // want "//constable:pure function writes global globalSlice"
}

//constable:pure
func WritesGlobalMapElem() {
	globalMap["x"] = 1 // want "//constable:pure function writes global globalMap"
}

//constable:pure
func DeletesGlobalMap() {
	delete(globalMap, "x") // want "//constable:pure function writes global globalMap"
}

//constable:pure
func WritesGlobalField() {
	globalStruct.n = 1 // want "//constable:pure function writes global globalStruct"
}

//constable:pure
func WritesGlobalDeref() {
	*globalPtr = 1 // want "//constable:pure function writes global globalPtr"
}

//constable:pure
func WritesGlobalAlias() {
	g := &counter
	*g = 1 // want "//constable:pure function writes global counter"
}

//constable:pure
func ReadsGlobal() int {
	return counter
}

//constable:pure
func MutatesLocal() int {
	x := 0
	x++
	return x
}

//constable:pure
func SpawnsGoroutine() {
	go func() {}() // want "//constable:pure function spawns goroutine"
}

//constable:pure
func SpawnsGoroutineInClosure() {
	f := func() {
		go func() {}() // want "//constable:pure function spawns goroutine"
	}
	f()
}

//constable:pure
func ReadsClock() time.Time {
	return time.Now() // want "//constable:pure function calls time.Now"
}

//constable:pure
func SleepsClock() {
	time.Sleep(time.Second) // want "//constable:pure function calls time.Sleep"
}

//constable:pure
func MakesClockDate() time.Time {
	return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
}

//constable:pure
func ReadsRandom() int {
	return rand.Intn(100) // #nosec G404 -- the fixture is the point: random reads are banned. // want "//constable:pure function calls rand.Intn"
}

//constable:pure
func ReadsEnv() string {
	return os.Getenv("HOME") // want "//constable:pure function calls os.Getenv"
}

//constable:pure
func ReadsProcess() int {
	return os.Getpid() // want "//constable:pure function calls os.Getpid"
}

//constable:pure
func ChecksErrorPredicate(err error) bool {
	return os.IsNotExist(err)
}

//constable:pure
func WritesFile() {
	_ = os.WriteFile("example.txt", []byte("x"), 0o600) // want "//constable:pure function calls os.WriteFile"
}

//constable:pure
func PrintsFmt() {
	fmt.Println("x") // want "//constable:pure function calls fmt.Println"
}

//constable:pure
func FormatsFmt() string {
	return fmt.Sprintf("%d", 1)
}

//constable:pure
func PrintsLog() {
	log.Println("x") // want "//constable:pure function calls log.Println"
}

//constable:pure
func PrintsBuiltin() {
	println("x") // want "//constable:pure function calls println"
}

//constable:pure
func ReadsArgs() []string {
	return os.Args // want "//constable:pure function reads os.Args"
}

//constable:pure
func ReadsLocalTime() *time.Location {
	return time.Local // want "//constable:pure function reads time.Local"
}

//constable:pure
func CallsPureHelpers() string {
	return strings.ToUpper("x")
}

//constable:pure
func MutatesLocalSlice() {
	s := make([]int, 1)
	s[0] = 1
	_ = s
}

//constable:pure
func DeletesLocalMap() {
	m := map[string]int{}
	delete(m, "x")
}

//constable:pure
func PureWithClosureCallsClock() time.Duration {
	f := func() time.Duration {
		return time.Since(time.Now().Add(-time.Hour)) // want "//constable:pure function calls time.Since" "//constable:pure function calls time.Now"
	}
	return f()
}

//constable:pure
func PanicsOnly() {
	panic("boom")
}

type Holder struct{ p *int }

//constable:pure
func MutatesFieldThroughPointer(h Holder) {
	*h.p = 1 // want "//constable:pure function mutates parameter h"
}

type Scalar struct{ n int }

//constable:pure
func MutatesScalarFieldCopy(s Scalar) Scalar {
	s.n++
	return s
}

//constable:pure
func MutatesPointerAliasPure(p *int) {
	q := p
	*q = 1 // want "//constable:pure function mutates pointer parameter p"
}

type Element struct{ n int }

//constable:pure
func MutatesSliceElementField(elements []Element) {
	elements[0].n = 1 // want "//constable:pure function mutates parameter elements"
}

//constable:pure
func MutatesNestedSlicePure(ss [][]int) {
	ss[0][0] = 1 // want "//constable:pure function mutates parameter ss"
}

type Inner struct{ data []byte }

type Outer struct{ inner Inner }

//constable:pure
func MutatesNestedFieldPure(o Outer) {
	o.inner.data[0] = 0 // want "//constable:pure function mutates parameter o"
}

func Identity[T any](x T) T { return x }

//constable:pure
func CallsGenericPure() int {
	return Identity[int](1)
}

//constable:pure
func ReadsNonAmbientVars() *time.Location {
	_ = time.UTC
	_ = os.ErrNotExist
	return time.UTC
}

//constable:pure
func ReadsStdoutVar() *os.File {
	return os.Stdout // want "//constable:pure function reads os.Stdout"
}
