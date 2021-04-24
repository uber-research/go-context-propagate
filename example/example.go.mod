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
