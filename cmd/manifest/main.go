package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"comrad/internal/comrad"
)

type manifest struct {
	GeneratedAt time.Time       `json:"generatedAt"`
	Artifacts   []manifestEntry `json:"artifacts"`
}

type manifestEntry struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
}

func main() {
	root := flag.String("root", ".", "root path used for relative manifest paths")
	out := flag.String("out", "", "output manifest path")
	flag.Parse()
	if *out == "" {
		log.Fatal("-out is required")
	}
	m := manifest{GeneratedAt: time.Now().UTC()}
	for _, path := range flag.Args() {
		sha, size, err := comrad.FileSHA256(path)
		if err != nil {
			log.Fatal(err)
		}
		rel, err := filepath.Rel(*root, path)
		if err != nil {
			rel = path
		}
		m.Artifacts = append(m.Artifacts, manifestEntry{Path: filepath.ToSlash(rel), SHA256: sha, SizeBytes: size})
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil && !strings.EqualFold(filepath.Dir(*out), ".") {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		log.Fatal(err)
	}
}
