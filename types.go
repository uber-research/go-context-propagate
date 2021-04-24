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
	"go/ast"
	"go/token"
	"go/types"
	cg "golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

// replacementInfo contains information needed to replace a given leaf
// function call.
type replacementInfo struct {
	// newName is the new name for the function (optional).
	newName string
	// argPos is position of the context parameter (optional -
	// defaults to first position).
	argPos int
	// ctxImports is information about import that needs to be
	// auto-injected for the code to build (optiona).
	ctxImports map[string]string // import path -> alias (optionally empty string)
	// ctxRegExpr is the string defining context expresion (with
	// wildcards) to be used as argument for the call (optional -
	// defaults to the context variable itself).
	ctxRegExpr string
	// ctxExpr is the same as ctxRegExpr but with wildcards resolved
	// (expression ready for injection).
	ctxExpr string
}

// pkgInfo maps package paths to package names defined on these paths.
type pkgInfo map[string]map[string]bool // pkgPath -> pkgName -> exists-on-the-path
// typeInfo maps type names to package info where the types are defined.
type typeInfo map[string]pkgInfo // type -> pkgInfo
// fnInfo maps function/method names to the receiver info and then to
// package info where they are defined.
type fnInfo map[string]map[string]pkgInfo // func/method -> receiver -> pkgInfo
// fnReplacementInfo maps function/method names to the receiver info
// and then to the information for replacing this function call with
// its context-aware version.
type fnReplacementInfo map[string]map[string]*replacementInfo // func/method -> receiver -> replacementInfo

type jsonConfig struct {
	// CtxPkgPath is package path for the context type.
	CtxPkgPath string
	// CtxPkgName is package name for the context type.
	CtxPkgName string
	// CtxPkgAlias is package alias for the context type (optional).
	CtxPkgAlias string
	// CtxParamName is context parameter name (to be used in function
	// definitions and function calls).
	CtxParamName string
	// CtxParamType is context type.
	CtxParamType string
	// CtxParamInvalid is an expression defining "invalid" context (to
	// be used when propagated context is unavailable).
	CtxParamInvalid string
	// LibPkgPath is path to library where "leaf" functions are
	// defined.
	LibPkgPath string
	// LibPkgName is a name of the library where "leaf" functions are
	// defined.
	LibPkgName string
	// interface defining "leaf" functions (optional - not used if
	// "leaf" functions specified by describing their definitions).
	LibIface string

	// The following describe custom context 0 the one from which a
	// "regular" context can be extracted via ctxCustomExprExtract.

	// CtxCustomPkgPath is custom context path.
	CtxCustomPkgPath string
	// CtxCustomPkgName is custom context path.
	CtxCustomPkgName string
	// CtxCustomParamType is custom context type.
	CtxCustomParamType string
	// CtxCustomExprExtranct is custom context extraction expression.
	CtxCustomExprExtract string

	// ExtPkgPaths are paths where external packages reside.
	ExtPkgPaths []string
	// ExtEmbedTypes are external types (currently meant for structs
	// only) that are embedded in user types (methods on these user
	// types should not have their signatures changed).
	ExtEmbedTypes typeInfo
	// LibFns are "leaf" functions definitions.
	LibFns fnReplacementInfo
	// PropagationStops are functions where upward propagating context
	// should stop.
	PropagationStops fnInfo
	// LoadPaths are source code paths.
	LoadPaths []string
}

// uniquePosInfo represents position info across different file
// sets. See config.fsets field definition below to see why this is
// needed.
type uniquePosInfo struct {
	pos  token.Pos
	fset *token.FileSet
}

// debugInfo represents debugging information collected during
// analysis and transformation process.
type debugInfo struct {
	// Excluded is a list of packages excluded from the analysis
	// (e.g. due to build problems).
	Excluded []string
	// Warnings is a list of warnings to be reported to the tool user.
	Warnings []map[string]string
}

