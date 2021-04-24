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

type ParamFn func(ctx lib.Context) bool

// function to be passed as parameter
func Foo(ctx lib.Context) bool {
	return lib.CtxA(ctx)
}

// function whose parameter type is meant to change
// to accomodate context
func Bar(ctx lib.Context, f ParamFn) bool {
	return f(ctx)
}

func main() {
	ctx := lib.Background()
	Bar(ctx, Foo)
}
