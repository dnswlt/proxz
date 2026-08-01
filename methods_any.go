//go:build any_methods

package main

import "fmt"

func checkMethodAllowed(method string) error {
	return nil
}

func printAllowedMethods() {
	fmt.Println("GET POST PUT PATCH DELETE HEAD OPTIONS")
}
