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
	"fmt"
	"go/token"
	"go/types"
	cg "golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
	"log"
	"strconv"
	"strings"
)

// analyze is the main driver function of the analysis phase.
func (cfg *analyzerConfig) analyze() {

	// collect some preliminary information from the code base that is used later on during analysis
	cfg.collectInterfacesAndThirdPartyEmbeds()
	cfg.collectCollectionFnsAndMarkExternalInterfaceFns()
	cfg.markExternalParamFns()
	// start building work list of functions that need to be modified using "leaf" API calls
	nodesWorkList, nodesVisited := cfg.processLeafCalls()
	// process remaining items on the work list
	cfg.collect(nodesWorkList, nodesVisited)

	// Visit all functions again to see if any of the interface-type
	// parameters takes a value of type that is not context-aware yet.
	// Iterate until no new interfaces are added to the set that needs
	// to be processed.
	namedModified := make(map[*types.Named]bool)
	added := true
	for added {
		added = cfg.collectIfaces(namedModified)
		cfg.collectNamedTypes(namedModified)
	}

}

// collectInterfacesAndThirdPartyEmbeds gathers information about all
// defined interfaces and all struct types that embed a third-party
// struct.
func (cfg *analyzerConfig) collectInterfacesAndThirdPartyEmbeds() {
	cfg.ifaces = make(map[*types.Interface]*types.Package)
	cfg.extRecvTypes = make(map[*types.Struct]bool)
	for _, pkg := range cfg.initial {
		for _, name := range pkg.Types.Scope().Names() {
			typ := pkg.Types.Scope().Lookup(name).Type().Underlying()
			// collect info about all interfaces
			if i, ok := typ.(*types.Interface); ok {
				cfg.ifaces[i] = pkg.Types
				if pkg.PkgPath == cfg.LibPkgPath && pkg.Name == cfg.LibPkgName {
					if name == cfg.LibIface {
						cfg.libIfaces = append(cfg.libIfaces, i)
					}
				}
			}
			// collect info about all structs that embed a third-party struct type specified in the config file
			s, ok := typ.(*types.Struct)
			if !ok {
				// not a struct
				continue
			}
			for i := 0; i < s.NumFields(); i++ {
				f := s.Field(i)
				if !f.Embedded() {
					continue
				}
				named, ok := f.Type().(*types.Named)
				if !ok {
					// not a named type
					continue
				}
				pkgPaths, exists := cfg.ExtEmbedTypes[named.Obj().Name()]
				if !exists {
					// named type is not external embedded type
					continue
				}
				pkgNames, exists := pkgPaths[named.Obj().Pkg().Path()]
				if !exists {
					// named type is not external embedded type
					// (package path mismatch)
					continue
				}

				if pkgNames[named.Obj().Pkg().Name()] {
					cfg.extRecvTypes[s] = true
				}
			}
		}
	}
}

// collectCollectionFnsAndMarkExternalInterfaceFns collects signatures
// of functions that can be stored in collections and marks functions
// that implement external interfaces as being used externally.
func (cfg *analyzerConfig) collectCollectionFnsAndMarkExternalInterfaceFns() {
	// The two pieces functionality are combined for performance
	// reasons as they require iterating over all instructions.
	for f, _ := range cfg.graph.Nodes {
		if f != nil && f.Package() != nil && f.Blocks == nil {
			// not a "concrete" (with a body) function
			continue
		}
		for _, b := range f.Blocks {
			for _, inst := range b.Instrs {
				if mm, ok := inst.(*ssa.MakeMap); ok {
					m := mm.Type().Underlying().(*types.Map) // this never fails
					cfg.addCollectionFn(inst, m.Key())
					cfg.addCollectionFn(inst, m.Elem())
				} else if ms, ok := inst.(*ssa.MakeSlice); ok {
					s := ms.Type().(*types.Slice) // this never fails
					cfg.addCollectionFn(inst, s.Elem())
				} else if a, ok := inst.(*ssa.Alloc); ok {
					elType := a.Type().Underlying().(*types.Pointer).Elem()
					if s, ok := elType.(*types.Slice); ok {
						cfg.addCollectionFn(inst, s.Elem())
					} else if ar, ok := elType.(*types.Array); ok {
						cfg.addCollectionFn(inst, ar.Elem())
					}
				} else if mi, ok := inst.(*ssa.MakeInterface); ok {
					// mark all methods that implement third-party
					// interfaces as such to avoid modifying their
					// signatures
					named, ok := mi.Type().(*types.Named)
					if !ok {
						// not a named (interface) type
						continue
					}
					for _, pkgPath := range cfg.ExtPkgPaths {
						if named.Obj().Pkg() == nil || !strings.HasPrefix(named.Obj().Pkg().Path(), pkgPath) {
							// interface type is not an external one
							continue
						}
						methodSet := cfg.prog.MethodSets.MethodSet(mi.X.Type())
						for j := 0; j < methodSet.Len(); j++ {
							sel := methodSet.At(j)
							fun := cfg.prog.MethodValue(sel)
							if fun != nil {
								cfg.fnVisited[cfg.getUniquePosSSAFn(fun, fun.Pos())] = extFn
							}
						}
					}
				}
			}
		}
	}
}

// addCollectionFn records signature of a function used in a
// collection.
func (cfg *analyzerConfig) addCollectionFn(inst ssa.Instruction, typ types.Type) {
	parent := inst.Parent()
	if parent == nil {
		// no parent that can be used to identify the
		// package where function declaration has been
		// used in a collection
		return
	}
	if sig, ok := typ.Underlying().(*types.Signature); ok {
		mapSigToPkg(cfg.mapAndSliceFuncs, parent.Pkg, sig)
	}
}

