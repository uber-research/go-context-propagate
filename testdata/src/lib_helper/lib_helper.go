// Copyright (c) 2021 Uber Technologies, Inc.
//
// Licensed under the Uber Non-Commercial License (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at the root directory of this project.
//
// See the License for the specific language governing permissions and
// limitations under the License.

package lib_helper

import "lib"

type SpecInter interface {
	Z() bool
}

type EmbedStruct struct {
	P bool
}

type LibCallInter interface {
	Foo() bool
}

type ReturnInter interface {
	Baz(bool) bool
}

type Context interface {
}

type CustomContextStruct struct {
	lib.Context
}

type CustomContext interface {
	lib.Context
}

func Ident(ctx lib.Context) lib.Context {
	return ctx
}

func Register(f func() bool) bool {
	return f()
}

// dummy function so that we can call to this package
func Foo(p bool) bool {
	return p
}
