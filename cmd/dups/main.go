package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	images map[[sha256.Size]byte][]string
)

func analyze(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if info.IsDir() {
		if strings.HasSuffix(path, "_dups") {
			return filepath.SkipDir
		}
		return nil
	}

	fmt.Printf("Reading %s\n", path)
	file, err := os.Open(path)
	if err == nil {
		digest := sha256.New()
		_, err = io.Copy(digest, file)
		if err == nil {
			var hash [sha256.Size]byte
			copy(hash[:], digest.Sum(nil))
			if dupfiles, found := images[hash]; found {
				dupdir := fmt.Sprintf("%s_dups", dupfiles[0])
				err = os.MkdirAll(dupdir, 0750)
				if err == nil {
					err = os.Rename(path, filepath.Join(dupdir, filepath.Base(path)))
				}
			}
			images[hash] = append(images[hash], path)
		}
	}

	return err
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <path>\n", os.Args[0])
		os.Exit(-1)
	}

	images = make(map[[sha256.Size]byte][]string)

	err := filepath.Walk(os.Args[1], analyze)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAILURE: %v", err)
		os.Exit(-1)
	}

	for _, files := range images {
		if len(files) > 1 {
			fmt.Printf("Duplicates:\n")
			for _, file := range files {
				fmt.Printf("\t%s\n", file)
			}
		}
	}
}