// markParamAsExternalFn finds functions from external packages that
// take other functions as parameters and marks these other functions
// as being used externally.
func (cfg *analyzerConfig) markExternalParamFns() {
	for f, n := range cfg.graph.Nodes {
		if f == nil || f.Package() == nil {
			// not an actual function
			continue
		}
		for _, pkgPath := range cfg.ExtPkgPaths {
			if !strings.HasPrefix(f.Package().Pkg.Path(), pkgPath) {
				// not an external function
				continue
			}
			sig := f.Signature
			params := sig.Params()
			for i := 0; i < params.Len(); i++ {
				p := params.At(i)
				_, ok_func := p.Type().(*types.Signature)
				_, ok_iface := p.Type().(*types.Interface)

				if ok_func || ok_iface {
					for _, caller := range n.In {
						arg := getActualCallArg(caller.Site.Common(), i)
						cfg.markParamAsExternalFn(&arg)
					}
				}
			}
			// handle variadic signatures
			if params.Len() > 0 && sig.Variadic() {
				for _, caller := range n.In {
					common := caller.Site.Common()

					// this is fairly fragile as it depends on the way varArgs are generated
					// into the instruction stream when building SSA representation
					// but I can't figure out a better way
					getVarArgs := func(vals *[]*ssa.Value) {
						arg := getActualCallArg(common, params.Len()-1)
						s, ok := arg.(*ssa.Slice)
						if !ok {
							// argument is not a variadic (it's not a slice)
							return
						}
						b := s.Block() // basic block to which slice instruction belongs
						instrs := b.Instrs
						for ind, inst := range instrs {
							// find instruction that indexes a store of an element to the slice
							ia, ok := inst.(*ssa.IndexAddr)
							if !ok {
								// not an index instruction
								continue
							}
							if ia.X != s.X {
								// index instruction but for the wrong slice
								continue
							}
							// next instruction actually represents a stored value - record it
							conv := instrs[ind+1]
							if ct, ok := conv.(*ssa.ChangeType); ok {
								*vals = append(*vals, &ct.X)
							} else if ci, ok := conv.(*ssa.ChangeInterface); ok {
								*vals = append(*vals, &ci.X)
							} else if mi, ok := conv.(*ssa.MakeInterface); ok {
								*vals = append(*vals, &mi.X)
							} else if c, ok := conv.(*ssa.Convert); ok {
								*vals = append(*vals, &c.X)
							}
						}
					}

					var varArgs []*ssa.Value
					getVarArgs(&varArgs)
					for _, arg := range varArgs {
						cfg.markParamAsExternalFn(arg)
					}
				}
			}
		}
	}
}

// processLeafCalls marks "leaf" API calls for addition of the context
// argument (and optional renaming) and start processing their callers
// transitively.
func (cfg *analyzerConfig) processLeafCalls() ([]*cg.Node, map[int]bool) {
	leafCalls := make(map[uniquePosInfo]bool)
	nodesWorkList := make([]*cg.Node, 0)
	nodesVisited := make(map[int]bool)
	for f, n := range cfg.graph.Nodes {
		if f == nil {
			// not an actual function
			continue
		}
		for libFnName, recvs := range cfg.LibFns {
			sig := f.Signature
			if f.Name() != libFnName {
				// not a leaf function name
				continue
			}

			// currently we support either specifying concrete leaf functions and methods (with renaming)
			// or specifying interface in the library where leaf methods are defined (no renaming)
			// TODO: this does not currently work for methods with receiver type that's a pointer
			recv := sig.Recv()
			if cfg.libIfaces != nil {
				if recv == nil || (recv.Pkg() != nil && cfg.isPkgExternal(recv.Pkg().Path())) {
					// ignore non-methods and methods whose receiver
					// is defined externally (we can't do much about
					// medthod implementations in third-party code)
					continue
				}
				for _, li := range cfg.libIfaces {
					if types.Implements(recv.Type(), li) {
						msg := "WARNING: function " + f.Name() + " implements library interface " + cfg.LibIface + " and, consequently, receives context parameter but may in fact not use context"
						cfg.writeWarning(cfg.getFset(f), f.Pos(), msg)
						cfg.collectFnDef(nodesWorkList, nodesVisited, n, f.Name(), getTypeWithPkgFromVar(recv))
					}
				}
				continue // we are specifying functions via an interface so skip the rest of the loop
			}

			for recv, callReplacement := range recvs {
				pkg := f.Package()
				if pkg == nil || pkg.Pkg.Path() != cfg.LibPkgPath || pkg.Pkg.Name() != cfg.LibPkgName {
					// function definition does not match a given leaf
					// function specified in the config file
					continue
				}
				libFnRecvType := getTypeWithPkgFromVar(sig.Recv())
				if libFnRecvType != recv {
					// function's receiver does not match one
					// (possibly nil) specified for a given leaf
					// function in the config file
					continue

				}
				for _, in := range n.In {
					uniquePos := cfg.getUniquePosSSAFn(in.Site.Parent(), in.Pos())
					doRename := func(pkgPath string, pkgName string, recvType string, fnName string) {
						if pkgPath == cfg.LibPkgPath && pkgName == cfg.LibPkgName && recvType == libFnRecvType && fnName == libFnName && callReplacement.newName != "" {
							cfg.callSitesRenamed[uniquePos] = callReplacement.newName
						}
					}
					calledViaLiteral := renameCall(in.Site.Common(), doRename)
					if !calledViaLiteral {
						// function is not called via a function
						// literal (instead, for example, it's called
						// via a variable)
						continue
					}
					leafCalls[uniquePos] = true
					paramName := cfg.collectFnDef(nodesWorkList, nodesVisited, in.Caller, in.Caller.Func.Name(),
						getTypeWithPkgFromVar(in.Caller.Func.Signature.Recv()))
					if paramName == cfg.CtxParamName {
						// use default context parameter name specified in the config file
						cfg.callSites[uniquePos] = callReplacement
					} else {
						// use context parameter name specified in the caller
						newCallReplacement := replacementInfo{callReplacement.newName,
							callReplacement.argPos,
							callReplacement.ctxImports,
							callReplacement.ctxRegExpr,
							replaceCtxExprWildcard(ctxWildcard, callReplacement.ctxRegExpr, paramName)}
						cfg.callSites[uniquePos] = &newCallReplacement
					}
				}
			}
		}
	}
	if cfg.debugLevel > 0 {
		fmt.Println("LEAF FUNCTION CALLS: " + strconv.Itoa(len(leafCalls)))
	}
	return nodesWorkList, nodesVisited
}

