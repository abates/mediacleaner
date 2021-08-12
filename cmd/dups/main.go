package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	images  map[[sha256.Size]byte][]string
	remove  bool
	rename  bool
	verbose bool
)

func init() {
	flag.BoolVar(&remove, "remove", false, "remove duplicate files")
	flag.BoolVar(&rename, "rename", false, "rename duplicate files")
	flag.BoolVar(&verbose, "verbose", false, "print verbose log")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "%s [-remove] [-rename] <path>\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "Either -remove or -rename must be specified, but not both\n")
	}
}

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
	log.Printf("Analyzing %q", path)

	file, err := os.Open(path)
	if err == nil {
		digest := sha256.New()
		_, err = io.Copy(digest, file)
		if err == nil {
			var hash [sha256.Size]byte
			copy(hash[:], digest.Sum(nil))
			if dupfiles, found := images[hash]; found {
				log.Printf("%q is a duplicate of %q", path, dupfiles[0])
				if rename {
					dupdir := fmt.Sprintf("%s_dups", dupfiles[0])
					err = os.MkdirAll(dupdir, 0750)
					if err == nil {
						newpath := filepath.Join(dupdir, filepath.Base(path))
						log.Printf("Renaming %q to %q", path, newpath)
						err = os.Rename(path, newpath)
					}
				} else if remove {
					log.Printf("Removing %q", path)
					err = os.Remove(path)
				}
			}
			images[hash] = append(images[hash], path)
		}
	}

	return err
}

func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(-1)
	}

	if !verbose {
		log.SetOutput(ioutil.Discard)
	}

	inputPath := flag.Args()[0]

	images = make(map[[sha256.Size]byte][]string)

	err := filepath.Walk(inputPath, analyze)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAILURE: %v", err)
		os.Exit(-1)
	}

	for _, files := range images {
		if len(files) > 1 {
			log.Printf("Duplicates:\n")
			for _, file := range files {
				log.Printf("\t%s\n", file)
			}
		}
	}
}
