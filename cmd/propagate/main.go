// Copyright (c) 2021 Uber Technologies, Inc.
//
// Licensed under the Uber Non-Commercial License (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at the root directory of this project.
//
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"

	"github.com/uber-research/go-context-propagate"
)

// DefaultDebugLevel defines default debugging level for generating
// additional information during context propagation.
const DefaultDebugLevel = 2

func main() {
	// input to the tool
	configFilePath := flag.String("config", "", "path to the JSON configuration file")
	// additional output from the tool
	debugFilePath := flag.String("debug", "", "path to the JSON file containing additional comments and warnings")
	flag.Parse()

	propagate.Run(*configFilePath, *debugFilePath, nil, DefaultDebugLevel)
}
