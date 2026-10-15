package ibeamcorelib

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LogStreamEnabled allows clients to stream the log of the core via
// SubscribeLogs. If set, the environment variable IBEAM_LOG_STREAM=true|false
// overrides it.
var LogStreamEnabled = true

// logStreamEnabled returns whether log streaming is allowed, taking
// IBEAM_LOG_STREAM into account
var logStreamEnabled = sync.OnceValue(func() bool {
	enabled, err := logStreamSetting(os.Getenv("IBEAM_LOG_STREAM"), LogStreamEnabled)
	log.ShouldWrap(err, "on reading IBEAM_LOG_STREAM")
	return enabled
})

// logStreamSetting applies the IBEAM_LOG_STREAM value to the default
func logStreamSetting(value string, defaultEnabled bool) (bool, error) {
	if value == "" {
		return defaultEnabled, nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return defaultEnabled, fmt.Errorf("'%s' is not true or false, keeping %t", value, defaultEnabled)
	}
	return enabled, nil
}

// LogStreamBufferSize is how many log statements are buffered per log client
// before statements are dropped for it
var LogStreamBufferSize = 1000

// LogStreamMaxTextSize limits the message and each field value of a streamed
// statement, trace statements can dump whole parameter sets
var LogStreamMaxTextSize = 16 * 1024

// SubscribeLogs streams the log statements of the core up to the requested
// level. More verbose statements than the log config asks for are only
// produced while a client asks for them, see env_logger.AddSink.
func (s *IBeamServer) SubscribeLogs(req *pb.LogSubscribeRequest, stream pb.IbeamCore_SubscribeLogsServer) error {
	if !logStreamEnabled() {
		return status.Error(codes.Unavailable, "log streaming is disabled on this core")
	}
	level := min(logrus.Level(req.Level), logrus.TraceLevel)

	entries := make(chan *pb.LogEntry, LogStreamBufferSize)
	var dropped atomic.Uint32

	// Runs on the goroutine that logs: must never block and must not log
	remove := log.AddSink(level, func(line log.LogLine) {
		select {
		case entries <- toLogEntry(line):
		default:
			dropped.Add(1)
		}
	})
	defer remove()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case entry := <-entries:
			entry.Dropped = dropped.Swap(0)
			if err := stream.Send(entry); err != nil {
				return nil
			}
		}
	}
}

func toLogEntry(line log.LogLine) *pb.LogEntry {
	entry := &pb.LogEntry{
		Time:    line.Time.UnixNano(),
		Level:   pb.LogLevel(line.Level),
		Module:  clipText(line.Module),
		Message: clipText(line.Message),
	}
	if len(line.Fields) > 1 {
		entry.Fields = make(map[string]string, len(line.Fields)-1)
		for key, value := range line.Fields {
			if key != "module" {
				entry.Fields[clipText(key)] = clipText(fmt.Sprint(value))
			}
		}
	}
	return entry
}

// clipText makes s fit LogStreamMaxTextSize and valid UTF-8, which every
// proto string has to be, or sending the entry fails and ends the stream.
func clipText(s string) string {
	if len(s) > LogStreamMaxTextSize {
		cut := LogStreamMaxTextSize
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = fmt.Sprintf("%s… (%d bytes cut)", s[:cut], len(s)-cut)
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	return s
}
