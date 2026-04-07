// Package main is a deliberately-mixed fixture for the staticcheck
// adapter: some things are used, others are dead code of different
// kinds.
package main

import "fmt"

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
	fmt.Println(UsedConst, t.Name, usedHelper(2))
}

func usedHelper(x int) int {
	return x * 2
}

func unusedHelper(x int) int {
	return x + 1
}
