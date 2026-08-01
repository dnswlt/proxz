//go:build !any_methods

package main

import "fmt"

func checkMethodAllowed(method string) error {
	if method != "get" && method != "GET" {
		return fmt.Errorf("unknown command %q; proxz only performs GET requests", method)
	}
	return nil
}

func printAllowedMethods() {
	fmt.Println("GET")
}
