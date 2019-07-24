package main

import (
	"path"
	"testing"
	"time"

	"github.com/mh-orange/vfs"
)

func TestGetDateFromFilename(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Time
		wantErr error
	}{
		{"2010_01_10_06:57:48_0000.jpg", time.Date(2010, 1, 10, 6, 57, 48, 0, time.UTC), nil},
		{"2010-08-08 14.26.21.jpg", time.Date(2010, 8, 8, 14, 26, 21, 0, time.UTC), nil},
		{"2012-06-25_16-58-20_209.jpg", time.Date(2012, 6, 25, 16, 58, 20, 0, time.UTC), nil},
		{"20160529_102009", time.Date(2016, 5, 29, 10, 20, 9, 0, time.UTC), nil},
		{"IMG_20130525_125511_332", time.Date(2013, 5, 25, 12, 55, 11, 0, time.UTC), nil},
		{"VID_20130525_125511_332", time.Date(2013, 5, 25, 12, 55, 11, 0, time.UTC), nil},
		{"Vfoo", time.Time{}, errUnknownDateFormat},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, gotErr := getDateFromFilename(test.input)
			if test.wantErr == gotErr {
				if gotErr == nil {
					if test.want != got {
						t.Errorf("Wanted %v got %v", test.want, got)
					}
				}
			} else {
				t.Errorf("Wanted error %v got %v", test.wantErr, gotErr)
			}
		})
	}
}

func TestJobGetPrefix(t *testing.T) {
	fs := vfs.NewTempFs()
	defer fs.Close()
	dir := "/my/media/files"
	vfs.MkdirAll(fs, dir, 0755)
	fs.Create(path.Join(dir, "2010_01_10_06:57:48_0000.jpg"))
	fs.Create(path.Join(dir, "2010_01_10_06:58:48_0000.jpg"))
	fs.Create(path.Join(dir, "2010_01_10_06:58:48_0001.jpg"))

	tests := []struct {
		input string
		want  string
	}{
		{"2011_01_10_06:57:48", "2011_01_10_06:57:48_0000"},
		{"2010_01_10_06:57:48", "2010_01_10_06:57:48_0001"},
		{"2010_01_10_06:58:48", "2010_01_10_06:58:48_0002"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			jb := &job{fs: fs}
			got, _ := jb.getPrefix(dir, test.input)
			if got != test.want {
				t.Errorf("Wanted filename %q got %q", test.want, got)
			}
		})
	}
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
		{"/noexif.png", "", "", errNoExif},
		{"/nodate.png", "", "", errNoExifDate},
		{"/IMG_20130525_125511_332.jpg", "/2013/05", "2013_05_25_12:55:11_0000", nil},
	}
	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			jb := &job{fs: fs, root: "testdata/", filename: test.filename}
			gotErr := jb.check()
			if ce, ok := gotErr.(*checkError); ok {
				gotErr = ce.cause
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