// collect gathers information about call sites and function
// definitions that must be re-written for context propagation.
func (cfg *analyzerConfig) collect(nodesWorkList []*cg.Node, nodesVisited map[int]bool) {
	l := len(nodesWorkList)
	if l <= 0 {
		// no more work items
		return
	}
	// get a node from the work list
	n := nodesWorkList[l-1]
	nodesWorkList = nodesWorkList[:l-1]
	// iterate over this function's call sites
	for _, in := range n.In {
		if !in.Pos().IsValid() {
			// TODO not sure what to do with functions that do not really exist in the source
			cfg.collect(nodesWorkList, nodesVisited)
			return

		}
		if strings.ContainsAny(n.Func.Name(), "$") && n.Func.Parent() != in.Site.Parent() {
			// if a call to anonymous function is not in the same scope as the function definition
			// then the call graph information about this call is likely incorrect - ignore
			continue
		}
		// record each call site; documentation for https://godoc.org/golang.org/x/tools/go/ssa#Call
		// says: "Pos() returns the ast.CallExpr.Lparen, if explicit in the source"

		// determine if the function containing the call site should have context argument injection skipped
		skipContextParam := false
		if !skipContextParam {
			// skip if first parameter is context already
			isParamContext, _, paramName, _, custom := cfg.isFirstParamContext(in.Site.Common().Signature())
			skipContextParam = isParamContext && (custom || paramName == "_" || paramName == "" || paramName == cfg.CtxParamName)
		}

		if !skipContextParam {
			uniquePos := cfg.getUniquePosSSAFn(in.Site.Parent(), in.Pos())
			caller := in.Caller
			if caller.Func.Name() == "init" {
				// syntheised package initializer as per https://godoc.org/golang.org/x/tools/go/ssa#Function
				if cfg.debugLevel > 0 && cfg.callSites[uniquePos] != &cfg.nilCallReplacement {
					if !cfg.isPkgExternal(caller.Func.Pkg.Pkg.Path()) {
						msg := "WARNING: function " + in.Callee.Func.Name() + " is called from synthetic package initializer - receives ARTFICIAL context as an argument"
						cfg.writeWarning(cfg.getFset(caller.Func), in.Pos(), msg)
					}
				}
				cfg.callSites[uniquePos] = &cfg.nilCallReplacement
			} else {

				// if function called via a function parameter, record parameter for update
				cfg.collectFnParam(nodesWorkList, nodesVisited, in)

				// mark call site as visited
				cfg.callSites[uniquePos] = &cfg.commonCallReplacement

				// put each caller on the work list
				if caller.Func.Pkg != nil {
					pkgPath := caller.Func.Pkg.Pkg.Path()
					pkgName := caller.Func.Pkg.Pkg.Name()
					fnName := caller.Func.Name()
					recvType := getTypeWithPkgFromVar(caller.Func.Signature.Recv())
					// check if propagation should stop with the selected function
					if recvs, exists := cfg.PropagationStops[fnName]; exists {
						if pkgPaths, exists := recvs[recvType]; exists {
							if pkgNames, exists := pkgPaths[pkgPath]; exists {
								if _, exists := pkgNames[pkgName]; exists {
									continue
								}
							}
						}
					}
					paramName := cfg.collectFnDef(nodesWorkList, nodesVisited, caller, fnName, recvType)
					if paramName != cfg.CtxParamName {
						newCallReplacement := replacementInfo{cfg.commonCallReplacement.newName,
							cfg.commonCallReplacement.argPos,
							cfg.commonCallReplacement.ctxImports,
							cfg.commonCallReplacement.ctxRegExpr,
							replaceCtxExprWildcard(ctxWildcard, cfg.commonCallReplacement.ctxRegExpr, paramName)}
						cfg.callSites[uniquePos] = &newCallReplacement
					}
				}
			}
		}
	}
	cfg.collect(nodesWorkList, nodesVisited)
}

// collectFnParam collects function parameter declaration (of type
// function) that will itself receive injection of the context
// parameter (as a result of this function-type parameter being used
// to call a freshly made context-sensitive function).
func (cfg *analyzerConfig) collectFnParam(nodesWorkList []*cg.Node, nodesVisited map[int]bool, edge *cg.Edge) {
	callValue := edge.Site.Common().Value
	p, ok := callValue.(*ssa.Parameter)
	if !ok {
		// a function call at the call site is not performed via the
		// enclosing function's parameter
		return
	}

	uniquePos := cfg.getUniquePosSSAFn(p.Parent(), p.Pos())
	_, exists := cfg.fnParamsVisited[uniquePos]

	if exists {
		// we have already processed a call made vi this parameter
		return
	}

	if sig, ok := p.Type().(*types.Signature); ok {
		// if the call happens through a parameter and type of this parameter
		// represents a signature (which in this case it should), mark this parameter
		// for addition of the context parameter unless it's already there
		isParamContext, _, paramName, paramType, custom := cfg.isFirstParamContext(sig)
		skipContextParam := isParamContext && (custom || paramName == "_" || paramName == "" || paramName == cfg.CtxParamName)
		if skipContextParam {
			return
		}
		if cfg.debugLevel > 0 && paramType == cfg.CtxParamType && !cfg.isPkgExternal(edge.Caller.Func.Pkg.Pkg.Path()) {
			msg := "WARNING: argument " + p.Name() + " of type function takes the first parameter that is of type " + cfg.CtxParamType + " defined in different package than " + cfg.CtxPkgPath + "/" + cfg.CtxPkgName
			cfg.writeWarning(cfg.getFset(p.Parent()), p.Pos(), msg)
		}
		cfg.fnParamsVisited[uniquePos] = true

		// find all other functions that can be called through this function argument
		/// and add them to the work list so that context argument may be added
		// to them as well (this may result in functions to receive context argument
		// even though they don't need it, if the call graph is imprecise, which it
		// sometime is)
		for _, o := range edge.Caller.Out {
			oUniquePos := cfg.getUniquePosSSAFn(o.Site.Parent(), o.Pos())
			edgeUniquePos := cfg.getUniquePosSSAFn(edge.Site.Parent(), edge.Pos())
			if oUniquePos == edgeUniquePos {
				fnName := o.Callee.Func.Name()
				recvType := getTypeWithPkgFromVar(o.Callee.Func.Signature.Recv())
				cfg.collectFnDef(nodesWorkList, nodesVisited, o.Callee, fnName, recvType)
			}
		}
	}
}

