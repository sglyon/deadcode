package main

// This entire file is dead code. None of its symbols are referenced
// from main.go. staticcheck should flag each one.

type orphanType struct {
	x int
	y int // never accessed
}

func orphanFunc() string {
	return "orphan"
}

const orphanConst = 42
