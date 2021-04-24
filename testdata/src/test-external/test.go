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

type OuterStruct struct {
	lib_helper.EmbedStruct
}

type ReceiverStructExt struct {
}

type ReceiverStructReturn struct {
}

// method implementing an external interface - no context parameter injection
func (*ReceiverStructExt) Foo() bool {
	return lib.A()
}

// another method implementing an external interface - no context parameter injection
func (*ReceiverStructReturn) Baz(p bool) bool {
	return lib.B(p)
}

// function to be passed as parameter to external function - no context parameter injection
func bar() bool {
	return lib.A()
}

// method whose receiver type embeds explicitly specifed external type - no context parameter injection
// (useful for preventing context injection to methods used by certain frameworks, e.g. for testing)
func (*OuterStruct) baz() bool {
	return lib.A()
}

// interface method Foo has to be actually called via interface
// (passed as parameter) to be prevented from having context parameter
// injected (otherwise Foo's definition will get a context parameter)
func callExt(inter lib_helper.LibCallInter) bool {
	return inter.Foo()
}

// returns an interface to be used at a call site
func callReturn() lib_helper.ReturnInter {
	return &ReceiverStructReturn{}
}

func main() {
	ext := ReceiverStructExt{}
	callExt(&ext)
	// interface method Baz has to be called via interface (returned from
	// a function) to be prevented from having context parameter injected
	// (otherwise Baz's definition will get a context parameter)
	callReturn().Baz(true)
	lib_helper.Register(bar)
	o := OuterStruct{}
	o.baz()
}
