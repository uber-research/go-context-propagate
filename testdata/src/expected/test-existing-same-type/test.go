// Copyright (c) 2021 Uber Technologies, Inc.
//
// Licensed under the Uber Non-Commercial License (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at the root directory of this project.
//
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import (
	"lib"
	"lib_helper"
)

// lib.A() should pick up existing context
func FooA(ctx lib.Context) bool {
	return lib.CtxA(ctx)
}

// lib.B() should pick up existing context with the right (non-default) name
func FooB(existingCtx lib.Context, p bool) bool {
	return lib.CtxB(existingCtx, p)
}

// lib.C() should get another context injected as the existing one
// is not in the first position
func FooC(ctx lib.Context, p bool, existingCtx lib.Context) bool {
	return lib.CtxC(ctx, p)
}

// lib.D() should pick up existing context with the right (non-default) name
// for the context expression
func FooD(existingCtx lib.Context, p1 bool, p2 bool) bool {
	return lib.CtxD(p1, lib_helper.Ident(existingCtx), p2)
}

// _ should be replaced with the actual context parameter and used in call to lib.E
func FooE(ctx lib.Context, p1 bool, p2 bool) bool {
	return lib.CtxE(p1, p2, ctx)
}

// context parameter should get the name and this name should be used in call to lib.G
func FooG(ctx lib.Context) bool {
	return lib.CtxG(ctx)
}
