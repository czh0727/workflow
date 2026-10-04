package obs

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

type boundedFormatter struct {
	inner        logrus.Formatter
	maxLineBytes int
}

var preservedLogKeys = []string{
	"service",
	"module",
	"trace_id",
	"span_id",
	"trace_flags",
	"lane",
	"tenant_id",
	"user_id",
}

func (f boundedFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	maxLineBytes := normalizeLogMaxLineBytes(f.maxLineBytes)
	rendered, err := f.inner.Format(entry)
	if err != nil {
		return rendered, err
	}
	line, newline := trimOneTrailingNewline(rendered)
	originalBytes := len(line)
	if originalBytes <= maxLineBytes {
		return rendered, nil
	}
	for _, stringBudget := range []int{512, 256, 128, 64, 32, 16, 8, 0} {
		candidate := boundedEntry(entry, stringBudget, originalBytes, maxLineBytes)
		next, err := f.inner.Format(candidate)
		if err != nil {
			return next, err
		}
		nextLine, nextNewline := trimOneTrailingNewline(next)
		if len(nextLine) <= maxLineBytes {
			return restoreTrailingNewline(nextLine, newline || nextNewline), nil
		}
	}
	minimal := minimalBoundedEntry(entry, originalBytes, maxLineBytes)
	next, err := f.inner.Format(minimal)
	if err != nil {
		return next, err
	}
	nextLine, nextNewline := trimOneTrailingNewline(next)
	return restoreTrailingNewline(nextLine, newline || nextNewline), nil
}

func boundedEntry(entry *logrus.Entry, stringBudget int, originalBytes int, maxLineBytes int) *logrus.Entry {
	data := truncateFields(entry.Data, stringBudget)
	data["log_truncated"] = true
	data["log_original_bytes"] = originalBytes
	data["log_max_bytes"] = maxLineBytes
	return cloneEntry(entry, truncateString(entry.Message, stringBudget), data)
}

func minimalBoundedEntry(entry *logrus.Entry, originalBytes int, maxLineBytes int) *logrus.Entry {
	data := logrus.Fields{
		"log_truncated":      true,
		"log_original_bytes": originalBytes,
		"log_max_bytes":      maxLineBytes,
	}
	for _, key := range preservedLogKeys {
		if value, ok := entry.Data[key]; ok {
			data[key] = truncateValue(value, 16)
		}
	}
	return cloneEntry(entry, truncateString(entry.Message, 16), data)
}

func cloneEntry(entry *logrus.Entry, message string, data logrus.Fields) *logrus.Entry {
	return &logrus.Entry{
		Logger:  entry.Logger,
		Data:    data,
		Time:    entry.Time,
		Level:   entry.Level,
		Caller:  entry.Caller,
		Message: message,
		Context: entry.Context,
	}
}

func truncateFields(fields logrus.Fields, stringBudget int) logrus.Fields {
	out := make(logrus.Fields, len(fields))
	for _, key := range preservedLogKeys {
		if value, ok := fields[key]; ok {
			out[key] = value
		}
	}
	for key, value := range fields {
		if _, ok := out[key]; ok {
			continue
		}
		out[key] = truncateValue(value, stringBudget)
	}
	return out
}

func truncateValue(value any, stringBudget int) any {
	switch v := value.(type) {
	case string:
		return truncateString(v, stringBudget)
	case []byte:
		return truncateString(string(v), stringBudget)
	case fmt.Stringer:
		return truncateString(v.String(), stringBudget)
	case error:
		return truncateString(v.Error(), stringBudget)
	case map[string]any:
		return truncateAnyMap(v, stringBudget)
	case map[string]string:
		out := make(map[string]string, len(v))
		for key, child := range v {
			out[key] = truncateString(child, stringBudget)
		}
		return out
	case []any:
		return truncateAnySlice(v, stringBudget)
	case []string:
		limit := min(len(v), 10)
		out := make([]string, 0, limit)
		for _, child := range v[:limit] {
			out = append(out, truncateString(child, stringBudget))
		}
		return out
	default:
		return value
	}
}

func truncateAnyMap(values map[string]any, stringBudget int) map[string]any {
	out := make(map[string]any, len(values))
	for key, child := range values {
		out[key] = truncateValue(child, stringBudget)
	}
	return out
}

func truncateAnySlice(values []any, stringBudget int) []any {
	limit := min(len(values), 10)
	out := make([]any, 0, limit)
	for _, child := range values[:limit] {
		out = append(out, truncateValue(child, stringBudget))
	}
	return out
}

func truncateString(value string, stringBudget int) string {
	if stringBudget <= 0 {
		return fmt.Sprintf("<truncated len=%d>", len([]rune(value)))
	}
	if len([]rune(value)) <= stringBudget {
		return value
	}
	suffix := fmt.Sprintf("...<truncated len=%d>", len([]rune(value)))
	suffixRunes := []rune(suffix)
	if stringBudget <= len(suffixRunes) {
		return string(suffixRunes[:stringBudget])
	}
	runes := []rune(value)
	return string(runes[:stringBudget-len(suffixRunes)]) + suffix
}

func trimOneTrailingNewline(line []byte) ([]byte, bool) {
	if bytes.HasSuffix(line, []byte("\n")) {
		return line[:len(line)-1], true
	}
	return line, false
}

func restoreTrailingNewline(line []byte, newline bool) []byte {
	if !newline {
		return line
	}
	if bytes.HasSuffix(line, []byte("\n")) {
		return line
	}
	return []byte(strings.TrimRight(string(line), "\n") + "\n")
}

func envIntOr(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func normalizeLogMaxLineBytes(value int) int {
	if value <= 0 {
		return defaultLogMaxLineBytes
	}
	if value < minLogMaxLineBytes {
		return minLogMaxLineBytes
	}
	return value
}
