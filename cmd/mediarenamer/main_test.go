package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"

	"github.com/abates/goexiftool"
	"github.com/abates/mediacleaner"
	"github.com/mh-orange/vfs"
)

func fakeExiftool(s ...string) *exec.Cmd {
	cs := []string{"-test.run=TestFakeExifTool", "--"}
	cs = append(cs, s...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestFakeExifTool(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	output, err := ioutil.ReadFile(args[0])
	if err == nil {
		os.Stdout.Write(output)
	} else {
		t.Errorf("Failed to read %q: %v", os.Args[len(os.Args)-1], err)
	}
	os.Exit(0)
}

func TestJobCheck(t *testing.T) {
	fs := vfs.NewOsFs("testdata")
	defer fs.Close()
	tests := []struct {
		filename        string
		wantNewDir      string
		wantNewFilename string
		wantErr         error
	}{
		{"/2010/01", "", "", errIsDir},
		{"/2010/02", "", "", errNoFile},
		{"/2010/01/2010_01_13_22:01:37_0000.jpg", "", "", errAlreadyProcessed},
		{"/2010/01/13/2010_01_13_22:01:37_0000.jpg", "", "", errAlreadyProcessed},
		{"/noexif.png", "", "", errNoExifDate},
		{"/nodate.png", "", "", errNoExifDate},
		{"/IMG_20130525_125511_332.jpg", "/2013/05", "2013_05_25_12:55:11_0000.jpg", nil},
	}

	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			outfile := fmt.Sprintf("testdata/%s.out", test.filename[0:len(test.filename)-len(filepath.Ext(test.filename))])
			if _, err := os.Stat(outfile); err == nil {
				goexiftool.ExifTool = fakeExiftool(outfile)
				defer func() { goexiftool.ExifTool = nil }()
			}

			jb := &job{fs: fs, root: "testdata/", filename: test.filename}
			gotErr := jb.Check()
			if ce, ok := gotErr.(*mediacleaner.CheckError); ok {
				gotErr = ce.Cause
			}

			if test.wantErr == gotErr {
				if gotErr == nil {
					if test.wantNewDir != jb.newDir {
						t.Errorf("Wanted newDir %q got %q", test.wantNewDir, jb.newDir)
					}

					if test.wantNewFilename != jb.newFilename {
						t.Errorf("Wanted newFilename %q got %q", test.wantNewFilename, jb.newFilename)
					}
				}
			} else {
				t.Errorf("Wanted error %v got %v", test.wantErr, gotErr)
			}
		})
	}
}

func TestJobExecute(t *testing.T) {
	tests := []struct {
		filename        string
		wantNewFilename string
		wantErr         error
	}{
		{"/IMG_20130525_125511_332.jpg", "/2013/05/2013_05_25_12:55:11_0000.jpg", nil},
	}
	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			fs := vfs.NewTempFs()
			defer fs.Close()
			fs.Create(test.filename)
			jb := &job{
				fs:          fs,
				filename:    test.filename,
				newFilename: path.Base(test.wantNewFilename),
				newDir:      path.Dir(test.wantNewFilename),
			}
			gotErr := jb.Execute()
			if ce, ok := gotErr.(*mediacleaner.ExecuteError); ok {
				gotErr = ce.Cause
			}

			if test.wantErr == gotErr {
				if gotErr == nil {
					if _, err := fs.Stat(path.Dir(test.wantNewFilename)); err != nil {
						t.Errorf("Wanted directory to exist got %v", err)
					}

					if _, err := fs.Stat(test.wantNewFilename); err != nil {
						t.Errorf("Wanted file to have been renamed, got %v", err)
					}

					if _, err := fs.Stat(test.filename); !vfs.IsNotExist(err) {
						t.Errorf("Wanted file to have been renamed, got %v", err)
					}
				}
			} else {
				t.Errorf("Wanted error %v got %v", test.wantErr, gotErr)
			}
		})
	}
}
