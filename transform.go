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
	"go/ast"
	"go/token"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
	"log"
	"strconv"
	"strings"
)

// transform is the main driver function of the transformation phase.
func (cfg *transformerConfig) transform() map[*packages.Package]map[*ast.File]int {

	results := make(map[*packages.Package]map[*ast.File]int)
	visitedFiles := make(map[string]bool)
	importsAdded := 0

	for _, p := range cfg.initial {
		// iterate over all packages
		if cfg.isPkgExternal(p.PkgPath) {
			// don't modify (external) package where the interface
			// containing leaf methods is defined
			continue
		}
		cfg.currentPkg = p
		for ind, f := range p.Syntax {
			// iterate over all files in a given package

			// if the list paths passed to packages.LoadAll contains duplicates
			// we could process some files twice which would generated incorrect code
			if visitedFiles[p.CompiledGoFiles[ind]] {
				continue
			}
			visitedFiles[p.CompiledGoFiles[ind]] = true

			cfg.computeExistingImports(f)
			// init context-related expressions that depend on the
			// current file's import statements
			cfg.initContextExpressions()
			// perform AST transformation
			cfg.newImports = make(map[string]string)

			// cfg.modified will be set to true during AST traversal
			// if the code actually changes
			cfg.modified = false
			res := astutil.Apply(f, nil, cfg.astRewrite)

			if res != f {
				log.Fatalf("root note of rewritten AST unexpectedly changed")
			}
			if cfg.modified {
				addResult(results, p, f, ind)
				if cfg.addImports(f) {
					importsAdded++
				}
			}
		}
	}
	if cfg.debugLevel > 0 {
		fmt.Println("IFACES MODIFIED: " + strconv.Itoa(len(cfg.astIfaceModified)) + " METHODS: " + strconv.Itoa(cfg.ifaceMethodModifiedNum))
		fmt.Println("NAMED MODIFIED: " + strconv.Itoa(cfg.astNamedModifiedNum))
		fmt.Println("PARAMS MODIFIED: " + strconv.Itoa(cfg.astParamsModifiedNum))
		fmt.Println("CALLS MODIFIED: " + strconv.Itoa(cfg.astCallsModifiedNum))
		fmt.Println("SIGNATURES MODIFIED: " + strconv.Itoa(cfg.astSigsModifiedNum))
		fmt.Println("DEFINITIONS MODIFIED: " + strconv.Itoa(cfg.astDefsModifiedNum))
		fmt.Println("IMPORTS ADDED: " + strconv.Itoa(importsAdded))
	}

	return results
}

// computeExistingImports computes information about imports already
// existing in the analyzed AST.
func (cfg *transformerConfig) computeExistingImports(f *ast.File) {
	cfg.existingImports = make(map[string]string)
	for _, para := range astutil.Imports(cfg.currentPkg.Fset, f) {
		for _, imp := range para {
			importPath := imp.Path.Value
			len := len(importPath)
			// in well-formed Go file, import path is enclosed in quotes
			importPath = importPath[1 : len-1]
			if imp.Name == nil {
				cfg.existingImports[importPath] = ""
			} else {
				cfg.existingImports[importPath] = imp.Name.Name
			}
		}
	}
}

// initContextExpressions initializes expressions to be injected into
// the code whose final shape depends on the imports already defined
// in the analyzed AST.
func (cfg *transformerConfig) initContextExpressions() {
	pkgAlias, importFound := cfg.existingImports[cfg.CtxPkgPath]
	if importFound {
		if pkgAlias != "" {
			cfg.CtxParamInvalid = pkgAlias + "." + cfg.CtxParamInvalid
			cfg.ctxParamTypeWithPkgAlias = pkgAlias + "." + cfg.CtxParamType
		} else {
			cfg.CtxParamInvalid = cfg.CtxPkgName + "." + cfg.CtxParamInvalid
			cfg.ctxParamTypeWithPkgAlias = cfg.CtxPkgName + "." + cfg.CtxParamType
		}
	} else {
		if cfg.CtxPkgAlias == "" {
			cfg.CtxParamInvalid = cfg.CtxPkgName + "." + cfg.CtxParamInvalid
			cfg.ctxParamTypeWithPkgAlias = cfg.CtxPkgName + "." + cfg.CtxParamType
		} else {
			cfg.CtxParamInvalid = cfg.CtxPkgAlias + "." + cfg.CtxParamInvalid
			cfg.ctxParamTypeWithPkgAlias = cfg.CtxPkgAlias + "." + cfg.CtxParamType
		}
	}
	cfg.nilCallReplacement = replacementInfo{"", 1, nil, "", cfg.CtxParamInvalid}
}

