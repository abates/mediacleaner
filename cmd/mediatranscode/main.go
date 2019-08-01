package main

import (
	"errors"
	"fmt"
	"log"
	"path"

	"github.com/abates/mediacleaner"
	"github.com/mh-orange/ffmpeg"
	"github.com/mh-orange/vfs"
	pb "gopkg.in/cheggaaa/pb.v1"
)

var (
	errAlreadyMp4 = errors.New("file is already an mp4 file")
	errNotVideo   = errors.New("file doesn't appear to be a video file")
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
		log.Printf("Transcoding %q", jb.filename)
	}
	output := input[0 : len(input)-len(path.Ext(input))]
	output = fmt.Sprintf("%s.mp4", output)
	transcoder := ffmpeg.NewTranscoder()
	proc, err := transcoder.Transcode(ffmpeg.Input(ffmpeg.InputFilename(input)), ffmpeg.Output(ffmpeg.OutputFilename(output), ffmpeg.DefaultMpeg4()))
	if err == nil {
		if !mediacleaner.QuietFlag {
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

	if err == nil {
		err = mediacleaner.WrapExecuteError(fmt.Sprintf("failed to remove %q", jb.filename), jb.fs.Remove(jb.filename))
	} else {
		err = &mediacleaner.ExecuteError{Msg: fmt.Sprintf("failed to transcode %q", jb.filename), Cause: err}
	}
	return err
}

func main() {
	mediacleaner.Run(func(fs vfs.FileSystem, filename string, root string) mediacleaner.Job {
		return &job{fs: fs, root: root, filename: filename}
	})
}
