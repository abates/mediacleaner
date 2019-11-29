package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/abates/goexiftool"
	"github.com/abates/mediacleaner"
	"github.com/mh-orange/vfs"
)

var (
	errNoFile           = errors.New("File removed prior to processing")
	errIsDir            = errors.New("File is a directory")
	errNoExifDate       = errors.New("Exif data has no known date")
	errAlreadyProcessed = errors.New("File has already been processed")
)

type job struct {
	fs          vfs.FileSystem
	root        string
	filename    string
	newFilename string
	newDir      string
}

func (jb *job) Name() string {
	return jb.filename
}

func (jb *job) Check() error {
	if fi, err := jb.fs.Stat(jb.filename); vfs.IsNotExist(err) {
		return &mediacleaner.CheckError{errNoFile}
	} else if fi.IsDir() {
		return &mediacleaner.CheckError{errIsDir}
	}

	dir := []byte(path.Dir(jb.filename))
	if mediacleaner.YearMonthDir.Match(dir) || mediacleaner.YearMonthDayDir.Match(dir) {
		fn := []byte(path.Base(jb.filename))
		if mediacleaner.FilePrefix.Match(fn) {
			return &mediacleaner.CheckError{errAlreadyProcessed}
		}
	}

	t, err := mediacleaner.GetDateFromFilename(jb.filename)
	if err != nil {
		exif, err := goexiftool.NewMediaFile(path.Join(jb.root, jb.filename))
		if err != nil {
			return &mediacleaner.CheckError{err}
		}

		t, err = exif.GetDate()
		if err != nil {
			return &mediacleaner.CheckError{errNoExifDate}
		}
	}
	jb.newFilename = t.Format("2006_01_02_15:04:05")
	jb.newDir = t.Format("/2006/01")

	jb.newFilename, err = mediacleaner.GetPrefix(jb.fs, jb.newDir, jb.newFilename)
	if err == nil {
		jb.newFilename = fmt.Sprintf("%s%s", jb.newFilename, strings.ToLower(path.Ext(jb.filename)))
	}
	return err
}

func (jb *job) Execute() error {
	err := vfs.MkdirAll(jb.fs, jb.newDir, 0750)
	if err == nil {
		newFilename := path.Join(jb.newDir, jb.newFilename)
		err = jb.fs.Rename(jb.filename, newFilename)
		if err == nil {
			jb.filename = newFilename
		} else {
			err = &mediacleaner.ExecuteError{fmt.Sprintf("failed to rename %q to %q", jb.filename, newFilename), err}
		}
	} else {
		err = &mediacleaner.ExecuteError{fmt.Sprintf("failed creating directory %q", jb.newDir), err}
	}
	return err
}

func main() {
	p := mediacleaner.Run(os.Args, func(fs vfs.FileSystem, filename string, root string) mediacleaner.Job {
		return &job{fs: fs, root: root, filename: filename}
	})
	p.Wait()
}