// astRewrite implements the main AST rewriting logic.
func (cfg *transformerConfig) astRewrite(c *astutil.Cursor) bool {
	if e, ok := c.Node().(*ast.CallExpr); ok {
		pos := cfg.renameCallSite(c, e)
		cfg.rewriteCallSite(c, e, pos)

	} else if fd, ok := c.Parent().(*ast.FuncDecl); ok && c.Name() == "Type" {
		uniquePos := cfg.getUniquePosPkg(cfg.currentPkg.Types, fd.Name.NamePos)
		if fnType, exists := cfg.fnVisited[uniquePos]; exists && fnType == regularFn {
			// modify "regular" (named) function definition to inject context parameter
			ft := c.Node().(*ast.FuncType)
			cfg.addContextParam(ft.Params)
			cfg.modified = true
			cfg.astSigsModifiedNum++
		}
	} else if fl, ok := c.Parent().(*ast.FuncLit); ok && c.Name() == "Type" {
		uniquePos := cfg.getUniquePosPkg(cfg.currentPkg.Types, fl.Type.Func)
		if fnType, exists := cfg.fnVisited[uniquePos]; exists && fnType == regularFn {
			// modify function literal (e.g. anonymous function definition) to inject context parameter
			ft := c.Node().(*ast.FuncType)
			cfg.addContextParam(ft.Params)
			cfg.modified = true
			cfg.astSigsModifiedNum++
		}
	} else if fl, ok := c.Node().(*ast.FieldList); ok && c.Name() == "Params" {
		// modify function type definition representing some other function's parameter to inject context parameter
		if fl.List != nil {
			for _, fld := range fl.List {
				uniquePos := cfg.getUniquePosPkg(cfg.currentPkg.Types, fld.Pos())
				if cfg.fnParamsVisited[uniquePos] {
					astutil.Apply(fld.Type, cfg.addContextParamApply, nil)
					cfg.modified = true
					cfg.astParamsModifiedNum++
				}
			}
		}
	} else if iface, ok := c.Parent().(*ast.InterfaceType); ok && c.Name() == "Methods" {
		// modify function type definition in an interface
		fl := c.Node().(*ast.FieldList)
		if fl.List != nil {
			for _, fld := range fl.List {
				uniquePos := cfg.getUniquePosPkg(cfg.currentPkg.Types, fld.Pos())
				if fnType, exists := cfg.fnVisited[uniquePos]; exists && fnType == regularFn {
					cfg.astIfaceModified[iface] = true
					astutil.Apply(fld.Type, cfg.addContextParamApply, nil)
					cfg.modified = true
					cfg.ifaceMethodModifiedNum++
				}
			}
		}
	} else if fd, ok := c.Parent().(*ast.FuncDecl); ok && c.Name() == "Body" {
		uniquePos := cfg.getUniquePosPkg(cfg.currentPkg.Types, fd.Name.NamePos)
		if fnType, exists := cfg.fnVisited[uniquePos]; exists && fnType == freshCtxFn {
			// modify "regular" (named) function definition to inject context variable declaration
			if fd.Body == nil {
				log.Fatalf("adding artificial context to function declaration with no body")
			}
			fd.Body.List = cfg.addContextInitStmt(fd.Body.List, fd.Name.NamePos)
			cfg.modified = true
			cfg.astDefsModifiedNum++
		}
	} else if fl, ok := c.Parent().(*ast.FuncLit); ok && c.Name() == "Body" {
		uniquePos := cfg.getUniquePosPkg(cfg.currentPkg.Types, fl.Type.Func)
		if fnType, exists := cfg.fnVisited[uniquePos]; exists && fnType == freshCtxFn {
			// modify function literal (e.g. anonymous function definition) to inject context variable declaration
			if fl.Body == nil {
				log.Fatalf("adding artificial context to function literal with no body")
			}
			fl.Body.List = cfg.addContextInitStmt(fl.Body.List, fl.Type.Func)
			cfg.modified = true
			cfg.astDefsModifiedNum++
		}
	} else if ft, ok := c.Parent().(*ast.TypeSpec); ok && c.Name() == "Type" {
		uniquePos := cfg.getUniquePosPkg(cfg.currentPkg.Types, ft.Name.NamePos)
		if fnType, exists := cfg.fnVisited[uniquePos]; exists && fnType == regularFn {
			// modify named type to inject context parameter
			astutil.Apply(c.Node(), cfg.addContextParamApply, nil)
			cfg.modified = true
			cfg.astNamedModifiedNum++
		}
	} else if fld, ok := c.Node().(*ast.Field); ok && fld.Names == nil {
		uniquePos := cfg.getUniquePosPkg(cfg.currentPkg.Types, fld.Pos())
		if cfg.renameParamsVisited[uniquePos] {
			fld.Names = []*ast.Ident{ast.NewIdent(cfg.CtxParamName)}
		}
	} else if fld, ok := c.Parent().(*ast.Field); ok && c.Name() == "Names" && c.Index() == 0 {
		uniquePos := cfg.getUniquePosPkg(cfg.currentPkg.Types, fld.Pos())
		if cfg.renameParamsVisited[uniquePos] {
			c.Replace(ast.NewIdent(cfg.CtxParamName))
		}
	}
	return true
}