// collectFnDef, given a call graph node, collects information about a
// function definition that will receive injection of the context
// parameter
func (cfg *analyzerConfig) collectFnDef(nodesWorkList []*cg.Node,
	nodesVisited map[int]bool,
	caller *cg.Node,
	fnName string,
	fnRecv string) string {

	// check if the first parameter is a context parameter already in which case do nothing
	var isParamContext bool
	var renameParamPos token.Pos
	var paramName string
	var paramType string
	if isParamContext, renameParamPos, paramName, paramType, _ = cfg.isFirstParamContext(caller.Func.Signature); isParamContext {
		if paramName == "_" || paramName == "" {
			// will be renamed to ctxParamName
			cfg.renameParamsVisited[cfg.getUniquePosSSAFn(caller.Func, renameParamPos)] = true
			return cfg.CtxParamName
		}
		// context parameter exists - either with the name the same as specified in config
		// or different one (in which case all calls within function must use the new name)
		return paramName
	}
	parent := caller.Func.Parent()
	if parent != nil && cfg.graph.Nodes[parent] != nil {
		// as we are trying to minimize changes, particularly for function signatures (that may be arguments for other functions, implement interfaces, etc.),
		// for nested functions we pass context as a free variable to the closure
		recvType := getTypeWithPkgFromVar(parent.Signature.Recv())
		return cfg.collectFnDef(nodesWorkList, nodesVisited, cfg.graph.Nodes[parent], parent.Name(), recvType)
	}
	// check if a node  has already been processed; if not, add it to visited map
	// and inspect callers of the function it represents (apparently there can be
	// multiple nodes with the same function and different callers/callees sets)
	// documentation for https://godoc.org/golang.org/x/tools/go/ssa#Function says:
	// "Pos() returns the declaring ast.FuncLit.Type.Func or the position
	// of the ast.FuncDecl.Name, if the function was explicit in the source"
	if nodesVisited[caller.ID] {
		return cfg.CtxParamName
	}

	nodesVisited[caller.ID] = true
	uniquePos := cfg.getUniquePosSSAFn(caller.Func, caller.Func.Pos())
	fnType, exists := cfg.fnVisited[uniquePos]
	if (!exists || fnType == extFn) && cfg.debugLevel > 0 && paramType == cfg.CtxParamType && !cfg.isPkgExternal(caller.Func.Pkg.Pkg.Path()) {

		msg := "WARNING: function " + caller.Func.Name() + " takes the first parameter that is of type " + cfg.CtxParamType + " defined in different package than " + cfg.CtxPkgPath + "/" + cfg.CtxPkgName
		cfg.writeWarning(cfg.getFset(caller.Func), caller.Func.Pos(), msg)

	}
	if (exists && fnType != regularFn) || isTestingInitOrMainFunction(caller.Func.Name(), caller.Func.Signature) {
		cfg.markFnAsFreshCtx(uniquePos, cfg.getFset(caller.Func), caller.Func.Name(), caller.Func.Pkg.Pkg.Path(), fnType, exists)
	} else if cfg.isMapOrSliceSig(caller.Func.Pkg, caller.Func.Signature) {
		cfg.markFnAsFreshCtx(uniquePos, cfg.getFset(caller.Func), caller.Func.Name(), caller.Func.Pkg.Pkg.Path(), containerSig, exists)
	} else if cfg.isExtReceiver(caller.Func.Signature) {
		cfg.markFnAsFreshCtx(uniquePos, cfg.getFset(caller.Func), caller.Func.Name(), caller.Func.Pkg.Pkg.Path(), extRecv, exists)
	} else {
		modified := cfg.addIfacesModified(caller.Func.Signature, caller.Func.Name(), fnRecv)
		if modified {
			cfg.fnVisited[uniquePos] = regularFn
			// put new function node in the work list
			nodesWorkList = append(nodesWorkList, caller)
			cfg.collect(nodesWorkList, nodesVisited)
		} else {
			cfg.markFnAsFreshCtx(uniquePos, cfg.getFset(caller.Func), caller.Func.Name(), caller.Func.Pkg.Pkg.Path(), extPkg, exists)
		}
	}
	return cfg.CtxParamName
}

// getUniquePosSSAFn returns unique position of a function described
// by its SSA representation.
func (cfg *analyzerConfig) getUniquePosSSAFn(fn *ssa.Function, pos token.Pos) uniquePosInfo {
	if fn.Pkg == nil {
		return cfg.getUniquePosPkg(nil, pos)
	}
	return cfg.getUniquePosPkg(fn.Pkg.Pkg, pos)
}

// getActualCallArg returns an argument for a function call at a given
// position.
func getActualCallArg(common *ssa.CallCommon, ind int) ssa.Value {
	// if common represents a method, the first arguement is receiver
	// which we don't want to analyze
	if !common.IsInvoke() && common.Signature().Recv() != nil {
		return common.Args[ind+1]
	}
	return common.Args[ind]
}

// markParamAsExternalFn marks a given parameter as one representing
// an external function.
func (cfg *analyzerConfig) markParamAsExternalFn(arg *ssa.Value) {
	var extFun *ssa.Function
	if mc, ok := (*arg).(*ssa.MakeClosure); ok {
		extFun = mc.Fn.(*ssa.Function) // always a function
	} else if mi, ok := (*arg).(*ssa.MakeInterface); ok {
		if extFun, ok = mi.X.(*ssa.Function); !ok {
			return
		}
	} else if extFun, ok = (*arg).(*ssa.Function); !ok {
		return
	}
	// mark function as external so propagation stops here if context needs to be injected
	// and "fake" context variable is injected at the begining of the function
	cfg.fnVisited[cfg.getUniquePosSSAFn(extFun, extFun.Pos())] = extFn

}

// getTypeWithPkgFromVar returns a string representing type of a
// variable qualified with its defining package name and path.
func getTypeWithPkgFromVar(v *types.Var) string {
	if v == nil {
		return ""
	}
	return types.TypeString(v.Type(), computePkgID)
}

