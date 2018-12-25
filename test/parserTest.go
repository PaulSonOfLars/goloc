package test

import "fmt"

func main() {
	noLoad()
	// a comment
	fmt.Println("hello there")
	// another
	fmt.Printf("hello there %s", "in a format call")
}

func noLoad() {
	// junk
}
