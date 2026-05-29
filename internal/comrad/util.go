package comrad

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

func NormalizeSHA256(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "sha256:") {
		return s
	}
	return "sha256:" + s
}

func FileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), n, nil
}

func VerifyFileSHA256(path, expected string) error {
	got, _, err := FileSHA256(path)
	if err != nil {
		return err
	}
	if NormalizeSHA256(got) != NormalizeSHA256(expected) {
		return fmt.Errorf("%s: expected %s got %s", FailureArtifactDigestMismatch, NormalizeSHA256(expected), got)
	}
	return nil
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func SafeJoin(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(rootAbs, filepath.Clean(rel))
	joinedAbs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	if joinedAbs != rootAbs && !strings.HasPrefix(joinedAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root")
	}
	return joinedAbs, nil
}

func Contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func HasAll(list []string, required []string) bool {
	for _, req := range required {
		if !Contains(list, req) {
			return false
		}
	}
	return true
}
