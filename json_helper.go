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
	"encoding/json"
	"log"
)

// UnmarshalJSON unmarshals function replacement info from JSON byte
// data.
func (m fnReplacementInfo) UnmarshalJSON(b []byte) error {
	var data []interface{}
	err := json.Unmarshal(b, &data)
	if err != nil {
		return err
	}
	for _, mapping := range data {
		fnDesc := mapping.(map[string]interface{})
		name := fnDesc["Name"].(string)
		recvMapping := fnDesc["Recv"]
		recv := getRecvStringFromJson(recvMapping)
		callReplacement := replacementInfo{}
		if fnDesc["NewName"] != nil {
			callReplacement.newName = fnDesc["NewName"].(string)
		} else {
			callReplacement.newName = ""
		}
		if fnDesc["ArgPos"] != nil {
			callReplacement.argPos = int(fnDesc["ArgPos"].(float64))
		} else {
			callReplacement.argPos = 1
		}
		if fnDesc["CtxImports"] != nil {
			callReplacement.ctxImports = make(map[string]string)
			for _, mapping := range fnDesc["CtxImports"].([]interface{}) {
				ctxImports := mapping.(map[string]interface{})
				impStr := ctxImports["Import"].(string)
				if ctxImports["Alias"] == nil {
					callReplacement.ctxImports[impStr] = ""
				} else {
					callReplacement.ctxImports[impStr] = ctxImports["Alias"].(string)
				}
			}
		} else {
			callReplacement.ctxImports = nil
		}
		if fnDesc["CtxExpr"] != nil {
			callReplacement.ctxRegExpr = fnDesc["CtxExpr"].(string)
		} else {
			callReplacement.ctxRegExpr = ""
		}
		callReplacement.ctxExpr = ""
		mapFnToReplacementInfo(m, name, recv, &callReplacement)
	}
	return nil
}

// UnmarshalJSON unmarshals function/method info from JSON byte data.
func (m fnInfo) UnmarshalJSON(b []byte) error {
	var data []interface{}
	err := json.Unmarshal(b, &data)
	if err != nil {
		return err
	}
	for _, mapping := range data {
		fnDesc := mapping.(map[string]interface{})
		name := fnDesc["Name"].(string)
		pkgPath := fnDesc["PkgPath"].(string)
		pkgName := fnDesc["PkgName"].(string)
		recvMapping := fnDesc["Recv"]
		recv := getRecvStringFromJson(recvMapping)
		mapFnToPkgInfo(m, name, recv, pkgPath, pkgName)
	}
	return nil
}

// UnmarshalJSON unmarshals type info from JSON byte data.
func (m typeInfo) UnmarshalJSON(b []byte) error {
	var data []interface{}
	err := json.Unmarshal(b, &data)
	if err != nil {
		return err
	}
	for _, mapping := range data {
		typeDesc := mapping.(map[string]interface{})
		typeName := typeDesc["Name"].(string)
		pkgPath := typeDesc["PkgPath"].(string)
		pkgName := typeDesc["PkgName"].(string)
		mapTypeToPkgInfo(m, typeName, pkgPath, pkgName)
	}
	return nil
}

// getRecvStringFromJson computes a string representing method
// receiver type from JSON representation.
func getRecvStringFromJson(mapping interface{}) string {
	if mapping == nil {
		return ""
	}
	recvDesc := mapping.(map[string]interface{})
	pkgPath := recvDesc["PkgPath"].(string)
	pkgName := recvDesc["PkgName"].(string)
	recvType := recvDesc["Type"].(string)
	return getQualifiedType(recvType, pkgPath, pkgName)
}

// getQualifiedType returns a string representing a type (which may be
// a pointer type) qualified with package name and path.
func getQualifiedType(orgTypName string, pkgPath string, pkgName string) string {
	if len(orgTypName) == 0 {
		log.Fatalf("unexpected empty type in config file")
	}
	if orgTypName[0:1] == "*" {
		// receiver is a pointer type
		typName := orgTypName[1:]
		if typName[0:1] == "*" {
			log.Fatalf("unexpected multiple level pointer type in config file")
		}
		return "*" + pkgPath + pkgName + "." + typName
	}
	return pkgPath + pkgName + "." + orgTypName
}

// mapTypeToPkgInfo adds package info (name and path) to a
// type->pkgInfo map.
func mapTypeToPkgInfo(types typeInfo, typeName string, pkgPath string, pkgName string) {
	pkgs, exists := types[typeName]
	if !exists {
		pkgs = make(pkgInfo)
		types[typeName] = pkgs
	}
	mapPkgInfo(pkgs, pkgPath, pkgName)
}

// mapFnToPkgInfo adds package info and receiver info to a
// func/method->receiver->pkgInfo map.
func mapFnToPkgInfo(fns fnInfo, fnName string, recv string, pkgPath string, pkgName string) {
	receivers, exists := fns[fnName]
	if !exists {
		receivers = make(map[string]pkgInfo)
		fns[fnName] = receivers
	}
	pkgs, exists := receivers[recv]
	if !exists {
		pkgs = make(pkgInfo)
		receivers[recv] = pkgs
	}
	mapPkgInfo(pkgs, pkgPath, pkgName)
}

// mapPkgInfo adds a package name to a
// pkgPath->pkgName->exists-on-the-path map.
func mapPkgInfo(pkgs pkgInfo, pkgPath string, pkgName string) {
	pkgNames, exists := pkgs[pkgPath]
	if !exists {
		pkgNames = make(map[string]bool)
		pkgs[pkgPath] = pkgNames
	}
	pkgNames[pkgName] = true
}

// mapFnToReplacementInfo adds function/method info and replacement
// info to a func/method->receiver->replacementInfo map.
func mapFnToReplacementInfo(fns fnReplacementInfo, fnName string, recv string, replacement *replacementInfo) {
	receivers, exists := fns[fnName]
	if !exists {
		receivers = make(map[string]*replacementInfo)
		fns[fnName] = receivers
	}
	receivers[recv] = replacement
}
