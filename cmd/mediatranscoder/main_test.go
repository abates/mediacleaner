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
	"github.com/mh-orange/cmd"
	"github.com/mh-orange/ffmpeg"
	"github.com/mh-orange/vfs"
)

func readFile(t *testing.T, filename, ext string) []byte {
	for ext := filepath.Ext(filename); len(ext) > 0; ext = filepath.Ext(filename) {
		filename = filename[0 : len(filename)-len(ext)]
	}

	content := []byte{}
	filename = fmt.Sprintf("testdata%s.%s", filename, ext)
	if _, err := os.Stat(filename); err == nil {
		content, err = ioutil.ReadFile(filename)
		if err != nil {
			t.Logf("Failed to read file %q: %v", filename, err)
		}
	}

	return content
}

func mockCmd(t *testing.T, input string) func() {
	oldFfprobe := ffmpeg.Ffprobe
	oldFfmpeg := ffmpeg.Ffmpeg

	ffm := &cmd.TestCmd{
		Stdout: readFile(t, input, "ffmpeg"),
		Stderr: readFile(t, input, "ffmpeg_err"),
	}

	ffp := &cmd.TestCmd{
		Stdout: readFile(t, input, "ffprobe"),
		Stderr: readFile(t, input, "ffprobe_err"),
	}

	ffmpeg.Ffmpeg = ffm
	ffmpeg.Ffprobe = ffp

	return func() {
		ffmpeg.Ffmpeg = oldFfmpeg
		ffmpeg.Ffprobe = oldFfprobe
	}
}

func TestJobCheck(t *testing.T) {
	fs := vfs.NewOsFs("testdata")
	defer fs.Close()
	tests := []struct {
		filename string
		wantErr  error
	}{
		{"/2010_01_01_00:00:00_0001.mov", errNotRenamed},
		{"/2010/2010_01_01_00:00:00_0001.mov", errNotRenamed},
		{"/2010/01/2010_01_01_00:00:00_0001.mp4", errAlreadyMp4},
		{"/2010/01/2010_01_01_00:00:00_0002.jpg", errNotVideo},
		{"/2010/01/2010_01_01_00:00:00_0003.mpg", nil},
		{"/2010/01/foo.mpg", errNotRenamed},
		{"/2010/01/01/2010_01_01_00:00:00_0003.mpg", nil},
	}

	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			defer mockCmd(t, test.filename)()
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

	filepath.Walk("testdata", func(inpath string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			outpath := strings.TrimPrefix(inpath, "testdata")
			vfs.MkdirAll(fs, filepath.Dir(outpath), 0750)
			in, _ := os.Open(inpath)
			out, err := fs.Create(outpath)
			if err == nil {
				io.Copy(out, in)
				in.Close()
				if closer, ok := out.(io.Closer); ok {
					closer.Close()
				}
			} else {
				panic(err.Error())
			}
		}
		return err
	})

	tests := []struct {
		filename string
		wantLog  string
		wantErr  string
	}{
		{"/2010/01/2010_01_01_00:00:00_0003.mpg", "Transcoding \"/2010/01/2010_01_01_00:00:00_0003.mpg\"\n", ""},
		{"/2010/01/2010_01_01_00:00:00_0004.txt.gz", "Transcoding \"/2010/01/2010_01_01_00:00:00_0004.txt.gz\"\n", fmt.Sprintf("%v/2010/01/2010_01_01_00:00:00_0004.txt.gz: Invalid data found when processing input", tempdir)},
	}

	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			defer mockCmd(t, test.filename)()
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
				// make sure original file was removed
				if _, err := fs.Stat(test.filename); !vfs.IsNotExist(err) {
					t.Errorf("Wanted original file to have been removed, got %v", err)
				}
			} else {
				// make sure original file still exists
				if _, err := fs.Stat(test.filename); err != nil {
					t.Errorf("Wanted original file still exist, got %v", err)
				}

				if test.wantErr != gotErr.Error() {
					t.Errorf("Wanted error %q got %q", test.wantErr, gotErr.Error())
				}
			}
		})
	}
}