// addResult records a file modified during AST traversal.
func addResult(results map[*packages.Package]map[*ast.File]int, pkg *packages.Package, f *ast.File, ind int) {
	var exists bool
	var nodes map[*ast.File]int
	if nodes, exists = results[pkg]; !exists {
		nodes = make(map[*ast.File]int)
		results[pkg] = nodes
	}
	nodes[f] = ind
}

// addImports adds imports needed by the code added during AST
// modification.
func (cfg *transformerConfig) addImports(f *ast.File) bool {
	// add library package path if missing - check if path is imported
	// to cover both named and unnamed import
	added := false
	_, importFound := cfg.existingImports[cfg.CtxPkgPath]
	if !importFound {
		if cfg.CtxPkgAlias == "" {
			added = astutil.AddImport(cfg.currentPkg.Fset, f, cfg.CtxPkgPath) || added
		} else {
			added = astutil.AddNamedImport(cfg.currentPkg.Fset, f, cfg.CtxPkgAlias, cfg.CtxPkgPath) || added
		}
	}
	for imp, alias := range cfg.newImports {
		if alias == "" {
			added = astutil.AddImport(cfg.currentPkg.Fset, f, imp) || added
		} else {
			added = astutil.AddNamedImport(cfg.currentPkg.Fset, f, alias, imp) || added
		}
	}
	return added
}

// renameCallSite renames function/method at a given call site and
// returns position of the call site expression.
func (cfg *transformerConfig) renameCallSite(c *astutil.Cursor, e *ast.CallExpr) token.Pos {
	pos := e.Lparen
	if g, ok := c.Parent().(*ast.GoStmt); ok {
		pos = g.Go
	} else if d, ok := c.Parent().(*ast.DeferStmt); ok {
		pos = d.Defer
	}
	// rename functions at call sites
	uniquePos := cfg.getUniquePosPkg(cfg.currentPkg.Types, pos)
	if newName, exists := cfg.callSitesRenamed[uniquePos]; exists {
		newIdent := ast.NewIdent(newName)
		if n, ok := e.Fun.(*ast.SelectorExpr); ok {
			// must replace the whole selector expression
			e.Fun = &ast.SelectorExpr{X: n.X, Sel: newIdent}
		} else if _, ok := e.Fun.(*ast.Ident); ok {
			e.Fun = newIdent
		} else {
			log.Fatalf("unrecognized call expression when rewriting AST")
		}
		cfg.modified = true
	}
	return pos

}

