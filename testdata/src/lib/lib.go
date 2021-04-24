// Copyright (c) 2021 Uber Technologies, Inc.
//
// Licensed under the Uber Non-Commercial License (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at the root directory of this project.
//
// See the License for the specific language governing permissions and
// limitations under the License.

package lib

type ContextStruct struct {
	Context
	P bool
}

type Context interface {
	Val() bool
}

type Rec struct {
	R bool
}

func (ctx ContextStruct) Val() bool {
	return ctx.P
}

func Background() Context {
	return ContextStruct{P: true}
}

func Copy(ctx Context) Context {
	return ContextStruct{P: ctx.Val()}
}

func A() bool {
	return true
}

func CtxA(ctx Context) bool {
	return ctx.Val()
}

func B(b bool) bool {
	return b
}

func CtxB(ctx Context, b bool) bool {
	return ctx.Val() || b
}

func C(b bool) bool {
	return true
}

func CtxC(ctx Context, b bool) bool {
	return ctx.Val() || b
}

func D(b1 bool, b2 bool) bool {
	return b1 || b2
}

func CtxD(b1 bool, ctx Context, b2 bool) bool {
	return ctx.Val() || b1 || b2
}

func E(b1 bool, b2 bool) bool {
	return b1 || b2
}

func CtxE(b1 bool, b2 bool, ctx Context) bool {
	return ctx.Val() || b1 || b2
}

func (r *Rec) F() bool {
	return r.R
}

func (r *Rec) CtxF(ctx Context) bool {
	return r.R || ctx.Val()
}

func G() bool {
	return true
}

func CtxG(ctx Context) bool {
	return ctx.Val()
}

func H() bool {
	return true
}

func CtxH(ctx Context) bool {
	return ctx.Val()
}

func I() bool {
	return true
}

func CtxI(ctx Context) bool {
	return ctx.Val()
}

func J() bool {
	return true
}

func CtxJ(ctx Context) bool {
	return ctx.Val()
}
