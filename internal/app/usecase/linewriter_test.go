package usecase

import (
	"strings"
	"testing"
)

func collect() (*lineWriter, *[]string) {
	lines := &[]string{}
	writer := newLineWriter(func(line string) { *lines = append(*lines, line) })
	return writer, lines
}

func TestLineWriterEmitsWholeLines(t *testing.T) {
	t.Parallel()

	writer, lines := collect()
	writer.Write([]byte("first\nsec"))
	writer.Write([]byte("ond\r\nthird"))
	writer.Close()

	want := []string{"first", "second", "third"}
	if len(*lines) != len(want) {
		t.Fatalf("want %v, got %v", want, *lines)
	}
	for index := range want {
		if (*lines)[index] != want[index] {
			t.Fatalf("line %d: want %q, got %q", index, want[index], (*lines)[index])
		}
	}
}

func TestLineWriterSkipsBlankLines(t *testing.T) {
	t.Parallel()

	writer, lines := collect()
	writer.Write([]byte("\n   \n\nreal\n"))
	writer.Close()

	if len(*lines) != 1 || (*lines)[0] != "real" {
		t.Fatalf("want only the real line, got %v", *lines)
	}
}

func TestLineWriterFlushesPathologicalLines(t *testing.T) {
	t.Parallel()

	writer, lines := collect()
	writer.Write([]byte(strings.Repeat("x", maxLineBytes+10)))

	if len(*lines) == 0 {
		t.Fatal("a line without a newline must be flushed instead of buffered forever")
	}
}
