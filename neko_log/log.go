package neko_log

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/matsuridayo/libneko/neko_common"
	"github.com/matsuridayo/libneko/syscallw"
)

var LogWriter *logWriter
var LogWriterDisable = false
var TruncateOnStart = true
var NB4AGuiLogWriter io.Writer

func SetupLog(maxSize int, path string) (err error) {
	if LogWriter != nil {
		return
	}

	var f *os.File
	f, err = os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0644)
	if err == nil {
		fd := int(f.Fd())
		if TruncateOnStart {
			syscallw.Flock(fd, syscallw.LOCK_EX)
			// Check if need truncate
			if size, _ := f.Seek(0, io.SeekEnd); size > int64(maxSize) {
				// read oldBytes for maxSize
				f.Seek(-int64(maxSize), io.SeekCurrent)
				oldBytes, err := io.ReadAll(f)
				if err == nil {
					// truncate file
					if runtime.GOOS == "windows" {
						f.Close()
						os.Remove(path)
						// reopen file
						f, err = os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0644)
					} else {
						err = f.Truncate(0)
					}
					// write oldBytes
					if err == nil {
						f.Write(oldBytes)
					}
				}
			}
			syscallw.Flock(fd, syscallw.LOCK_UN)
		}
		if neko_common.RunMode == neko_common.RunMode_NekoBoxForAndroid {
			// redirect stderr
			syscallw.Dup3(fd, int(os.Stderr.Fd()), 0)
		}
	}

	if err != nil {
		err = fmt.Errorf("error open log: %v", err)
		log.Println(err)
	}

	//
	LogWriter = &logWriter{}
	if neko_common.RunMode == neko_common.RunMode_NekoBoxForAndroid {
		LogWriter.writers = []io.Writer{NB4AGuiLogWriter, f}
	} else {
		LogWriter.writers = []io.Writer{os.Stdout, f}
	}
	// setup std log — format HI-style: [2026-10-12 08:35:28]
	log.SetFlags(0)
	log.SetOutput(&timestampWriter{inner: LogWriter})

	return
}

type logWriter struct {
	writers []io.Writer
}

func (w *logWriter) Write(p []byte) (int, error) {
	if LogWriterDisable {
		return len(p), nil
	}

	for _, w := range w.writers {
		if w == nil {
			continue
		}
		if f, ok := w.(*os.File); ok {
			fd := int(f.Fd())
			syscallw.Flock(fd, syscallw.LOCK_EX)
			f.Write(p)
			syscallw.Flock(fd, syscallw.LOCK_UN)
		} else {
			w.Write(p)
		}
	}

	return len(p), nil
}

func (w *logWriter) Truncate() {
	for _, w := range w.writers {
		if w == nil {
			continue
		}
		if f, ok := w.(*os.File); ok {
			_ = f.Truncate(0)
		}
	}
}

func (w *logWriter) Close() error {
	for _, w := range w.writers {
		if w == nil {
			continue
		}
		if f, ok := w.(*os.File); ok {
			_ = f.Close()
		}
	}
	return nil
}

// timestampWriter: prefix setiap baris log dengan [YYYY-MM-DD HH:MM:SS] (gaya HTTP Injector)
type timestampWriter struct {
	inner io.Writer
}

func (w *timestampWriter) Write(p []byte) (int, error) {
	ts := time.Now().Format("[2006-01-02 15:04:05] ")
	var out []byte
	for _, line := range bytes.Split(p, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		out = append(out, []byte(ts)...)
		out = append(out, line...)
		out = append(out, '\n')
	}
	if len(out) == 0 {
		return len(p), nil
	}
	_, err := w.inner.Write(out)
	return len(p), err
}