// computePkgID returns package identifier consisting of its name and
// path.
func computePkgID(p *types.Package) string {
	return p.Path() + p.Name()
}

// renameCall returns false if function isn't called via literal
// (e.g. via another function's parameter), true otherwise. Returned
// value also indicates if the renaming was attempted or not.
func renameCall(call *ssa.CallCommon, doRename func(pkgPath string, pkgName string, recvType string, fnName string)) bool {
	// check if function needs to be renamed, but only if call site is an actual call expression
	// due to inaccuracy of call graph construction, calls through function-type parameters can
	// appear as direct calls to a function that needs to be renamed (as they may share the same
	// signature that the function that needs to be renamed) but in this case renaming should
	// be skipped; for example
	//
	// func foo(f func() bool) bool {
	//   return	f()
	// }
	//
	// and the library function that we need to rename
	//
	// func libBar() bool {
	// ...
	// }
	//
	// In an inaccurate call graph, there may be an edge from foo (caller) to libBar (callee) as
	// libBar and function-type parameter to foo share the same signature. In this case we
	// don't want to rename
	if call.IsInvoke() {
		pkgPath := call.Method.Pkg().Path()
		pkgName := call.Method.Pkg().Name()
		recvType := getTypeWithPkgFromVar(call.Signature().Recv())
		fnName := call.Method.Name()
		doRename(pkgPath, pkgName, recvType, fnName)
		return true
	}
	if _, ok := call.Value.(*ssa.Parameter); !ok {
		// if a call isn't through a literal (e.g. through a parameter instead)
		// then there is nothing to rename
		var f *ssa.Function
		if mc, ok := call.Value.(*ssa.MakeClosure); ok {
			f = mc.Fn.(*ssa.Function) // always a function
		} else if f, ok = call.Value.(*ssa.Function); !ok {
			return false
		}
		pkgPath := f.Pkg.Pkg.Path()
		pkgName := f.Pkg.Pkg.Name()
		recvType := getTypeWithPkgFromVar(f.Signature.Recv())
		fnName := f.Name()
		doRename(pkgPath, pkgName, recvType, fnName)
		return true
	}
	return false
}

// replaceCtxExprWildcard computes context expression from the config
// file by replacing the (optional) wildcard with the context literal.
func replaceCtxExprWildcard(wildcard string, ctxRegExpr string, ctxLit string) string {
	if ctxRegExpr == "" {
		return ctxLit
	}
	if strings.Contains(ctxRegExpr, wildcard) {
		return strings.ReplaceAll(ctxRegExpr, wildcard, ctxLit)
	}
	return ctxRegExpr
}

// getFset returns FileSet for a given function.
func (cfg *analyzerConfig) getFset(fn *ssa.Function) *token.FileSet {
	if cfg.largeCode {
		return cfg.fsets[fn.Pkg.Pkg]
	}
	return fn.Prog.Fset
}

// isFirstParamContext checks if the firs parameter is of specified context type and returns result as the first value.
// The other return values represent, respectively:
// - position of the context parameter (if any - otherwise invalid position)
// - name in the function definition (to be used for callers needing context parameter)
// - type in the function definition.
func (cfg *analyzerConfig) isFirstParamContext(sig *types.Signature) (bool, token.Pos, string, string, bool) {
	params := sig.Params()
	if params == nil {
		return false, token.NoPos, cfg.CtxParamName, "", false
	}

	v := params.At(0)
	typeName := v.Type().String()
	if named, ok := v.Type().(*types.Named); ok {
		typeName = named.Obj().Name()
	}

	t := getTypeWithPkgFromVar(v)
	if t == cfg.ctxParamTypeWithPkgPathName {
		// context parameter of appropriate type already exists
		return true, v.Pos(), v.Name(), typeName, false
	}
	if t == cfg.ctxCustomParamTypeWithPkgPathName {
		// context parameter represents custom context - its name
		// cannot be used directly; instead "regular" context must
		// be extracted from it using custom extraction
		// expression; this expression contains a wild card that
		// is a placeholder for the custom context name that must
		// be filled with the right value
		return true, v.Pos(), replaceCtxExprWildcard(ctxCustomWildcard, cfg.CtxCustomExprExtract, v.Name()), typeName, true
	}
	return false, token.NoPos, cfg.CtxParamName, typeName, false
}

// mapSigToPkg adds a function signature to a pkg->funcSig map.
func mapSigToPkg(sigMap map[*ssa.Package]map[*types.Signature]bool, pkg *ssa.Package, sig *types.Signature) {
	var exists bool
	var signatures map[*types.Signature]bool
	if signatures, exists = sigMap[pkg]; !exists {
		signatures = make(map[*types.Signature]bool)
		sigMap[pkg] = signatures
	}

	signatures[sig] = true
}

// isTestingInitOrMainFunction determines, based on a function
// signature, if a given function is a testing function or a main
// function.
func isTestingInitOrMainFunction(n string, sig *types.Signature) bool {
	if (n == "main" || isInitFuncName(n)) && sig.Results() == nil && sig.Params() == nil {
		return true
	}
	if len(n) < 5 {
		// has to be at least TestX
		return false
	}
	if n[:4] != "Test" {
		return false
	}
	if n[4:5] != "_" && strings.ToLower(n[4:5]) == n[4:5] {
		// X in TestX must be "_" or in upper case
		return false
	}
	params := sig.Params()
	if params == nil {
		return false
	}
	if params.Len() != 1 {
		return false
	}
	firstParamType := params.At(0).Type().String()
	if firstParamType == testingTypeT || (n == "TestMain" && firstParamType == testingTypeM) {
		return true
	}
	return false
}

// isInitFuncName deermines if a given function name represents an
// initialization function.
func isInitFuncName(n string) bool {
	s := strings.TrimPrefix(n, "init#")
	if s == n {
		// does not start with the right prefix
		return false
	}
	if i, err := strconv.ParseInt(s, 8, 32); err == nil && i > 0 {
		return true
	}
	return false
}

