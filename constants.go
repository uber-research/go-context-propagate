// Copyright (c) 2021 Uber Technologies, Inc.
//
// Licensed under the Uber Non-Commercial License (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at the root directory of this project.
//
// See the License for the specific language governing permissions and
// limitations under the License.

package propagate

// argBytesLimit establishes the total max length load paths argument
// can have.
const argBytesLimit = 200000

// The following describe different call graph construction
// algorithms.
const (
	cfgCHA = iota
	cfgRTA
	cfgPT
)

// cfgType defines the currently used call graph construction
// algorithm.
const cfgType = cfgRTA

// The following describe various types of wildcards used in the
// config file.
const (
	ctxWildcard       = "<?CTX?>"
	ctxCustomWildcard = "<?CTX_CUSTOM?>"
	ctxPrefWildcard   = "<?CTX_PREF?>"
	pathWildCard      = "<?PATH?>"
	aliasWildCard     = "<?ALIAS1?>"
)

// The following describe argument types of functions in the testing
// harness.
const (
	testingTypeT = "*testing.T"
	testingTypeM = "*testing.M"
)

// The following describe different different function types in fnVisited map.
const (
	regularFn = iota
	freshCtxFn
	containerSig
	extFn
	extPkg
	extRecv
)
