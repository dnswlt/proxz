package main

// Lightweight obfuscation for personal access tokens at rest.
//
// This is deliberately NOT strong cryptography, and it is important to be
// honest about that. The key material is compiled into this binary, so anyone
// who can run proxz and read this source can recover a stored token. Treat the
// config file as sensitive regardless.
//
// The threat being addressed is narrower: an LLM with broad read access should
// not be able to `cat ~/.config/proxz/config.json` and walk away with a PAT it
// can echo into a curl command or memorize. Scrambling turns the file into
// base64 noise, and proxz deliberately offers no command that prints a token
// back out. Recovering one requires deliberately reverse-engineering this
// file, which is a very different act from stumbling over a plaintext secret.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// scramblePrefix tags scrambled values so we can tell them apart from a token
// that was pasted into the config file by hand, and so the format can be
// versioned if the scheme ever changes.
const scramblePrefix = "pxz1:"

// buildKey is injected at link time from a locally generated, never-committed
// key file:
//
//	go build -ldflags "-X main.buildKey=$(cat .proxz-key)" -o proxz .
//
// Use the Makefile, which generates .proxz-key once and reuses it. Because the
// key lives only on this machine, reading this source tells an attacker
// nothing about your stored tokens.
//
// If buildKey is empty, the constants below are used instead. That keeps `go
// test` and `go run` working, but such a build is scrambled with a key
// published in this repo - dev only, never for real tokens.
var buildKey string

// Fallback key material, split into pieces that are only combined at runtime
// so that `strings proxz` does not hand over the key as one contiguous blob.
var (
	keyPartA = []byte{0x8d, 0x1f, 0xa6, 0x42, 0xc7, 0x0b, 0x93, 0xe5}
	keyPartB = []byte{0x2a, 0x74, 0xd8, 0x16, 0x5f, 0xb1, 0x3c, 0xe9}
	keyPartC = []byte{0xf0, 0x4b, 0x27, 0x9d, 0x61, 0xac, 0x38, 0x52}
	keySalt  = "proxz/v1/token-at-rest"
)

// usingBuildKey reports whether this binary was linked with a private key.
func usingBuildKey() bool { return buildKey != "" }

// deriveKey builds the AES key from the injected key and the compiled-in parts.
func deriveKey() []byte {
	h := sha256.New()
	h.Write([]byte(keySalt))
	h.Write([]byte(buildKey))
	// Interleave rather than concatenate, so no single byte slice in the
	// binary corresponds to a prefix of the key.
	for i := range keyPartA {
		h.Write([]byte{keyPartA[i], keyPartB[i], keyPartC[i]})
	}
	return h.Sum(nil)
}

func newGCM() (cipher.AEAD, error) {
	block, err := aes.NewCipher(deriveKey())
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// scramble obfuscates a token for storage in the config file.
func scramble(plain string) (string, error) {
	gcm, err := newGCM()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return scramblePrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// unscramble recovers a token written by scramble. A value without the
// expected prefix is returned as-is, so a token pasted into the config by hand
// still works; it just sits there in plaintext until the next `proxz login`.
func unscramble(stored string) (string, error) {
	if !strings.HasPrefix(stored, scramblePrefix) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, scramblePrefix))
	if err != nil {
		return "", fmt.Errorf("token is not valid base64: %w", err)
	}
	gcm, err := newGCM()
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("stored token is truncated")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// By far the most likely cause is a binary built with a different
		// .proxz-key than the one that wrote this token.
		return "", errors.New("stored token could not be unscrambled; " +
			"this binary was probably built with a different .proxz-key - re-run: proxz login <site>")
	}
	return string(plain), nil
}
