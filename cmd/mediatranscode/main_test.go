package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abates/mediacleaner"
	"github.com/mh-orange/vfs"
)

func TestJobCheck(t *testing.T) {
	fs := vfs.NewOsFs("testdata")
	defer fs.Close()
	tests := []struct {
		filename string
		wantErr  error
	}{
		{"test1.mp4", errAlreadyMp4},
		{"test2.jpg", errNotVideo},
		{"test3.mpg", nil},
	}

	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			jb := &job{fs: fs, root: "testdata/", filename: test.filename}
			gotErr := jb.Check()
			if ce, ok := gotErr.(*mediacleaner.CheckError); ok {
				gotErr = ce.Cause
			}

			if test.wantErr == gotErr {
				if gotErr == nil {
				}
			} else {
				t.Errorf("Wanted error %v got %v", test.wantErr, gotErr)
			}
		})
	}
}

func TestJobExecute(t *testing.T) {
	tempdir, _ := ioutil.TempDir("", "osfs_test")
	defer os.RemoveAll(tempdir)
	fs := vfs.NewOsFs(tempdir)
	defer fs.Close()

	files, _ := filepath.Glob("testdata/*")
	for _, filename := range files {
		in, err := os.Open(filename)
		if err == nil {
			out, err := fs.Create(filepath.Base(filename))
			if err == nil {
				io.Copy(out, in)
				in.Close()
				if closer, ok := out.(io.Closer); ok {
					closer.Close()
				}
			} else {
				panic(err.Error())
			}
		} else {
			panic(err.Error())
		}
	}

	tests := []struct {
		filename        string
		wantNewFilename string
		wantLog         string
		wantErr         string
	}{
		{"test3.mpg", "test3.mp4", "Transcoding \"test3.mpg\"\n", ""},
		{"test4.txt.gz", "", "Transcoding \"test4.txt.gz\"\n", fmt.Sprintf("%v/test4.txt.gz: Invalid data found when processing input", tempdir)},
	}

	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			builder := &strings.Builder{}
			oldLogger := mediacleaner.Logger
			mediacleaner.Logger = log.New(builder, "", 0)
			defer func() { mediacleaner.Logger = oldLogger }()
			mediacleaner.Output = ioutil.Discard

			jb := &job{
				fs:       fs,
				root:     tempdir,
				filename: test.filename,
			}
			gotErr := jb.Execute()
			if ce, ok := gotErr.(*mediacleaner.ExecuteError); ok {
				gotErr = ce.Cause
			}

			gotLog := builder.String()
			if test.wantLog != gotLog {
				t.Errorf("Wanted log %q got %q", test.wantLog, gotLog)
			}

			if gotErr == nil {
				if _, err := fs.Stat(test.wantNewFilename); err != nil {
					t.Errorf("Wanted file to have been renamed, got %v", err)
				}

				if _, err := fs.Stat(test.filename); !vfs.IsNotExist(err) {
					t.Errorf("Wanted file to have been renamed, got %v", err)
				}
			} else if test.wantErr != gotErr.Error() {
				t.Errorf("Wanted error %q got %q", test.wantErr, gotErr.Error())
			}
		})
	}
}