// markFnAsFreshCtx marks a given function as the one that will
// receive injection of artificial context variable at the beginnin of
// its body.
func (cfg *analyzerConfig) markFnAsFreshCtx(pos uniquePosInfo, fset *token.FileSet, name string, pkgPath string, fnType int, exists bool) {
	if cfg.debugLevel > 0 && (!exists || fnType == extFn) {
		if cfg.isPkgExternal(pkgPath) {
			// modifications of code in external packages is
			// suppressed and warning generation must be suppressed
			// as well
			return
		}

		msg := "WARNING: function " + name + " is a function used by the test harness (injecting ARTIFICIAL context)"
		if fnType == containerSig {
			msg = "WARNING: signature of function " + name + " is used as used as a type in construction of map or array/slice  (injecting ARTIFICIAL context)"
		} else if fnType == extFn {
			msg = "WARNING: function " + name + " is used as parameter by another function from an external package (injecting ARTIFICIAL context)"
		} else if fnType == extPkg {
			msg = "WARNING: function " + name + " implements interface from an external package (injecting ARTIFICIAL context)"
		} else if fnType == extRecv {
			msg = "WARNING: function " + name + " receiver type embeds another external type (injecting ARTIFICIAL context)"
		}
		cfg.writeWarning(fset, pos.pos, msg)

	}
	cfg.fnVisited[pos] = freshCtxFn
}

// isMapOrSliceSig determines if a signature of a given function is
// used in map or slice definition in the same package.
func (cfg *analyzerConfig) isMapOrSliceSig(pkg *ssa.Package, sig *types.Signature) bool {
	sigs, exists := cfg.mapAndSliceFuncs[pkg]
	if !exists {
		return false
	}
	for mapSig, _ := range sigs {
		if types.Identical(sig, mapSig) {
			return true
		}
	}
	return false
}

// isExtReceiver determines if a given method's receiver is of type
// that contains one of the embedded external types specified in the
// config file.
func (cfg *analyzerConfig) isExtReceiver(sig *types.Signature) bool {
	recv := sig.Recv()
	if recv == nil {
		return false
	}
	var t types.Type
	t = recv.Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}

	if s, ok := t.Underlying().(*types.Struct); ok && cfg.extRecvTypes[s] {
		return true
	}
	return false
}

// addIfacesModified records an interface function declaration that
// needs to be modified as a result of a concrete method
// implementation (implementing this interface) being modified.
func (cfg *analyzerConfig) addIfacesModified(sig *types.Signature,
	fnName string,
	fnRecv string) bool {
	if fnRecv == "" {
		// no interface to modify, but function's signature must change
		return true
	}

	var ifacesToModify []*types.Interface
	var methodsToModify []*types.Func
	modifiedNum := 0
	for iface, _ := range cfg.ifaces {
		if !types.Implements(sig.Recv().Type(), iface) {
			continue
		}
		// method may implement embedded interface
		m, actualIface := getMethodAndInterface(fnName, iface)
		if m == nil || actualIface == nil {
			// interface not found - keep looking
			continue
		}
		if _, exists := cfg.ifaces[actualIface]; !exists {
			// external interface - do not modify any interface nor method's signature
			return false
		}
		ifacesToModify = append(ifacesToModify, actualIface)
		methodsToModify = append(methodsToModify, m)
		modifiedNum = modifiedNum + 1
	}
	for i := 0; i < modifiedNum; i++ {
		modifiedIface := ifacesToModify[i]
		modifiedMethod := methodsToModify[i]

		// all interface methods must be regular functions
		// as they have no body and there is no way to inject
		// a context variable into the body
		cfg.fnVisited[cfg.getUniquePosTypesFn(modifiedMethod, modifiedMethod.Pos())] = regularFn

		var exists bool
		var methods map[string]bool
		if methods, exists = cfg.ifaceModified[modifiedIface]; !exists {
			methods = make(map[string]bool)
			cfg.ifaceModified[modifiedIface] = methods
		}
		methods[fnName] = true
	}
	return true
}

// getUniquePosTypesFn returns unique position of a function described
// by its type.
func (cfg *analyzerConfig) getUniquePosTypesFn(fn *types.Func, pos token.Pos) uniquePosInfo {
	return cfg.getUniquePosPkg(fn.Pkg(), pos)
}

// getMethodAndInterface, based on method name its interface, returns
// the actual function and the actual (possibly embedded) interface it
// implements.
func getMethodAndInterface(methodName string, iface *types.Interface) (*types.Func, *types.Interface) {
	for i := 0; i < iface.NumExplicitMethods(); i++ {
		m := iface.ExplicitMethod(i)
		if m.Name() == methodName {
			return m, iface
		}
	}
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		t := iface.EmbeddedType(i)
		embed, ok := t.Underlying().(*types.Interface)
		if !ok {
			// not a interface
			continue
		}
		m, actualIface := getMethodAndInterface(methodName, embed)
		if m != nil && actualIface != nil {
			return m, actualIface
		}
	}
	return nil, nil
}

// getFuncFromArg returns function definition representing a given
// value (or nil if the value is not of function type).
func getFuncFromArg(arg ssa.Value) *ssa.Function {
	if ct, ok := arg.(*ssa.ChangeType); ok {
		if mc, ok := ct.X.(*ssa.MakeClosure); ok {
			return mc.Fn.(*ssa.Function) // always a function
		}
		if fn, ok := ct.X.(*ssa.Function); ok {
			return fn
		}
	} else if c, ok := arg.(*ssa.Call); ok {
		res := c.Common().Signature().Results()
		if res.Len() != 1 {
			log.Fatalf("function call argument has more than one return value (expected one of function type)")
		}
		// TODO: ignore for now, possibly deal with later if need be
	} else if _, ok := arg.(*ssa.Parameter); ok {
		// TODO: ignore for now, possibly deal with later if need be
	} else if _, ok := arg.(*ssa.UnOp); ok {
		// TODO: ignore for now, possibly deal with later if need be
	} else if _, ok := arg.(*ssa.Const); ok {
		// TODO: ignore for now, possibly deal with later if need be
	} else if _, ok := arg.(*ssa.Phi); ok {
		// TODO: ignore for now, possibly deal with later if need be
	} else if _, ok := arg.(*ssa.Extract); ok {
		// TODO: ignore for now, possibly deal with later if need be
	} else {
		log.Fatalf("unrecognized argument for parameter of type function")
	}
	return nil
}

