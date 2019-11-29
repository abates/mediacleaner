package main

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/abates/mediacleaner"
	"github.com/mh-orange/ffmpeg"
	"github.com/mh-orange/vfs"
	pb "gopkg.in/cheggaaa/pb.v1"
)

var (
	errAlreadyMp4 = errors.New("file is already an mp4 file")
	errNotVideo   = errors.New("file doesn't appear to be a video file")
	errNotRenamed = errors.New("will only transcode files that have been named correctly (/YYYY/MM/YYYY_MM_DD_HH:MM:SS_xxxx.ext)")
)

type job struct {
	fs       vfs.FileSystem
	root     string
	filename string
}

func (jb *job) Name() string {
	return jb.filename
}

func (jb *job) Check() error {
	// only convert files that have already been named correctly
	dir := []byte(path.Dir(jb.filename))
	if mediacleaner.YearMonthDir.Match(dir) || mediacleaner.YearMonthDayDir.Match(dir) {
		fn := []byte(path.Base(jb.filename))
		if !mediacleaner.FilePrefix.Match(fn) {
			return &mediacleaner.CheckError{errNotRenamed}
		}
	} else {
		return &mediacleaner.CheckError{errNotRenamed}
	}

	// convert video files to mp4's that can be pretty much played anywhere
	if path.Ext(jb.filename) == ".mp4" {
		return &mediacleaner.CheckError{errAlreadyMp4}
	}
	if ok, _ := ffmpeg.IsVideo(path.Join(jb.root, jb.filename)); !ok {
		return &mediacleaner.CheckError{errNotVideo}
	}
	return nil
}

func (jb *job) Execute() error {
	input := path.Join(jb.root, jb.filename)
	if !mediacleaner.QuietFlag {
		mediacleaner.Infof("Transcoding %q", jb.filename)
	}
	output := input[0 : len(input)-len(path.Ext(input))]
	output = fmt.Sprintf("%s.mp4", output)
	transcoder := ffmpeg.NewTranscoder()
	proc, err := transcoder.Transcode(ffmpeg.Input(ffmpeg.InputFilename(input)), ffmpeg.Output(ffmpeg.OutputFilename(output), ffmpeg.DefaultMpeg4()))
	if err == nil {
		if !mediacleaner.QuietFlag {
			bar := pb.New(0)
			bar.Output = mediacleaner.Output
			for info := range proc.Progress() {
				if bar.Total == 0 {
					bar.Total = int64(info.Duration)
					bar = bar.Start()
				}
				bar.Set64(int64(info.Time))
			}
			bar.Set64(bar.Total)
			bar.Finish()
		}
		err = proc.Wait()
	}

	if err == nil {
		err = jb.fs.Remove(jb.filename)
		if err != nil {
			err = &mediacleaner.ExecuteError{fmt.Sprintf("failed to remove %q", jb.filename), err}
		}
	} else {
		err = &mediacleaner.ExecuteError{Msg: fmt.Sprintf("failed to transcode %q", jb.filename), Cause: err}
	}
	return err
}

func main() {
	p := mediacleaner.Run(os.Args, func(fs vfs.FileSystem, filename string, root string) mediacleaner.Job {
		return &job{fs: fs, root: root, filename: filename}
	})
	p.Wait()
}
