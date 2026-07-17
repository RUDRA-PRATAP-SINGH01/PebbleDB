// Package util implements general utility functions for checksumming, file operations,
// and string manipulators used across framework verification and logging stages.
//
// Dependency Rules:
// - This is a leaf package. It must not import other framework packages.
package util

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// ComputeFileSHA256 returns the SHA-256 hex checksum of the file at path.
func ComputeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// CopyFile duplicates a file from src to dst.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
