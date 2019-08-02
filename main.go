package mediacleaner

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/mh-orange/vfs"
)

var (
	SkipName   = regexp.MustCompile(`^\/\d{4}\/\d{2}\/\d{4}_\d{2}_\d{2}_\d{2}:\d{2}:\d{2}`)
	DirPrefix  = regexp.MustCompile(`^\/\d{4}\/\d{2}`)
	FilePrefix = regexp.MustCompile(`^\d{4}_\d{2}_\d{2}_\d{2}:\d{2}:\d{2}`)

	filePrefixes = map[*regexp.Regexp]string{
		regexp.MustCompile(`^\d{4}_\d{2}_\d{2}_\d{2}:\d{2}:\d{2}`):     "2006_01_02_15:04:05",
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}`):     "2006-01-02_15-04-05",
		regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\s+\d{2}\.\d{2}\.\d{2}`): "2006-01-02 15.04.05",
		regexp.MustCompile(`^\d{8}_\d{6}`):                             "20060102_150405",
		regexp.MustCompile(`^IMG_\d{8}_\d{6}`):                         "IMG_20060102_150405",
		regexp.MustCompile(`^VID_\d{8}_\d{6}`):                         "VID_20060102_150405",
	}

	ScanFlag  bool
	WatchFlag bool
	QuietFlag bool

	ErrUnknownDateFormat = errors.New("Unknown date format")

	Output = io.Writer(os.Stderr)
	Logger *log.Logger
)

type CheckError struct {
	Cause error
}

func (err *CheckError) Error() string {
	return err.Cause.Error()
}

type ExecuteError struct {
	Msg   string
	Cause error
}

func (err *ExecuteError) Error() string {
	return fmt.Sprintf("%s: %v", err.Msg, err.Cause)
}

type Job interface {
	Name() string
	Check() error
	Execute() error
}

type FileCallback func(fs vfs.FileSystem, filename string, root string) Job

func GetDateFromFilename(filename string) (t time.Time, err error) {
	match := []byte(path.Base(filename))
	for exp, layout := range filePrefixes {
		if str := exp.Find(match); str != nil {
			t, _ = time.Parse(layout, string(str))
			return
		}
	}
	err = ErrUnknownDateFormat
	return
}

func GetPrefix(fs vfs.FileSystem, dirname, prefix string) (string, error) {
	entries, err := vfs.Glob(fs, fmt.Sprintf("%s/%s_*.*", dirname, prefix))
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

func WrapExecuteError(msg string, err error) error {
	if err != nil {
		err = &ExecuteError{Msg: msg, Cause: err}
	}
	return err
}

func skip(info os.FileInfo, filename string) bool {
	if info.IsDir() {
		return true
	}

	if SkipName.Match([]byte(filename)) {
		return true
	}
	return false
}

func walk(fs vfs.FileSystem, root string, queue chan<- Job, cb FileCallback) vfs.WalkFunc {
	return func(filename string, info os.FileInfo, err error) error {
		if err != nil || skip(info, filename) {
			return err
		}

		job := cb(fs, filename, root)
		if job != nil {
			queue <- job
		}
		return err
	}
}

func watch(fs vfs.FileSystem, root string, events <-chan vfs.Event, jobQueue chan<- Job, cb FileCallback) {
	walkFn := walk(fs, root, jobQueue, cb)
	for event := range events {
		if event.Type&vfs.CreateEvent == vfs.CreateEvent {
			info, err := fs.Stat(event.Path)
			err = walkFn(event.Path, info, err)
			if err != nil {
				Logger.Printf("error when trying to stat newly created file %q: %v", event.Path, err)
			}
		}
	}
}

func (p *Process) process(queue <-chan Job) {
	errChs := []chan error{}
	watchers := []vfs.Watcher{}
	done := false
	for !done {
		select {
		case job, open := <-queue:
			if !open {
				done = true
				continue
			}
			err := job.Check()
			if err == nil {
				err = job.Execute()
				if err != nil {
					Logger.Printf("Failed to process %s: %v", job.Name(), err)
				}
			} else if _, ok := err.(*CheckError); !ok {
				Logger.Printf("Failed to perform checks on %s: %v", job.Name(), err)
			}
		case errCh := <-p.killCh:
			errChs = append(errChs, errCh)
			for _, watcher := range watchers {
				err := watcher.Close()
				if err != nil {
					Logger.Printf("Failed to close logger: %v", err)
				}
			}
			watchers = nil
		case watcher := <-p.watcherCh:
			watchers = append(watchers, watcher)
		}
	}

	for _, errCh := range errChs {
		errCh <- nil
	}
}

// Kill a currently running watch process, this terminates any
// filesystem watchers that are set up
func (p *Process) Kill() (err error) {
	wait := make(chan error)
	p.killCh <- wait
	return <-wait
}

// Wait for the process to complete
func (p *Process) Wait() {
	p.pwg.Wait()
}

type Process struct {
	killCh    chan chan error
	watcherCh chan vfs.Watcher
	pwg       sync.WaitGroup
	wg        sync.WaitGroup
}

func init() {
	Logger = log.New(Output, "", log.LstdFlags)
}

func Run(args []string, cb FileCallback) *Process {
	name := args[0]
	flags := flag.NewFlagSet(name, flag.ExitOnError)
	flags.BoolVar(&QuietFlag, "q", false, "quiet - hide the progress bar")
	flags.BoolVar(&ScanFlag, "s", false, "scan - scan directories and process the files")
	flags.BoolVar(&WatchFlag, "w", false, "watch - watch for changes to the filesystem and process newly created files")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [options] <dir1> <dir2> ...\n\nOptions:\n", name)
		flags.PrintDefaults()
	}

	flags.Parse(args[1:])
	args = flags.Args()
	if len(args) < 1 {
		flags.Usage()
		os.Exit(1)
	}

	p := &Process{
		killCh:    make(chan chan error),
		watcherCh: make(chan vfs.Watcher),
	}
	queue := make(chan Job)
	events := make(chan vfs.Event, 16384)

	p.pwg.Add(1)
	Logger.Printf("Starting processing thread")
	go func() {
		p.process(queue)
		p.pwg.Done()
	}()

	for _, path := range args {
		fs := vfs.NewOsFs(path)
		if ScanFlag {
			Logger.Printf("Scanning %q", path)
			p.wg.Add(1)
			go func(fs vfs.FileSystem) {
				vfs.Walk(fs, "/", walk(fs, path, queue, cb))
				p.wg.Done()
			}(fs)
		}

		if WatchFlag {
			p.wg.Add(1)
			watcher, err := vfs.Watch(fs, "/", events)
			if err == nil {
				p.watcherCh <- watcher
				go func(fs vfs.FileSystem) {
					Logger.Printf("Watching %q", path)
					watch(fs, path, events, queue, cb)
					p.wg.Done()
				}(fs)
			} else {
				Logger.Printf("Failed to start watch: %v", err)
			}
		}
	}

	go func() {
		p.wg.Wait()
		close(queue)
	}()

	return p
}
