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
func FooA(ctx lib.Context) bool {
	return lib.CtxA(ctx)
}

// insert context in default (first) position
func FooB(ctx lib.Context, p bool) bool {
	return lib.CtxB(ctx, p)
}

// insert context in the first position
func FooC(ctx lib.Context, p bool) bool {
	return lib.CtxC(ctx, p)
}

// insert context in the second position
func FooD(ctx lib.Context, p1 bool, p2 bool) bool {
	return lib.CtxD(p1, ctx, p2)
}

// insert context in the last position
func FooE(ctx lib.Context, p1 bool, p2 bool) bool {
	return lib.CtxE(p1, p2, ctx)
}

// insert context to a method
func FooF(ctx lib.Context) bool {
	r := lib.Rec{R: true}
	return r.CtxF(ctx)
}

// insert context expression
func FooG(ctx lib.Context) bool {
	return lib.CtxG(lib.Copy(ctx))
}

func bar(p bool) bool {
	return p
}

func baz(p bool) bool {
	return p
}

func qux(ctx lib.Context) {
	// make sure that context argument is inserted for functions
	// called as parameters
	bar(FooA(ctx))
	baz(FooB(ctx, true))
}
