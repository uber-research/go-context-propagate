// Copyright (c) 2021 Uber Technologies, Inc.
//
// Licensed under the Uber Non-Commercial License (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at the root directory of this project.
//
// See the License for the specific language governing permissions and
// limitations under the License.

package test

// both caller and callee should get context parameter, but
// this really tests insertion of import for the package containing context definition
func FooACaller() bool {
	return FooA()
}
