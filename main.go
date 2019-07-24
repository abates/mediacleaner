package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/abates/goexiftool"
	"github.com/mh-orange/ffmpeg"
	"github.com/mh-orange/vfs"
	pb "gopkg.in/cheggaaa/pb.v1"
)

var (
	skipName  = regexp.MustCompile(`^\/\d{4}\/\d{2}\/\d{4}_\d{2}_\d{2}_\d{2}:\d{2}:\d{2}`)
	dirPrefix = regexp.MustCompile(`^\/\d{4}\/\d{2}`)

	filePrefix1 = regexp.MustCompile(`^\d{4}_\d{2}_\d{2}_\d{2}:\d{2}:\d{2}`)
	filePrefix2 = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}`)
	filePrefix3 = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\s+\d{2}\.\d{2}\.\d{2}`)
	filePrefix4 = regexp.MustCompile(`^\d{8}_\d{6}`)
	filePrefix5 = regexp.MustCompile(`^IMG_\d{8}_\d{6}`)
	filePrefix6 = regexp.MustCompile(`^VID_\d{8}_\d{6}`)

	scanFlag  bool
	watchFlag bool
	quietFlag bool

	errUnknownDateFormat = errors.New("Unknown date format")
	errNoFile            = errors.New("File removed prior to processing")
	errIsDir             = errors.New("File is a directory")
	errAlreadyProcessed  = errors.New("File is already in the final/processed directory")
	errNoExif            = errors.New("File has no exif data")
	errNoExifDate        = errors.New("Exif data has no known date")
)

type checkError struct {
	cause error
}

func (err *checkError) Error() string {
	return err.cause.Error()
}

type executeError struct {
	msg   string
	cause error
}

func (err *executeError) Error() string {
	return fmt.Sprintf("%s: %v", err.msg, err.cause)
}

type job struct {
	fs          vfs.FileSystem
	root        string
	filename    string
	newFilename string
	newDir      string
	done        chan job
}

type normalizer struct {
	queue chan<- job
}

func getDateFromFilename(filename string) (t time.Time, err error) {
	if str := filePrefix1.Find([]byte(path.Base(filename))); str != nil {
		t, _ = time.Parse("2006_01_02_15:04:05", string(str))
	} else if str := filePrefix2.Find([]byte(path.Base(filename))); str != nil {
		t, _ = time.Parse("2006-01-02_15-04-05", string(str))
	} else if str := filePrefix3.Find([]byte(path.Base(filename))); str != nil {
		t, _ = time.Parse("2006-01-02 15.04.05", string(str))
	} else if str := filePrefix4.Find([]byte(path.Base(filename))); str != nil {
		t, _ = time.Parse("20060102_150405", string(str))
	} else if str := filePrefix5.Find([]byte(path.Base(filename))); str != nil {
		t, _ = time.Parse("IMG_20060102_150405", string(str))
	} else if str := filePrefix6.Find([]byte(path.Base(filename))); str != nil {
		t, _ = time.Parse("VID_20060102_150405", string(str))
	} else {
		err = errUnknownDateFormat
	}
	return
}

func (jb *job) getPrefix(dirname, prefix string) (string, error) {
	entries, err := vfs.Glob(jb.fs, fmt.Sprintf("%s/%s_*.*", dirname, prefix))
	if len(entries) > 0 {
		sort.Strings(entries)
		num := 0
		entry := path.Base(entries[len(entries)-1])
		entry = entry[0 : len(entry)-len(path.Ext(entry))]
		fmt.Sscanf(entry, fmt.Sprintf("%s_%%d", prefix), &num)
		prefix = fmt.Sprintf("%s_%04d", prefix, num+1)
	} else {
		prefix = fmt.Sprintf("%s_0000", prefix)
	}
	return prefix, err
}

func (jb *job) transcode(filename string) error {
	log.Printf("Transcoding %q", filename)
	output := filename[0 : len(filename)-len(path.Ext(filename))]
	output = fmt.Sprintf("%s.mp4", output)
	transcoder := ffmpeg.NewTranscoder()
	proc, err := transcoder.Transcode(ffmpeg.Input(ffmpeg.InputFilename(filename)), ffmpeg.Output(ffmpeg.OutputFilename(output), ffmpeg.DefaultMpeg4()))
	if err == nil {
		if !quietFlag {
			bar := pb.New(0)
			for info := range proc.Progress() {
				if bar.Total == 0 {
					bar.Total = int64(info.Duration)
					bar = bar.Start()
				}
				bar.Set64(int64(info.Time))
			}
			bar.Finish()
		}
		err = proc.Wait()
	}
	return err
}

