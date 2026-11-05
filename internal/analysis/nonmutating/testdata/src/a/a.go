package a

func UnannotatedMutation(p *int) {
	*p = 1
}

// Other annotation
func OtherAnnotationMutation(p *int) {
	*p = 1
}

//constable:nonmutating
func MutatesPointer(p *int) {
	*p = 1 // want "//constable:nonmutating function mutates pointer parameter p"
}

//constable:nonmutating
func ReadsPointer(p *int) int {
	return *p
}

//constable:nonmutating
func ReassignsParameter(p *int) {
	p = nil
}

//constable:nonmutating
func MutatesPointerAlias(p *int) {
	q := p
	*q = 1 // want "//constable:nonmutating function mutates pointer parameter p"
}

//constable:nonmutating
func MutatesSlice(s []int) {
	s[0] = 1 // want "//constable:nonmutating function mutates parameter s"
}

//constable:nonmutating
func MutatesSliceAlias(s []int) {
	t := s
	t[0] = 1 // want "//constable:nonmutating function mutates parameter s"
}

//constable:nonmutating
func MutatesMap(m map[string]int) {
	m["x"] = 1 // want "//constable:nonmutating function mutates parameter m"
}

//constable:nonmutating
func DeletesMapEntry(m map[string]int) {
	delete(m, "x") // want "//constable:nonmutating function deletes from map parameter m"
}

//constable:nonmutating
func MutatesLocal() int {
	x := 0
	x++
	return x
}

//constable:nonmutating
func MutatesLocalPointer() int {
	x := 0
	p := &x
	*p = 1
	return x
}

type Counter struct{ n int }

//constable:nonmutating
func (c *Counter) Inc() {
	c.n++ // want "//constable:nonmutating method mutates receiver c"
}

type Count struct{ n int }

//constable:nonmutating
func (c Count) Increment() Count {
	c.n++
	return c
}

type Buffer struct{ data []byte }

//constable:nonmutating
func (b Buffer) ClearFirstByte() {
	b.data[0] = 0 // want "//constable:nonmutating method mutates receiver b"
}

type Holder struct{ p *int }

//constable:nonmutating
func (h Holder) MutatesPointedValue() {
	*h.p = 1 // want "//constable:nonmutating method mutates receiver h"
}

type MHolder struct{ m map[string]int }

//constable:nonmutating
func (h MHolder) MutatesMapField() {
	h.m["x"] = 1 // want "//constable:nonmutating method mutates receiver h"
}

//constable:nonmutating
func (h Holder) ReplacesPointerInCopy() {
	h.p = nil
}

//constable:nonmutating
func MutatesNestedSlice(ss [][]int) {
	ss[0][0] = 1 // want "//constable:nonmutating function mutates parameter ss"
}

//constable:nonmutating
func DeletesLocalMap() {
	m := map[string]int{}
	delete(m, "x")
}

//constable:nonmutating
func MutatesDoublePointer(pp **int) {
	**pp = 1 // want "//constable:nonmutating function mutates pointer parameter pp"
}

type Element struct{ n int }

//constable:nonmutating
func MutatesFieldOfSliceElement(elements []Element) {
	elements[0].n = 1 // want "//constable:nonmutating function mutates parameter elements"
}

//constable:nonmutating
func MutatesNestedSliceElementField(ss [][]Element) {
	ss[0][0].n = 1 // want "//constable:nonmutating function mutates parameter ss"
}

type Inner struct{ data []byte }

type Outer struct{ inner Inner }

//constable:nonmutating
func MutatesFieldThroughDeref(p *Outer) {
	(*p).inner.data[0] = 0 // want "//constable:nonmutating function mutates pointer parameter p"
}

//constable:nonmutating
func (o Outer) ClearNestedField() {
	o.inner.data[0] = 0 // want "//constable:nonmutating method mutates receiver o"
}

//constable:nonmutating
func (o Outer) ReplacesNestedField() {
	o.inner = Inner{}
}
