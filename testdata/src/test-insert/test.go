// Copyright (c) 2021 Uber Technologies, Inc.
//
// Licensed under the Uber Non-Commercial License (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at the root directory of this project.
//
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import "lib"

// insert context in default (first) position
func FooA() bool {
	return lib.A()
}

// insert context in default (first) position
func FooB(p bool) bool {
	return lib.B(p)
}

// insert context in the first position
func FooC(p bool) bool {
	return lib.C(p)
}

// insert context in the second position
func FooD(p1 bool, p2 bool) bool {
	return lib.D(p1, p2)
}

// insert context in the last position
func FooE(p1 bool, p2 bool) bool {
	return lib.E(p1, p2)
}

// insert context to a method
func FooF() bool {
	r := lib.Rec{R: true}
	return r.F()
}

// insert context expression
func FooG() bool {
	return lib.G()
}

func bar(p bool) bool {
	return p
}

func baz(p bool) bool {
	return p
}

func qux() {
	// make sure that context argument is inserted for functions
	// called as parameters
	bar(FooA())
	baz(FooB(true))
}