func (jb *job) check() error {
	if fi, err := jb.fs.Stat(jb.filename); vfs.IsNotExist(err) {
		return &checkError{errNoFile}
	} else if fi.IsDir() {
		return &checkError{errIsDir}
	}

	if skipName.Match([]byte(jb.filename)) {
		return &checkError{errAlreadyProcessed}
	}

	t, err := getDateFromFilename(jb.filename)
	if err != nil {
		exif, err := goexiftool.NewMediaFile(path.Join(jb.root, jb.filename))
		if err != nil {
			return &checkError{errNoExif}
		}

		t, err = exif.GetDate()
		if err != nil {
			return &checkError{errNoExifDate}
		}
	}
	jb.newFilename = t.Format("2006_01_02_15:04:05")
	jb.newDir = t.Format("/2006/01")

	jb.newFilename, err = jb.getPrefix(jb.newDir, jb.newFilename)
	return err
}

func wrapError(msg string, err error) error {
	if err != nil {
		err = &executeError{msg: msg, cause: err}
	}
	return err
}

func (jb *job) execute() error {
	err := wrapError(fmt.Sprintf("failed creating directory %q", jb.newDir), vfs.MkdirAll(jb.fs, jb.newDir, 0750))
	if err == nil {
		newFilename := path.Join(jb.newDir, fmt.Sprintf("%s%s", jb.newFilename, strings.ToLower(path.Ext(jb.filename))))
		err = wrapError(fmt.Sprintf("failed to rename %q to %q", jb.filename, newFilename), jb.fs.Rename(jb.filename, newFilename))
		if err == nil {
			jb.filename = newFilename

			// convert video files to mp4's that can be pretty much played anywhere
			if path.Ext(jb.filename) != ".mp4" {
				if ok, _ := ffmpeg.IsVideo(path.Join(jb.root, jb.filename)); ok {
					err = wrapError("failed to transcode", jb.transcode(path.Join(jb.root, jb.filename)))
					if err == nil {
						err = wrapError(fmt.Sprintf("failed to remove %q", jb.filename), jb.fs.Remove(jb.filename))
					}
				}
			}
		}
	}
	return err
}

func walk(fs vfs.FileSystem, root string, queue chan<- job) {
	done := make(chan job, 1)
	vfs.Walk(fs, "/", func(filename string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			queue <- job{fs: fs, root: root, filename: filename, done: done}
			<-done
		}
		return err
	})
}

func watch(fs vfs.FileSystem, root string, queue chan<- job) {
	events := make(chan vfs.Event, 16384)
	vfs.Watch(fs, "/", events)
	for event := range events {
		if event.Type&vfs.CreateEvent == vfs.CreateEvent {
			queue <- job{fs: fs, root: root, filename: event.Path}
		}
	}
}

func process(queue <-chan job) {
	for job := range queue {
		err := job.check()
		if err == nil {
			err = job.execute()
			if err != nil {
				log.Printf("Failed to process %q: %v", job.filename, err)
			}
		} else if _, ok := err.(*checkError); !ok {
			log.Printf("Failed to perform checks on %q: %v", job.filename, err)
		}

		if job.done != nil {
			job.done <- job
		}
	}
}

func init() {
	flag.BoolVar(&quietFlag, "q", false, "quiet - hide the progress bar")
	flag.BoolVar(&scanFlag, "s", false, "scan - scan directories and process the files")
	flag.BoolVar(&watchFlag, "w", false, "watch - watch for changes to the filesystem and process newly created files")
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] <dir1> <dir2> ...\n\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	queue := make(chan job, 4096)

	for _, path := range args {
		fs := vfs.NewOsFs(path)
		if scanFlag {
			log.Printf("Scanning %q", path)
			go walk(fs, path, queue)
		}

		if watchFlag {
			log.Printf("Watching %q", path)
			go watch(fs, path, queue)
		}
	}

	log.Printf("Starting processing thread")
	process(queue)
}
