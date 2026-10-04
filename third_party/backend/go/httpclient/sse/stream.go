package sse

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const maxEventBytes = 16 << 20

// Event is one Server-Sent Events message.
type Event struct {
	ID    string
	Name  string
	Data  []byte
	Retry time.Duration
}

// Stream reads events from an open SSE response.
// Recv must not be called concurrently.
type Stream struct {
	header      http.Header
	body        io.ReadCloser
	scanner     *bufio.Scanner
	lastEventID string
	closeOnce   sync.Once
	closeErr    error
}

func newStream(header http.Header, body io.ReadCloser) *Stream {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64<<10), maxEventBytes)
	scanner.Split(splitLines)
	return &Stream{
		header:  header.Clone(),
		body:    body,
		scanner: scanner,
	}
}

func (s *Stream) Header() http.Header {
	return s.header.Clone()
}

func (s *Stream) Recv() (Event, error) {
	var data bytes.Buffer
	var hasData bool
	var name string
	var retry time.Duration
	eventID := s.lastEventID

	for s.scanner.Scan() {
		line := s.scanner.Bytes()
		if len(line) == 0 {
			if !hasData {
				name = ""
				retry = 0
				eventID = s.lastEventID
				continue
			}
			return Event{
				ID:    eventID,
				Name:  name,
				Data:  append([]byte(nil), data.Bytes()...),
				Retry: retry,
			}, nil
		}
		if line[0] == ':' {
			continue
		}

		field, value, found := bytes.Cut(line, []byte{':'})
		if found && len(value) != 0 && value[0] == ' ' {
			value = value[1:]
		}
		switch string(field) {
		case "data":
			size := data.Len() + len(value)
			if hasData {
				size++
			}
			if size > maxEventBytes {
				return Event{}, fmt.Errorf("SSE event exceeds %d bytes", maxEventBytes)
			}
			if hasData {
				data.WriteByte('\n')
			}
			data.Write(value)
			hasData = true
		case "event":
			name = string(value)
		case "id":
			if !bytes.ContainsRune(value, '\x00') {
				s.lastEventID = string(value)
				eventID = s.lastEventID
			}
		case "retry":
			milliseconds, err := strconv.ParseInt(string(value), 10, 64)
			if err == nil && milliseconds >= 0 && milliseconds <= math.MaxInt64/int64(time.Millisecond) {
				retry = time.Duration(milliseconds) * time.Millisecond
			}
		}
	}

	if err := s.scanner.Err(); err != nil {
		return Event{}, fmt.Errorf("read SSE stream: %w", err)
	}
	if hasData {
		return Event{
			ID:    eventID,
			Name:  name,
			Data:  append([]byte(nil), data.Bytes()...),
			Retry: retry,
		}, nil
	}
	return Event{}, io.EOF
}

func (s *Stream) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.body.Close()
	})
	return s.closeErr
}

func splitLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		switch b {
		case '\n':
			return i + 1, data[:i], nil
		case '\r':
			if i+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			if i+1 < len(data) && data[i+1] == '\n' {
				return i + 2, data[:i], nil
			}
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) != 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
