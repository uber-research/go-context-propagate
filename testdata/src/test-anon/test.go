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

type ParamFn func() bool

func Foo(f ParamFn) bool {
	return f()
}

func main() {

	// anonymous function gets context from the closure so its
	// signature does not have to be modified
	f := func() bool {
		return lib.A()
	}
	f()

	// goroutines get context from the closure so Foo's signature
	// (and anonymous functions signature) does not have to be modified
	go Foo(func() bool {
		return lib.A()
	})

	// deferred functions get context from the closure so Foo's signature
	// (and anonymous functions signature) does not have to be modified
	defer Foo(func() bool {
		return lib.A()
	})

	// library function executed via goroutine should get context parameter
	go lib.B(true)

	// deferred library function should get context parameter
	defer lib.C(true)
}
