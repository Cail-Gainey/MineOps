// Command verify-sidecar verifies the detached Wails Ed25519ph signature of a release manifest.
package main

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
)

type sidecar struct {
	DigestAlgo    string `json:"digestAlgo"`
	Digest        string `json:"digest"`
	SignatureAlgo string `json:"signatureAlgo"`
	Signature     string `json:"signature"`
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: go run verify-sidecar.go <manifest> <sidecar> <public-key>")
		os.Exit(2)
	}
	manifest, err := os.ReadFile(os.Args[1])
	check(err)
	var metadata sidecar
	encoded, err := os.ReadFile(os.Args[2])
	check(err)
	check(json.Unmarshal(encoded, &metadata))
	if metadata.DigestAlgo != "sha512" || metadata.SignatureAlgo != "ed25519ph" {
		check(fmt.Errorf("unsupported signature algorithms %s/%s", metadata.DigestAlgo, metadata.SignatureAlgo))
	}
	digest := sha512.Sum512(manifest)
	if base64.StdEncoding.EncodeToString(digest[:]) != metadata.Digest {
		check(fmt.Errorf("manifest digest mismatch"))
	}
	keyPEM, err := os.ReadFile(os.Args[3])
	check(err)
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		check(fmt.Errorf("public key is not PEM encoded"))
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	check(err)
	publicKey, ok := parsed.(ed25519.PublicKey)
	if !ok {
		check(fmt.Errorf("public key is not Ed25519"))
	}
	signature, err := base64.StdEncoding.DecodeString(metadata.Signature)
	check(err)
	check(ed25519.VerifyWithOptions(publicKey, digest[:], signature, &ed25519.Options{Hash: crypto.SHA512}))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