// rewriteCallSite adds context arguments to a given call site.
func (cfg *transformerConfig) rewriteCallSite(c *astutil.Cursor, e *ast.CallExpr, pos token.Pos) {
	uniquePos := cfg.getUniquePosPkg(cfg.currentPkg.Types, pos)
	if _, ok := c.Parent().(*ast.CallExpr); !ok && e.Args == nil {
		// handle no-argument case for a call site only if this
		// call site is not an argument to another call;
		// this case is handled below due to some problem with AST
		// traversal when we need to replace CallExpr stored in an array
		// (of another function's arguments)
		if callReplacement, exists := cfg.callSites[uniquePos]; exists {
			if callReplacement.argPos != 1 {
				cfg.writeWarning(cfg.currentPkg.Fset, pos, "WARNING: requesting to put a context argument in a position other then the first one for parameter-less function - defaulting to first position")
			}
			ctxExpr := cfg.getCtxExprAndAddImports(cfg.existingImports, cfg.newImports, callReplacement)
			args := []ast.Expr{ast.Expr(ast.NewIdent(cfg.resolveCtxExprPackageWildcard(ctxExpr)))}
			ce := ast.CallExpr{Fun: e.Fun, Lparen: e.Lparen, Args: args, Ellipsis: e.Ellipsis, Rparen: e.Rparen}
			c.Replace(&ce)
			cfg.modified = true
			cfg.astCallsModifiedNum++

		}
	} else if e.Args != nil {
		for _, a := range e.Args {
			c, ok := a.(*ast.CallExpr)
			if !ok {
				continue
			}
			if c.Args != nil {
				continue
			}
			uniqueCallPos := cfg.getUniquePosPkg(cfg.currentPkg.Types, c.Lparen)
			callReplacement, exists := cfg.callSites[uniqueCallPos]
			if !exists {
				continue
			}
			if callReplacement.argPos != 1 {
				cfg.writeWarning(cfg.currentPkg.Fset, pos, "WARNING: requesting to put a context argument in a position other then the first one for parameter-less function - defaulting to first position")
			}
			ctxExpr := cfg.getCtxExprAndAddImports(cfg.existingImports, cfg.newImports, callReplacement)
			args := []ast.Expr{ast.Expr(ast.NewIdent(cfg.resolveCtxExprPackageWildcard(ctxExpr)))}
			c.Args = args
			cfg.modified = true
			cfg.astCallsModifiedNum++
		}
		if callReplacement, exists := cfg.callSites[uniquePos]; exists {
			var argPos int
			if callReplacement.argPos < 1 {
				// inject at the last position if negative argPos value
				argPos = len(e.Args)
			} else {
				argPos = callReplacement.argPos - 1
				if argPos > len(e.Args) {
					log.Fatalf("error requesting to put a context argument in a position beyond the last function parameter" + cfg.currentPkg.Fset.Position(pos).String())
				}
			}
			ctxExpr := cfg.getCtxExprAndAddImports(cfg.existingImports, cfg.newImports, callReplacement)
			var newArgs []ast.Expr
			newArgs = append(newArgs, e.Args[:argPos]...)
			newArgs = append(newArgs, ast.NewIdent(cfg.resolveCtxExprPackageWildcard(ctxExpr)))
			newArgs = append(newArgs, e.Args[argPos:]...)
			e.Args = newArgs
			cfg.modified = true
			cfg.astCallsModifiedNum++
		}
	}
}

// addContextParam adds additional context parameter.
func (cfg *transformerConfig) addContextParam(fl *ast.FieldList) {
	if fl.List == nil {
		names := []*ast.Ident{ast.NewIdent(cfg.CtxParamName)}
		// a little trick to avoid incorrectly printing coma after parameter declaration
		// due to missing position information
		typ := ast.Ident{NamePos: fl.Closing, Name: cfg.ctxParamTypeWithPkgAlias, Obj: nil}
		params := []*ast.Field{&ast.Field{Doc: nil, Names: names, Type: &typ, Tag: nil, Comment: nil}}
		fl.List = params
		// don't traverse list or parameters again
	} else {
		// we only want to process parameters (return types are represented by the same ast node type)
		// so we do recursive application on the parameters firgsPeld only
		astutil.Apply(fl, cfg.addContextParamNonEmptyListApply, nil)
	}
}

// addContextParamNonEmptyListApply adds additional context parameter
// to the existing list of declared function parameters (to be used
// with astutil.Apply function).
func (cfg *transformerConfig) addContextParamNonEmptyListApply(c *astutil.Cursor) bool {
	if fl, ok := c.Parent().(*ast.FieldList); ok && c.Name() == "List" {
		if c.Index() == 0 {
			// we may be adding a parameter to function signature but
			// also to function type representing type of a parameter;
			// in the latter case, both using param name and omitting
			// it is valid syntax but these two forms cannot be mixed
			if fl.List[0].Names == nil {
				c.InsertBefore(&ast.Field{Doc: nil, Names: nil, Type: ast.NewIdent(cfg.ctxParamTypeWithPkgAlias), Tag: nil, Comment: nil})
			} else {
				names := []*ast.Ident{ast.NewIdent(cfg.CtxParamName)}
				c.InsertBefore(&ast.Field{Doc: nil, Names: names, Type: ast.NewIdent(cfg.ctxParamTypeWithPkgAlias), Tag: nil, Comment: nil})
			}
		}
		// don't traverse any children to avoid spurious updates
		return false
	}
	return true
}

