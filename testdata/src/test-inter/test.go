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

// interface to get a method augmented with context parameter
type CallInter interface {
	Foo() bool
}

type ReceiverStruct struct {
}

type AnotherReceiverStruct struct {
}

// method whose context augmentation triggers interface modificaction
func (ReceiverStruct) Foo() bool {
	return lib.A()
}

// method which needs to get additional context parameter as a result of interface modification
func (AnotherReceiverStruct) Foo() bool {
	return true
}
