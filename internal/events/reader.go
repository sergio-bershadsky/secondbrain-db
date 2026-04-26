package events

import (
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadFile yields all events from a single .jsonl file. Caller closes nothing.
func ReadFile(path string) ([]*Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readAll(f)
}

// ReadGzipFile yields all events from a .jsonl.gz archive.
func ReadGzipFile(path string) ([]*Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	return readAll(gz)
}

func readAll(r io.Reader) ([]*Event, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), MaxLineBytes+128)
	var out []*Event
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		e, err := ParseLine(line)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// IterateLive walks all live (non-archived) daily files under root in lex
// order and calls fn for each event. Stops on first fn error.
func IterateLive(root string, fn func(filename string, e *Event) error) error {
	dir := filepath.Join(root, EventsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names := make([]string, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".jsonl") {
			continue
		}
		if _, _, _, _, ok := ParseDailyName(ent.Name()); ok {
			names = append(names, ent.Name())
		}
	}
	names = SortedDailyFiles(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		events, err := ReadFile(path)
		if err != nil {
			return err
		}
		for _, e := range events {
			if err := fn(name, e); err != nil {
				return err
			}
		}
	}
	return nil
}

// CountLines returns the number of newline-terminated lines in path.
// Used by archive verification (gz line count must match input).
func CountLines(path string) (int, error) {
	return countLinesIn(path)
}

// CountGzLines returns the line count of a gzipped jsonl file.
func CountGzLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer gz.Close()
	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 64*1024), MaxLineBytes+128)
	n := 0
	for scanner.Scan() {
		n++
	}
	return n, scanner.Err()
}
