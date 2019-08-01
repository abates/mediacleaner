package main

import (
	"testing"

	"github.com/abates/mediacleaner"
	"github.com/mh-orange/vfs"
)

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
		{"/noexif.png", "", "", errNoExif},
		{"/nodate.png", "", "", errNoExifDate},
		{"/IMG_20130525_125511_332.jpg", "/2013/05", "2013_05_25_12:55:11_0000", nil},
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