// addContextParamApply adds additional context parameter during AST
// traversal (to be used with astutil.Apply function).
func (cfg *transformerConfig) addContextParamApply(c *astutil.Cursor) bool {
	if fl, ok := c.Node().(*ast.FieldList); ok && c.Name() == "Params" {
		cfg.addContextParam(fl)
		return false
	}
	return true
}

// addContextInitStmt adds context variable definition at the
// beginning of the function's statement list.
func (cfg *transformerConfig) addContextInitStmt(stmtsList []ast.Stmt, sigPos token.Pos) []ast.Stmt {
	newStmt := ast.AssignStmt{
		Lhs:    []ast.Expr{ast.NewIdent(cfg.CtxParamName)},
		TokPos: sigPos, // use concrete position to avoid being split by a comment leading to syntax error
		Tok:    token.DEFINE,
		Rhs:    []ast.Expr{ast.NewIdent(cfg.CtxParamInvalid)}}
	var newStmtsList []ast.Stmt
	newStmtsList = append(newStmtsList, &newStmt)
	newStmtsList = append(newStmtsList, stmtsList...)
	return newStmtsList
}

// getCtxExprAndAddImports records which files need to get injected with
// import of the package defining context. It returns a fully fleshed
// out context expression with wild cards for imported package and
// (potentxially custom) parameter name filled with correct values.
func (cfg *transformerConfig) getCtxExprAndAddImports(existingImports map[string]string, newImports map[string]string, callReplacement *replacementInfo) string {
	if len(callReplacement.ctxImports) > 1 {
		log.Fatalf("currently only supporting one custom import per library call in the config file")
	}
	ctxExpr := callReplacement.ctxExpr
	if ctxExpr == "" {
		// context expression has not been initialized to reflect
		// "custom" context parameter name; initialize it using
		// "default" context parameter name
		ctxExpr = replaceCtxExprWildcard(ctxWildcard, callReplacement.ctxRegExpr, cfg.CtxParamName)
	}
	for newImp, newAlias := range callReplacement.ctxImports {
		if alias, exists := existingImports[newImp]; exists {
			// there is an existing import with a given path
			if strings.Contains(ctxExpr, aliasWildCard) {
				// we need an alias to fill in placeholder in ctxExpr
				if alias == "" {
					// existing import does not have an alias
					if newAlias == "" {
						log.Fatalf("alias placeholder for library call in the config file exists withou alias itself being defined")
					} else {
						newImports[newImp] = newAlias
						return replaceCtxExprWildcard(aliasWildCard, ctxExpr, newAlias)
					}
				} else {
					// existing import does not have an alias - use it
					newImports[newImp] = alias
					return replaceCtxExprWildcard(aliasWildCard, ctxExpr, alias)
				}
			} else {
				// import (if any) is hard-coded in the expression, no
				// need for replacing anything in the expression but
				// we still need to add import alias if specified
				if newAlias != "" {
					newImports[newImp] = newAlias
				}
			}
		} else {
			// no existing import with a given path
			if strings.Contains(ctxExpr, aliasWildCard) {
				if newAlias == "" {
					log.Fatalf("alias placeholder for library call in the config file exists withou alias itself being defined")
				} else {
					newImports[newImp] = newAlias
					return replaceCtxExprWildcard(aliasWildCard, ctxExpr, newAlias)
				}
			} else {
				newImports[newImp] = newAlias
			}
		}
	}
	return ctxExpr
}

// resolveCtxExprPackageWildcard resolves package-related wildcard
// portion of the context expression (it is resolved based on package
// import information in a given file).
func (cfg *transformerConfig) resolveCtxExprPackageWildcard(expr string) string {
	// if the caller contains context parameter at
	// the right position but this context
	// parameter represents custom context, the
	// "regular" context extraction expression may
	// contain a wild card being a placeholder for
	// the package name where the "regular"
	// context is defined; this package name
	// cannot be known for all files as it depends
	// on the pre-rewrite content of a given
	// file's imports statement
	pkgAlias, importFound := cfg.existingImports[cfg.CtxPkgPath]
	var replacementName string
	if pkgAlias != "" {
		replacementName = pkgAlias
	} else if !importFound && cfg.CtxPkgAlias != "" {
		replacementName = cfg.CtxPkgAlias
	} else {
		replacementName = cfg.CtxPkgName
	}
	return replaceCtxExprWildcard(ctxPrefWildcard, expr, replacementName)
}
