package usecase

import (
	"bytes"
	"strings"
)

// maxLineBytes flushes a pathological line (a progress bar without newlines,
// for instance) instead of buffering the whole agent output in memory.
const maxLineBytes = 8 << 10

// lineWriter turns the raw byte stream of an agent into whole lines, so the
// delivery layer receives something it can render.
type lineWriter struct {
	buffer bytes.Buffer
	emit   func(string)
}

func newLineWriter(emit func(string)) *lineWriter {
	return &lineWriter{emit: emit}
}

// Write implements io.Writer.
func (w *lineWriter) Write(chunk []byte) (int, error) {
	written := len(chunk)
	for {
		index := bytes.IndexByte(chunk, '\n')
		if index < 0 {
			break
		}
		w.buffer.Write(chunk[:index])
		w.flush()
		chunk = chunk[index+1:]
	}
	w.buffer.Write(chunk)
	if w.buffer.Len() >= maxLineBytes {
		w.flush()
	}
	return written, nil
}

// Close flushes a trailing line that never ended with a newline.
func (w *lineWriter) Close() error {
	w.flush()
	return nil
}

func (w *lineWriter) flush() {
	if w.buffer.Len() == 0 {
		return
	}
	line := strings.TrimRight(w.buffer.String(), "\r")
	w.buffer.Reset()
	if strings.TrimSpace(line) != "" {
		w.emit(line)
	}
}