// config is data shared by both the analysis and transformation
// phases.
type config struct {
	*jsonConfig

	// debugLevel is debugging level (0 - no debugging info at all).
	debugLevel int

	// debugData is debug data collected during analysis to either be
	// printed or stored into a file.
	debugData debugInfo

	// filePrefix is a prefix of the source files path.
	filePrefix string

	// commonCallReplacement represents call replacement info for
	// majority of functions taking context as the first argument
	// (unless overridden due to how surrounding code looks like).
	commonCallReplacement replacementInfo

	// nilCallReplacement represents call replacement info for all
	// functins taking "nil" (invalid) context as the first argument.
	nilCallReplacement replacementInfo

	// libIfaces contains interface definitions specifying methods
	// that need their signatures changed (describes by "libIface"
	// field in the JSON config file). The reason it is an array is
	// that sometimes (I think it's a bug), despite providing one load
	// path, the packages.Load function loads two packages (with two
	// interface definitions) from the same sources. If we are
	// unlucky, when checking for methods implementing the interface,
	// we will hit the "wrong" one for which types.Implements fill
	// fail
	libIfaces []*types.Interface

	// The following represent context param type qualified with pkg
	// name and with both path and name, respectively.
	ctxParamTypeWithPkgAlias    string
	ctxParamTypeWithPkgPathName string

	// ctxCustomParamTypeWithPkgPathName is custom context param type
	// qualified with both path and name.
	ctxCustomParamTypeWithPkgPathName string

	// ifaces is a list of all interfaces found in the source code.
	ifaces map[*types.Interface]*types.Package

	// extRecvTypes contains receiver types that contain one of the
	// embedded external types specified in the config file.
	extRecvTypes map[*types.Struct]bool

	// fsets is a mapping from packages to fsets for the case when we
	// have single program but multiple fset-s due to inremental
	// package loading for large code.
	fsets map[*types.Package]*token.FileSet

	// largeCode is true if incremental package loading was used.
	largeCode bool

	// initial is a list of packages loaded by the tool.
	initial []*packages.Package

	// The following are computed during analysis phase and used in
	// the transformation phase for AST rewriting.

	// fnVisited are functions that need rewriting.
	fnVisited map[uniquePosInfo]int
	// callSites are call sites that need an extra context argument.
	callSites map[uniquePosInfo]*replacementInfo
	// callSitesRenamed are call sites whose function names need to be
	// renamed.
	callSitesRenamed map[uniquePosInfo]string

	// ifaceModified are interface methods that need rewriting.
	ifaceModified map[*types.Interface]map[string]bool // iface -> iface methods

	// fnParamsVisited identifies positions of parameters whose type
	// is a function that needs a context injection in its definition.
	fnParamsVisited map[uniquePosInfo]bool

	// renameParamsVisited identifies of context parameters with no
	// name or with "_" name that need to be turned into named
	// parameters.
	renameParamsVisited map[uniquePosInfo]bool
}

// transformerConfig is data used in the transformation stage.
type transformerConfig struct {
	*config

	// The following are used when transforming a single AST and reset
	// before each AST transformation.

	// currentPkg is packege a code in a given AST belongs to.
	currentPkg *packages.Package
	// existingImports contains information about existing import
	// statements (the key is import path, and the value is an
	// optional alias - otherwise empty string).
	existingImports map[string]string // importPath -> importAlias
	// newImports contains information about new import statements to
	// be injected as a result of adding new code during
	// transformation (the key is import path, and the value is an
	// optional alias - otherwise empty string).
	newImports map[string]string // importPath -> importAlias
	// modified keeps track of whether a given AST has been modified
	// at all during transformation.
	modified bool

	// astIfaceModified collects information about interfaces modified
	// across traversing all AST traversals.
	astIfaceModified map[*ast.InterfaceType]bool

	// The following count different types of transformations that
	// actually take place when transforming all ASTs.
	ifaceMethodModifiedNum int
	astNamedModifiedNum    int
	astParamsModifiedNum   int
	astCallsModifiedNum    int
	astSigsModifiedNum     int
	astDefsModifiedNum     int
}

// analyzerConfig is data used in the analysis stage.
type analyzerConfig struct {
	*config

	// prog is the analyzed application (service).
	prog *ssa.Program
	// graph is the call graph representing the analyzed application
	// (service).
	graph *cg.Graph

	// mapAndSliceFuncs contains per-package signatures used in map
	// and slice construction so that we can avoid modifying functions
	// with these signatures.
	mapAndSliceFuncs map[*ssa.Package]map[*types.Signature]bool
}
