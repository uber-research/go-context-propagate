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
	"go/token"
	"go/types"
	"strconv"
	"strings"
)

// getUniquePosPkg returns unique position within a given package.
func (cfg *config) getUniquePosPkg(pkg *types.Package, pos token.Pos) uniquePosInfo {
	if cfg.largeCode {
		return uniquePosInfo{pos, cfg.fsets[pkg]}
	}
	return uniquePosInfo{pos, nil}
}

// isPkgExternal determines if a package external that is if its path is:
// - the same as that of the package where context is defined
// - the same as that of the package where leaf functions are defined
// - when it's on the on the explicit list of external package paths.
func (cfg *config) isPkgExternal(pkgPath string) bool {
	if strings.HasPrefix(pkgPath, cfg.CtxPkgPath) {
		return true
	}
	if strings.HasPrefix(pkgPath, cfg.LibPkgPath) {
		return true
	}
	for _, extPath := range cfg.ExtPkgPaths {
		if strings.HasPrefix(pkgPath, extPath) {
			return true
		}
	}
	return false
}

// writeWarning writes a warning, either to std out or as a command to
// script file issuing inline comments.
func (cfg *config) writeWarning(fset *token.FileSet, pos token.Pos, msg string) {
	p := fset.Position(pos)
	if cfg.debugLevel > 0 {
		m := make(map[string]string)
		m["file"] = strings.TrimPrefix(fset.File(pos).Name(), cfg.filePrefix)
		m["line"] = strconv.Itoa(p.Line)
		m["msg"] = msg
		cfg.debugData.Warnings = append(cfg.debugData.Warnings, m)
	}
}
