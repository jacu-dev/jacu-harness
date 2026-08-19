package project

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
)

func ID(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(resolved))
	return "prj_" + fmt.Sprintf("%x", digest[:8]), nil
}
