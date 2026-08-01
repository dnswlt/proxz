//go:build any_methods

package main

import "fmt"

const (
	usageTagline    = "proxz - proxy for Jira/Confluence/Bitbucket Data Center REST APIs (writes enabled)"
	usageWriteVerbs = "  proxz post|put|patch|delete <url>\n" +
		"                               Same, with a JSON body read from stdin\n"
)

func checkMethodAllowed(method string) error {
	return nil
}

func printAllowedMethods() {
	fmt.Println("GET POST PUT PATCH DELETE HEAD OPTIONS")
}
