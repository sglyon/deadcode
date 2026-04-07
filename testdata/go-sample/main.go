// Package main is a deliberately-mixed fixture exercising both Go
// adapters: the package-level unused symbols below are caught by
// staticcheck, while the unused export in ./lib is only caught by
// x/tools/cmd/deadcode.
package main

import (
	"fmt"

	"example.com/gosample/lib"
)

type UsedThing struct {
	Name string
}

type unusedThing struct {
	value int
}

const UsedConst = "hello"
const unusedConst = "goodbye"

func main() {
	t := UsedThing{Name: "world"}
	fmt.Println(UsedConst, t.Name, usedHelper(2), lib.UsedByMain())
}

func usedHelper(x int) int {
	return x * 2
}

func unusedHelper(x int) int {
	return x + 1
}
