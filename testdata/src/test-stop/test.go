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
	"testing"
)

type StopTestStruct struct {
}

// test propagation stop at main function level
func main() {
	lib.A()
}

// test propagation stop for test functions
func TestA(t *testing.T) {
	lib.A()
}

// test propagation stop for TestMain function
func TestMain(m *testing.M) {
	lib.A()
}

// helper function to add additional call to the chain
func bar() bool {
	return lib.A()
}

// test propagation stop for explicitly specified function
func FooFn() bool {
	ctx := lib.Background()
	return bar() || ctx.Val()
}

// test propagation stop for explicitly specified function
func (StopTestStruct) FooMethod() bool {
	ctx := lib.Background()
	return bar() || ctx.Val()
}

// TODO: implement propagation stop for a single-call chain
