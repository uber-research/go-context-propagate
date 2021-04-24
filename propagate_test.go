// Copyright (c) 2021 Uber Technologies, Inc.
//
// Licensed under the Uber Non-Commercial License (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at the root directory of this project.
//
// See the License for the specific language governing permissions and
// limitations under the License.

package propagate

import "testing"

func TestAnon(t *testing.T) {
	loadPath := "test-anon"
	srcPaths := []string{loadPath}
	results := propagate("testdata/config/test.json", "", srcPaths, 0)
	validateOutput(t, results, loadPath, true)
}

func TestCollection(t *testing.T) {
	loadPath := "test-collection"
	srcPaths := []string{loadPath}
	results := propagate("testdata/config/test.json", "", srcPaths, 0)
	validateOutput(t, results, loadPath, true)
}

func TestExternal(t *testing.T) {
	loadPath := "test-external"
	srcPaths := []string{loadPath}
	results := propagate("testdata/config/test_external.json", "", srcPaths, 0)
	validateOutput(t, results, loadPath, true)
}

func TestExisting(t *testing.T) {
	loadPath := "test-existing"
	srcPaths := []string{loadPath}
	results := propagate("testdata/config/test_existing.json", "", srcPaths, 0)
	validateOutput(t, results, loadPath, true)
}

func TestExistingSameType(t *testing.T) {
	loadPath := "test-existing-same-type"
	srcPaths := []string{loadPath}
	results := propagate("testdata/config/test_existing_same_type.json", "", srcPaths, 0)
	validateOutput(t, results, loadPath, true)
}

func TestFnParam(t *testing.T) {
	loadPath := "test-fn-param"
	srcPaths := []string{loadPath}
	results := propagate("testdata/config/test.json", "", srcPaths, 0)
	validateOutput(t, results, loadPath, true)
}

func TestImport(t *testing.T) {
	loadPath := "test-import"
	srcPaths := []string{loadPath}
	results := propagate("testdata/config/test_import.json", "", srcPaths, 0)
	validateOutput(t, results, loadPath, true)
}

func TestInsert(t *testing.T) {
	loadPath := "test-insert"
	srcPaths := []string{loadPath}
	results := propagate("testdata/config/test.json", "", srcPaths, 0)
	validateOutput(t, results, loadPath, true)
}

func TestInter(t *testing.T) {
	loadPath := "test-inter"
	srcPaths := []string{loadPath}
	results := propagate("testdata/config/test.json", "", srcPaths, 0)
	validateOutput(t, results, loadPath, true)
}

func TestStop(t *testing.T) {
	loadPath := "test-stop"
	srcPaths := []string{loadPath}
	results := propagate("testdata/config/test_stop.json", "", srcPaths, 0)
	validateOutput(t, results, loadPath, true)
}

func TestInterSpec(t *testing.T) {
	loadPath := "test-inter-spec"
	srcPaths := []string{loadPath}
	results := propagate("testdata/config/test_inter_spec.json", "", srcPaths, 0)
	// do not recompile transformed code as it would require manual
	// change of import to point to a new (context aware) interface
	// instead of the old (not-context aware one)
	validateOutput(t, results, loadPath, false)
}