// collectIfaces gathers information about additional interfaces that
// need to be modified as a result of prior modifications to function
// definitions.
func (cfg *analyzerConfig) collectIfaces(namedModified map[*types.Named]bool) bool {
	// we iterate over "original" ifaceModified structure so we don't
	// want to modify it in the middle of iteration
	ifaceModifiedNew := make(map[*types.Interface]map[string]bool)
	for f, n := range cfg.graph.Nodes {
		if f == nil {
			continue
		}
		// check if a given function implements one of the modified
		// interfaces without being modified itself already limit the
		// check to the same package, otherwise the unnecessarily
		// (though correctly) rewritten method signatures and injected
		// artificial contexts grow too large
		sig := f.Signature
		if sig.Recv() != nil {
			for iface, funcNames := range cfg.ifaceModified {
				if types.Implements(sig.Recv().Type(), iface) {
					if _, exists := funcNames[f.Name()]; exists {
						cfg.insertArtificialCtx(namedModified, f)
					}
				}
			}
		}
		// for each parameter of type interface, find actual receiver
		// type for the methods and alter method signatures for this
		// receiver type
		params := sig.Params()
		for ind := 0; ind < params.Len(); ind++ {
			p := params.At(ind)
			iface, ok := p.Type().Underlying().(*types.Interface)
			if !ok {
				// parameter if not of interface type
				continue
			}
			funcNames, exists := cfg.ifaceModified[iface]
			if !exists {
				// interface type has not been modified
				continue
			}
			// get the type of an argument at the call site for the selected function
			for _, caller := range n.In {
				arg := getActualCallArg(caller.Site.Common(), ind)

				// argType = arg.Type() does not work here (misses some cases)
				var argType types.Type
				if mi, ok := arg.(*ssa.MakeInterface); ok {
					argType = mi.X.Type()
				} else if c, ok := arg.(*ssa.Call); ok {
					res := c.Common().Signature().Results()
					if res.Len() != 1 {
						log.Fatalf("function call argument has more than one return value (expected one of interface type)")
					}
					argType = res.At(0).Type()
				} else if p, ok := arg.(*ssa.Parameter); ok {
					argType = p.Object().Type()
				} else if u, ok := arg.(*ssa.UnOp); ok {
					argType = u.Type()
				} else if e, ok := arg.(*ssa.Extract); ok {
					argType = e.Type()
				} else if p, ok := arg.(*ssa.Phi); ok {
					argType = p.Type()
				} else if c, ok := arg.(*ssa.Const); ok {
					argType = c.Type()
				} else if ct, ok := arg.(*ssa.ChangeType); ok {
					argType = ct.Type()
				} else if ci, ok := arg.(*ssa.ChangeInterface); ok {
					argType = ci.Type()
				} else {
					log.Fatalf("unrecognized argument for parameter of type interface")
				}
				methodSet := cfg.prog.MethodSets.MethodSet(argType)
				// get all methods defined for the given argument type
				for j := 0; j < methodSet.Len(); j++ {
					sel := methodSet.At(j)
					if funcNames[methodSet.At(j).Obj().Name()] {
						// mark method definitions that have not yet
						// been marked for addition of the context
						// argument unless "sel" represents abstract
						// (interface method)
						if cfg.prog.MethodValue(sel) != nil {
							fun := cfg.prog.MethodValue(sel)
							cfg.insertArtificialCtx(namedModified, fun)
						}
					}
				}
			}
		}
	}

	// merge info about modified interfaces
	added := false
	for ifaceNew, funcNamesNew := range ifaceModifiedNew {
		var funcs map[string]bool
		var exists bool
		if funcs, exists = cfg.ifaceModified[ifaceNew]; !exists {
			added = true
			cfg.ifaceModified[ifaceNew] = funcNamesNew
		} else {
			for name, _ := range funcNamesNew {
				if !funcs[name] {
					added = true
					funcs[name] = true
				}
			}
		}
	}
	return added
}

// collectNamedTypes gathers information about named types that need
// to be modified as a result of prior modifications to function
// definitions.
func (cfg *analyzerConfig) collectNamedTypes(namedModified map[*types.Named]bool) {
	for {
		// discover named types to be modified with injected context parameter
		namedModifiedNew := make(map[*types.Named]bool)
		for f, n := range cfg.graph.Nodes {
			if f == nil {
				continue
			}
			params := f.Signature.Params()
			for ind := 0; ind < params.Len(); ind++ {
				p := params.At(ind)
				namedUnmodifed, sig := cfg.getUnmodifiedNamedFunctionType(p.Type(), namedModified)
				if namedUnmodifed == nil {
					continue
				}
				isParamContext, _, _, _, _ := cfg.isFirstParamContext(sig)
				// do nothing as the named type already has context
				// parameter (this will catch named interfaces added
				// in previous iteration)
				if isParamContext {
					continue
				}
				for _, caller := range n.In {
					arg := getActualCallArg(caller.Site.Common(), ind)
					argFun := getFuncFromArg(arg)
					if argFun != nil {
						uniqueFnPos := cfg.getUniquePosSSAFn(argFun, argFun.Pos())
						if fnType, exists := cfg.fnVisited[uniqueFnPos]; exists && fnType != extFn {
							uniqueNamedPos := cfg.getUniquePosPkg(namedUnmodifed.Obj().Pkg(), namedUnmodifed.Obj().Pos())
							cfg.fnVisited[uniqueNamedPos] = regularFn
							namedModifiedNew[namedUnmodifed] = true
						}
					}
				}
			}
		}

		if len(namedModifiedNew) == 0 {
			break
		}

		// discover functions that have to be modified to take an additional context parameter
		// as a result of named types change
		for f, n := range cfg.graph.Nodes {
			if f == nil {
				continue
			}
			params := f.Signature.Params()
			for ind := 0; ind < params.Len(); ind++ {
				p := params.At(ind)
				named, ok := p.Type().(*types.Named)
				if !ok {
					// not a named type
					continue
				}
				// only analyze types added in the previous step
				if _, exists := namedModifiedNew[named]; !exists {
					continue
				}
				for _, caller := range n.In {
					arg := getActualCallArg(caller.Site.Common(), ind)
					fun := getFuncFromArg(arg)
					if fun != nil {
						cfg.insertArtificialCtx(namedModified, fun)
					}
				}
			}
		}

		for named, _ := range namedModifiedNew {
			namedModified[named] = true
		}

	}
}

