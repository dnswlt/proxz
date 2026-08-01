//go:build !writes

package main

import "fmt"

const (
	usageTagline    = "proxz - read-only proxy for Jira/Confluence/Bitbucket Data Center REST APIs"
	usageWriteVerbs = ""
)

func checkMethodAllowed(method string) error {
	if method != "get" && method != "GET" {
		return fmt.Errorf("unknown command %q; proxz only performs GET requests", method)
	}
	return nil
}

func printAllowedMethods() {
	fmt.Println("GET")
}
