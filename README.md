# Automate Context Propagation for Go (go-context-propagate) 

## Overview

The Go language ecosystem, particularly in the context of backend services, defines the concept of *context* that "carries deadlines, cancellation signals, and other request-scoped values across API boundaries and between processes". This quote is directly lifted from the documentation of Go's context [package](https://golang.org/pkg/context/), but context can be also encapsulated by user-defined types.

In order for the context to be useful, it has to be somehow propagated from the point in the code where it is received from another service (as part of a request) to the point in the code where it is used (as part of another request or as an argument to a relevant "leaf" API call at the bottom of some call chain).

The [Go-recommended](https://blog.golang.org/context) method of propagating context is to pass it on as "as the first argument to every function on the call path between incoming and outgoing requests". The goal of this project is to reduce manual effort required to propagate context argument through the code of a given Go service. The tool transforms a set of Go source files to propagate context from the context "use points" up the call chains until the point where the context is available is reached.

## Installation

`go get -u github.com/uber-research/go-context-propagate/...`

Please note that we must currently disable Go modules (`GO111MODULE=off` in the examples below) as they seem to interfere with the behavior of the tool chain used to implement this project.

## Running

Let's use the following piece of Go source code as our example (also in [example/example.go](example/example.go)):

```go
package main

import "log"

func main() {
	foo(true)
}

func foo(p bool) {
	bar(p)
}

func bar(p bool) {
	log.Print(p)
}
```

Assume that you would like to use context as the first argument to the `log.Print` "leaf" function call. We configure the tool using a file in the JSON format. In order to transform this simple example, we can use the following configuration ([example/example.json](example/example.json)):

```json
{
  "CtxPkgPath"      : "context",
  "CtxPkgName"      : "context",
  "CtxParamType"    : "Context",
  "CtxParamName"    : "ctx",
  "LibPkgPath"      : "log",
  "LibPkgName"      : "log",
  "LibFns"          : [{"Name": "Print"}],
  "LoadPaths"       : ["github.com/uber-research/go-context-propagate/example"]
  "CtxParamInvalid" : "Background()",
}
```

The type of context we are propagating here is the one defined in Go's context [package](https://golang.org/pkg/context/) (as defined in the `CgxPkgPath` , `CtxPkgName`, and `CtxParamName` fields). The name of the context parameter is user-defined as well (`CtxParamName`) so that it can be chosen to avoid name clashes. The "leaf" function is identified by the package name where it is defined (`LibPkgPath` and `LibPkgName` fields), and by its name (`LibFns` array field -  more than one function in the same package can be listed). Finally, the tool has to know the path where the source files to be modified reside relative to `GOPATH` (`LoadPaths` field). Please not that in our example, no context is available - the tool will handle this by injecting "invalid" (or "artificial) context (defined as an expression exported by the context package in the `CtxParamInvalid` field) once it reaches the top of the call chain.

Transformation of our example is triggered as follows:

```bash
cd $GOPATH/src/github.com/uber-research/go-context-propagate
GO111MODULE=off go run cmd/propagate/main.go -config example/example.json
```

The resulting transformed Go source file is deposited in the same location as the original one, with an added `.mod `extension ([example/example.go.mod](example/example.go.mod)):

```go
package main

import (
        "context"
        "log"
)

func main() {
        ctx := context.Background()
        foo(ctx, true)
}

func foo(ctx context.Context, p bool) {
        bar(ctx, p)
}

func bar(ctx context.Context, p bool) {
        log.Print(ctx, p)
}
```

Please not that in addition to injecting context argument to the `log.Print` call and propagating it up the call chain, both artificial context was injected into the `main` function and the required import statement for the context package was also automatically injected to the existing import clause.

While this example is intentionally very simple, the tool is quite complex to be able to handle real-life production Go code. In particular, it handles both Go functions and methods, and automatically modified other Go language constructs affected by function/methods signature modifications (e.g., interface and named types). One can explore these additional features by analyzing unit tests that can be found in the [testdata/src](testdata/src) directory (source files, with transformed files in the [testdata/src/expected](testdata/src/expected) directory)  [testdata/config](testdata/config) directory (config files).

## Testing

```bash
GO111MODULE=off GOPATH=$GOPATH:$GOPATH/src/github.com/uber-research/go-context-propagate/testdata go test -v
```



