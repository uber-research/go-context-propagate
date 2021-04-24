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

func FooA() bool {
	return lib.A()
}

// FooB should get "regular" context as first parameter
// since custom context parameter isn't in the first position itself
func FooB(p bool, ctxCustom lib_helper.CustomContext) bool {
	return lib.B(p)
}

// lib.C should get a "regular" context extracted from custom context as the argument
// to then be passed to context expression specified in config file rather than being used directly
func FooC(ctxCustom lib_helper.CustomContext, p bool) bool {
	return lib.C(p)
}