// insertArtificialCtx injects artificial context variable at the
// beginning of the function body (unless it already has a context
// parameter) and injects artificial context argument to all its call
// sites.
func (cfg *analyzerConfig) insertArtificialCtx(namedModified map[*types.Named]bool, fun *ssa.Function) {
	sig := fun.Signature
	var isParamContext bool
	var renameParamPos token.Pos
	var paramName string
	var paramType string
	var custom bool
	if isParamContext, renameParamPos, paramName, paramType, custom = cfg.isFirstParamContext(sig); isParamContext && (paramName == "_" || paramName == "") {
		// will be renamed to ctxParamName
		cfg.renameParamsVisited[cfg.getUniquePosSSAFn(fun, renameParamPos)] = true
	} else {
		skipContextParam := isParamContext && (custom || paramName == "_" || paramName == "" || paramName == cfg.CtxParamName)
		if skipContextParam {
			return
		}
		if cfg.debugLevel > 0 && paramType == cfg.CtxParamType && !cfg.isPkgExternal(fun.Pkg.Pkg.Path()) {
			msg := "WARNING: function " + fun.Name() + " takes the first parameter that is of type " + cfg.CtxParamType + " defined in different package than " + cfg.CtxPkgPath + "/" + cfg.CtxPkgName
			cfg.writeWarning(cfg.getFset(fun), fun.Pos(), msg)
		}
		uniquePos := cfg.getUniquePosSSAFn(fun, fun.Pos())
		fnType, exists := cfg.fnVisited[uniquePos]
		if (exists && fnType != regularFn) || isTestingInitOrMainFunction(fun.Name(), sig) {
			cfg.markFnAsFreshCtx(uniquePos, cfg.getFset(fun), fun.Name(), fun.Pkg.Pkg.Path(), fnType, exists)
		} else if cfg.isMapOrSliceSig(fun.Pkg, fun.Signature) {
			cfg.markFnAsFreshCtx(uniquePos, cfg.getFset(fun), fun.Name(), fun.Pkg.Pkg.Path(), containerSig, exists)
		} else if cfg.isExtReceiver(fun.Signature) {
			cfg.markFnAsFreshCtx(uniquePos, cfg.getFset(fun), fun.Name(), fun.Pkg.Pkg.Path(), extRecv, exists)
		} else {
			// add all interfaces that this method's receiver implements to the set
			// of these that still need to be processed (unless they are external interfaces)
			modified := cfg.addIfacesModified(sig,
				fun.Name(),
				getTypeWithPkgFromVar(sig.Recv()))
			if modified {
				cfg.fnVisited[uniquePos] = regularFn
				funNode := cfg.graph.Nodes[fun]
				if funNode != nil {
					cfg.insertArtificialCtxCallsites(namedModified, funNode)
				}
			} else {
				cfg.markFnAsFreshCtx(uniquePos, cfg.getFset(fun), fun.Name(), fun.Pkg.Pkg.Path(), extPkg, exists)
			}
		}
	}
}

// insertArtificialCtxCallsites injects artificial context argument to
// all call sites of a given function.
func (cfg *analyzerConfig) insertArtificialCtxCallsites(namedModified map[*types.Named]bool, funNode *cg.Node) {
	// find all call sites of a function we just modified and if no context argument
	// is already passed, pass nil as the first argument
	for _, in := range funNode.In {
		uniquePos := cfg.getUniquePosSSAFn(in.Site.Parent(), in.Pos())
		_, exists := cfg.callSites[uniquePos]
		if exists {
			// we have already processed this call site
			continue
		}
		skipContextParam := false
		if !skipContextParam {
			// skip if first parameter is context already
			isParamContext, _, _, _, _ := cfg.isFirstParamContext(in.Site.Common().Signature())
			skipContextParam = isParamContext
		}

		if !skipContextParam {
			// see if caller of this function has a context parameter
			isParamContext, renameParamPos, paramName, _, _ := cfg.isFirstParamContext(in.Caller.Func.Signature)
			if isParamContext {
				if paramName == "_" || paramName == "" {
					// param name is "_" or ther isn't a name - change it to default context parameter name
					cfg.callSites[uniquePos] = &cfg.commonCallReplacement
					// also change the name of the parameter of the to ctxParamName
					cfg.renameParamsVisited[cfg.getUniquePosSSAFn(in.Caller.Func, renameParamPos)] = true
				} else if paramName != cfg.CtxParamName {
					newCallReplacement := replacementInfo{cfg.commonCallReplacement.newName,
						cfg.commonCallReplacement.argPos,
						cfg.commonCallReplacement.ctxImports,
						cfg.commonCallReplacement.ctxRegExpr,
						replaceCtxExprWildcard(ctxWildcard, cfg.commonCallReplacement.ctxRegExpr, paramName)}
					cfg.callSites[uniquePos] = &newCallReplacement
				} else {
					cfg.callSites[uniquePos] = &cfg.commonCallReplacement
				}
			} else {
				// there is no context param - inject an artificial one
				// unless the call is through unmodifed named type
				namedUnmodifed, _ := cfg.getUnmodifiedNamedFunctionType(in.Site.Common().Value.Type(), namedModified)
				if namedUnmodifed == nil {
					cfg.callSites[uniquePos] = &cfg.nilCallReplacement
				}
			}
		}
	}
}

// getUnmodifiedNamedFunctionType returns unmodified function type or
// nil (if already modified or not a named function type).
func (cfg *analyzerConfig) getUnmodifiedNamedFunctionType(t types.Type, namedModified map[*types.Named]bool) (*types.Named, *types.Signature) {
	named, ok := t.(*types.Named)
	if !ok {
		// not a named type
		return nil, nil
	}
	sig, ok := named.Underlying().(*types.Signature)
	if !ok {
		// named type but not representing a signature
		return nil, nil
	}
	_, exists := namedModified[named]
	if exists {
		// modified named type representing a signature
		return nil, nil
	}
	return named, sig
}
