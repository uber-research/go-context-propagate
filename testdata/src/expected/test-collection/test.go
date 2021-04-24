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

type FuncStruct struct {
	f func(bool) bool
}

// function to be used in a map (directly) - no context parameter injection
func foo(p bool) bool {
	ctx := lib.Background()
	return lib.CtxA(ctx) || p
}

// function to be use in a map (indirectly) - no context parameter injection
func bar(p bool) bool {
	ctx := lib.Background()
	return lib.CtxA(ctx) || p
}

// function to be used in an array - no context parameter injection
func baz(p1 bool, p2 bool) bool {
	ctx := lib.Background()
	return lib.CtxA(ctx) || p1 || p2
}

func main() {
	s := FuncStruct{f: foo}
	m := map[int]func(bool) bool{
		7:  s.f,
		42: bar,
	}
	a := []func(bool, bool) bool{
		baz,
	}
	m[7](true)
	m[42](false)
	a[0](true, false)
}
