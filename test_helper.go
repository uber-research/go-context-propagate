// Copyright (c) 2021 Uber Technologies, Inc.
//
// Licensed under the Uber Non-Commercial License (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at the root directory of this project.
//
// See the License for the specific language governing permissions and
// limitations under the License.

package propagate

import (
	"bytes"
	"go/ast"
	"go/format"
	"golang.org/x/tools/go/packages"
	"io/ioutil"
	"strings"
	"testing"
)

// validateOutput compares generated output with expected output and
// type-checks expected output.
func validateOutput(t *testing.T, results map[*packages.Package]map[*ast.File]int, loadPath string, recompile bool) {
	fail := false
	if len(results) == 0 {
		t.Log("no files have been refactored")
		t.FailNow()
	}
	for p, nodes := range results {
		for n, ind := range nodes {
			expectedPath := strings.ReplaceAll(p.CompiledGoFiles[ind], "testdata/src", "testdata/src/expected")
			refactoredBuf, err := ioutil.ReadFile(expectedPath)
			if err != nil {
				t.Log("could not read file containing expected refactored output: " + expectedPath)
				t.FailNow()
			}
			var buf bytes.Buffer
			err = format.Node(&buf, p.Fset, n)
			if err != nil {
				t.Log("could not format refactored AST")
				t.FailNow()
			}
			if !bytes.Equal(refactoredBuf, buf.Bytes()) {
				t.Log("refactored file and expected refactored output have different content")
				t.Log("REFACTORED\n" + string(buf.Bytes()))
				t.Log("EXPECTED\n" + string(refactoredBuf))
				// set flag instead of failing so that we can observe more than one mismatch
				fail = true
			}
		}
	}
	if fail {
		t.FailNow()
	}
	if recompile {
		cfg := &packages.Config{Mode: packages.LoadAllSyntax, Tests: true}
		loaded, err := packages.Load(cfg, "expected/"+loadPath)
		if err != nil {
			t.Log("could not load refactored packages")
			t.Log(err)
			t.FailNow()
		}
		for _, p := range loaded {
			if len(p.Errors) > 0 {
				t.Log("refactored package loading errors")
				for _, e := range p.Errors {
					t.Log(e)
				}
				t.FailNow()
			}
		}
	}
}
