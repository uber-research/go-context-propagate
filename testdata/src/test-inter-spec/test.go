// Copyright (c) 2021 Uber Technologies, Inc.
//
// Licensed under the Uber Non-Commercial License (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at the root directory of this project.
//
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import "lib_helper"

// tests "leaf" functions specified in an external interface

type InterSpecRec struct {
}

func FooZ(rec lib_helper.SpecInter) bool {
	return rec.Z()
}

func (r InterSpecRec) Z() bool {
	return true
}

func main() {
	FooZ(InterSpecRec{})
}
