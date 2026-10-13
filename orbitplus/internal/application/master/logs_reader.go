package master

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Sentinel errors returned by LogsReadService. The HTTP layer maps these to
// status codes, mirroring the cache viewer error contract.
var (
	// ErrLogsNotConfigured means no log directory is available to read.
	ErrLogsNotConfigured = errors.New("logs directory is not configured")
	// ErrInvalidLogFile means the requested file name is empty, contains a
	// path separator, or otherwise escapes the log directory.
	ErrInvalidLogFile = errors.New("log file name is invalid")
	// ErrLogFileNotFound means the requested log file does not exist.
	ErrLogFileNotFound = errors.New("log file not found")
)

// maxLogContentBytes bounds how much of a log file is returned to the browser.
// Only the final chunk is returned, since recent lines matter most.
const maxLogContentBytes = 512 << 10 // 512 KiB

// LogSource names a directory of log files and the label shown to operators,
// for example {Label: "master", Dir: "logs"}.
type LogSource struct {
	Label string
	Dir   string
}

// LogFileInfo describes one log file in the directory listing. Source is the
// label of the directory the file came from, used to disambiguate identical
// file names across services.
type LogFileInfo struct {
	Source      string
	Name        string
	SizeBytes   int64
	ModifiedUTC time.Time
}

// LogFileContent is the (possibly truncated) tail of a single log file.
type LogFileContent struct {
	Source    string
	Name      string
	SizeBytes int64
	Content   string
	Truncated bool
}

// LogsReadService lists and reads application log files from one or more
// labeled directories. It never follows paths outside those directories.
type LogsReadService struct {
	sources []LogSource
}

// NewLogsReadService constructs the service over the given labeled sources.
// Sources with an empty label or directory are ignored. When no usable source
// remains, every call reports ErrLogsNotConfigured.
func NewLogsReadService(sources ...LogSource) *LogsReadService {
	cleaned := make([]LogSource, 0, len(sources))
	for _, source := range sources {
		label := strings.TrimSpace(source.Label)
		dir := strings.TrimSpace(source.Dir)
		if label == "" || dir == "" {
			continue
		}
		cleaned = append(cleaned, LogSource{Label: label, Dir: dir})
	}
	return &LogsReadService{sources: cleaned}
}

// List returns the log files across every configured source, newest first.
// Only regular files ending in .log are reported. A single unreadable source
// (for example a directory that does not exist yet) is skipped rather than
// failing the whole listing, so one service being down never hides another's
// logs.
func (service *LogsReadService) List(ctx context.Context) ([]LogFileInfo, error) {
	if service == nil || len(service.sources) == 0 {
		return nil, ErrLogsNotConfigured
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	files := make([]LogFileInfo, 0)
	for _, source := range service.sources {
		entries, err := os.ReadDir(source.Dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			files = append(files, LogFileInfo{Source: source.Label, Name: entry.Name(), SizeBytes: info.Size(), ModifiedUTC: info.ModTime().UTC()})
		}
	}
	sort.Slice(files, func(a, b int) bool { return files[a].ModifiedUTC.After(files[b].ModifiedUTC) })
	return files, nil
}

// Read returns the tail of a single log file identified by its source label
// and bare file name. The name must be a bare file name within the source
// directory; any path separators or parent references are rejected to prevent
// traversal.
func (service *LogsReadService) Read(ctx context.Context, source, name string) (LogFileContent, error) {
	if service == nil || len(service.sources) == 0 {
		return LogFileContent{}, ErrLogsNotConfigured
	}
	if err := ctx.Err(); err != nil {
		return LogFileContent{}, err
	}
	clean, label, err := service.resolve(source, name)
	if err != nil {
		return LogFileContent{}, err
	}
	info, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return LogFileContent{}, ErrLogFileNotFound
		}
		return LogFileContent{}, fmt.Errorf("stat log file: %w", err)
	}
	if info.IsDir() {
		return LogFileContent{}, ErrInvalidLogFile
	}
	file, err := os.Open(clean)
	if err != nil {
		return LogFileContent{}, fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	size := info.Size()
	truncated := false
	offset := int64(0)
	if size > maxLogContentBytes {
		offset = size - maxLogContentBytes
		truncated = true
	}
	if offset > 0 {
		if _, err := file.Seek(offset, 0); err != nil {
			return LogFileContent{}, fmt.Errorf("seek log file: %w", err)
		}
	}
	buffer := make([]byte, size-offset)
	read, err := file.Read(buffer)
	if err != nil && read == 0 {
		return LogFileContent{}, fmt.Errorf("read log file: %w", err)
	}
	return LogFileContent{Source: label, Name: info.Name(), SizeBytes: size, Content: string(buffer[:read]), Truncated: truncated}, nil
}

// resolve validates a source label and a bare file name, returning the file's
// path within the matching source directory plus the resolved label. When no
// source is supplied it falls back to the first configured source so a missing
// label never silently reads the wrong service's files.
func (service *LogsReadService) resolve(source, name string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", "", ErrInvalidLogFile
	}
	if !strings.HasSuffix(strings.ToLower(name), ".log") {
		return "", "", ErrInvalidLogFile
	}
	source = strings.TrimSpace(source)
	for _, candidate := range service.sources {
		if source == "" || candidate.Label == source {
			return filepath.Join(candidate.Dir, name), candidate.Label, nil
		}
	}
	return "", "", ErrInvalidLogFile
}
