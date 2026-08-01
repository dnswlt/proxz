//go:build ignore

// Command buildkey prints the machine-local key that the build links into the
// binary, creating it on first use. It exists so that generating the key needs
// nothing but a Go toolchain: the shell version wanted /dev/urandom, umask,
// head, base64 and chmod, which ruled out Windows outside Git Bash.
//
// Usage: go run buildkey.go [path]
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	path, err := keyPath()
	if err != nil {
		fail(err)
	}
	key, err := os.ReadFile(path)
	if err == nil {
		fmt.Print(string(key))
		return
	}
	if !os.IsNotExist(err) {
		fail(err)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		fail(err)
	}
	key = []byte(base64.StdEncoding.EncodeToString(raw))

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fail(err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "generated %s - keep it; rebuilding without it means re-running 'proxz login'\n", path)
	fmt.Print(string(key))
}

// keyPath mirrors the config location: the argument wins, then
// XDG_CONFIG_HOME, then the user's home directory.
func keyPath() (string, error) {
	if len(os.Args) > 1 && os.Args[1] != "" {
		return os.Args[1], nil
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "proxz", "build.key"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "proxz", "build.key"), nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "buildkey: "+err.Error())
	os.Exit(1)
}
