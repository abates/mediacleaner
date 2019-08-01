package mediacleaner

import (
	"errors"
	"log"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mh-orange/vfs"
)

func TestErrors(t *testing.T) {
	tests := []struct {
		name  string
		input error
		want  string
	}{
		{"check error", &CheckError{errors.New("test test test")}, "test test test"},
		{"execute error", &ExecuteError{Msg: "It Failed!", Cause: errors.New("Oh woe is me")}, "It Failed!: Oh woe is me"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.input.Error()
			if test.want != got {
				t.Errorf("Wanted error message %v got %v", test.want, got)
			}
		})
	}
}

func TestWrapExecuteError(t *testing.T) {
	tests := []struct {
		name     string
		inputMsg string
		inputErr error
		want     *ExecuteError
	}{
		{"nil error", "foo", nil, nil},
		{"execute error", "foo", ErrUnknownDateFormat, &ExecuteError{Msg: "foo", Cause: ErrUnknownDateFormat}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := WrapExecuteError(test.inputMsg, test.inputErr)
			if got, ok := err.(*ExecuteError); ok {
				if *test.want != *got {
					t.Errorf("Wanted error %v got %v", test.want, got)
				}
			} else if err != nil {
				t.Errorf("Wanted *ExecuteError got %T", err)
			}
		})
	}
}

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
		{"Vfoo", time.Time{}, ErrUnknownDateFormat},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, gotErr := GetDateFromFilename(test.input)
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

func TestGetPrefix(t *testing.T) {
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
			got, _ := GetPrefix(fs, dir, test.input)
			if got != test.want {
				t.Errorf("Wanted filename %q got %q", test.want, got)
			}
		})
	}
}

func TestSkip(t *testing.T) {
	fs := vfs.NewTempFs()
	defer fs.Close()
	vfs.MkdirAll(fs, "/2010/01", 0755)
	fs.Create("/2010/01/2010_01_10_06:57:48_0000.jpg")
	fs.Create("foo.jpg")

	tests := []struct {
		input string
		want  bool
	}{
		{"/", true},
		{"/2010/01/2010_01_10_06:57:48_0000.jpg", true},
		{"foo.jpg", false},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			info, _ := fs.Stat(test.input)
			got := skip(info, test.input)
			if test.want != got {
				t.Errorf("Wanted check to return %v got %v", test.want, got)
			}
		})
	}
}

type testFileInfo struct {
	name     string
	fileMode os.FileMode
}

func (tfi *testFileInfo) Name() string       { return tfi.name }
func (tfi *testFileInfo) Size() int64        { return 0 }
func (tfi *testFileInfo) Mode() os.FileMode  { return tfi.fileMode }
func (tfi *testFileInfo) ModTime() time.Time { return time.Time{} }
func (tfi *testFileInfo) IsDir() bool        { return tfi.fileMode.IsDir() }
func (tfi *testFileInfo) Sys() interface{}   { return nil }

type testJob struct {
	name       string
	check      bool
	checkErr   error
	execute    bool
	executeErr error
}

func (tj *testJob) Name() string { return tj.name }
func (tj *testJob) Check() error {
	tj.check = true
	return tj.checkErr
}

func (tj *testJob) Execute() error {
	tj.execute = true
	return tj.executeErr
}

func TestWalk(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		fileInfo     os.FileInfo
		job          Job
		err          error
		wantQueueLen int
		want         []string
	}{
		{"skip, no error", "/", &testFileInfo{fileMode: os.ModeDir}, nil, nil, 0, []string{}},
		{"no job", "/", &testFileInfo{}, nil, nil, 0, []string{"/"}},
		{"one job", "/", &testFileInfo{}, &testJob{name: "no job"}, nil, 1, []string{"/"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			queue := make(chan Job, 1)
			got := []string{}
			fs := vfs.NewMemFs()
			defer fs.Close()
			walkFn := walk(fs, "", queue, func(fs vfs.FileSystem, filename string, root string) Job {
				got = append(got, filename)
				return test.job
			})
			walkFn(test.filename, test.fileInfo, test.err)
			if !reflect.DeepEqual(test.want, got) {
				t.Errorf("Want %s got %s", test.want, got)
			}

			if len(queue) != test.wantQueueLen {
				t.Errorf("Wanted %d items in the queue, got %d", test.wantQueueLen, len(queue))
			}
		})
	}
}

func TestWatch(t *testing.T) {
	tests := []struct {
		name         string
		event        vfs.Event
		job          Job
		wantQueueLen int
		want         []string
	}{
		{"stat error", vfs.Event{Path: "/foo", Type: vfs.CreateEvent}, nil, 0, []string{}},
		{"skip directory", vfs.Event{Path: "/", Type: vfs.CreateEvent}, nil, 0, []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jobQueue := make(chan Job, 1)
			events := make(chan vfs.Event, 1)
			events <- test.event
			close(events)
			got := []string{}
			fs := vfs.NewMemFs()
			defer fs.Close()
			watch(fs, "", events, jobQueue, func(fs vfs.FileSystem, filename string, root string) Job {
				got = append(got, filename)
				return test.job
			})
			if !reflect.DeepEqual(test.want, got) {
				t.Errorf("Want %s got %s", test.want, got)
			}

			if len(jobQueue) != test.wantQueueLen {
				t.Errorf("Wanted %d items in the job queue, got %d", test.wantQueueLen, len(jobQueue))
			}
		})
	}
}

func TestProcess(t *testing.T) {
	tests := []struct {
		name             string
		scanJob          *testJob
		watchJob         *testJob
		wantScanExecute  bool
		wantWatchExecute bool
		wantLogMsg       string
	}{
		{"scan bad check", &testJob{name: "foo", checkErr: ErrUnknownDateFormat}, nil, false, false, "Failed to perform checks on foo: Unknown date format\n"},
		{"execute error", &testJob{name: "foo", executeErr: ErrUnknownDateFormat}, nil, true, false, "Failed to process foo: Unknown date format\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder := &strings.Builder{}
			Logger = log.New(builder, "", 0)
			fs := vfs.NewMemFs()
			defer fs.Close()
			scanQueue := make(chan Job, 1)
			watchQueue := make(chan Job, 1)
			if test.scanJob != nil {
				scanQueue <- test.scanJob
			}
			if test.watchJob != nil {
				watchQueue <- test.watchJob
			}
			close(scanQueue)
			close(watchQueue)
			process(scanQueue, watchQueue)
			if test.scanJob != nil && test.wantScanExecute != test.scanJob.execute {
				t.Errorf("Wanted scan execute to be %v got %v", test.wantScanExecute, test.scanJob.execute)
			}
			if test.watchJob != nil && test.wantWatchExecute != test.watchJob.execute {
				t.Errorf("Wanted watch execute to be %v got %v", test.wantWatchExecute, test.watchJob.execute)
			}
			gotLogMsg := builder.String()
			if test.wantLogMsg != gotLogMsg {
				t.Errorf("Wanted log message %q got %q", test.wantLogMsg, gotLogMsg)
			}
		})
	}
}
