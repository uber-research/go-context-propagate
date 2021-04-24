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
	context "lib"
	lib "lib_helper"
)

// FooACaller should get context with an alias specified in the config file (context.Context)
// rather than "normal" package name (lib.Context)
func FooACaller(ctx context.Context) bool {
	return FooA(ctx) || lib.Foo(true)
}

// FooA should get a "regular" context extracted from custom context as the argument
// (as a bonus extraction should use the right package alias for "regular" context
// as specified by the injected import - context.Context instead of lib.Context)
func FooAContextCaller(ctxCustom lib.CustomContext) bool {
	return FooA(ctxCustom.(context.Context))
}
