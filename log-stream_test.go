package ibeamcorelib

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func TestToLogEntry(t *testing.T) {
	now := time.Now()
	entry := toLogEntry(log.LogLine{
		Time:    now,
		Level:   logrus.DebugLevel,
		Module:  "ib/server",
		Message: "hello",
		Fields:  logrus.Fields{"module": "ib/server", "count": 3},
	})
	if entry.Time != now.UnixNano() || entry.Level != pb.LogLevel_LogDebug || entry.Module != "ib/server" || entry.Message != "hello" {
		t.Fatalf("unexpected entry %v", entry)
	}
	if len(entry.Fields) != 1 || entry.Fields["count"] != "3" {
		t.Fatalf("unexpected fields %v", entry.Fields)
	}
}

// Whatever is logged has to be sendable, or the stream breaks
func TestToLogEntryIsSendable(t *testing.T) {
	huge := strings.Repeat("ä", LogStreamMaxTextSize) // 2 bytes per rune
	invalid := string([]byte{0xff, 0xfe, 'x'})
	entry := toLogEntry(log.LogLine{
		Module:  invalid,
		Message: huge,
		Fields:  logrus.Fields{"module": invalid, "raw": invalid, invalid: "key"},
	})
	if len(entry.Message) > LogStreamMaxTextSize+64 || !utf8.ValidString(entry.Message) {
		t.Fatalf("message not clipped to valid UTF-8, %d bytes", len(entry.Message))
	}
	if !strings.HasSuffix(entry.Message, "bytes cut)") {
		t.Fatalf("clipping not marked: %q", entry.Message[len(entry.Message)-30:])
	}
	if _, err := proto.Marshal(entry); err != nil {
		t.Fatal(err)
	}
}

func TestLogStreamSetting(t *testing.T) {
	cases := []struct {
		value          string
		defaultEnabled bool
		want           bool
		wantErr        bool
	}{
		{"", true, true, false},
		{"", false, false, false},
		{"false", true, false, false},
		{"0", true, false, false},
		{"true", false, true, false},
		{"TRUE", false, true, false},
		{"nope", true, true, true},
		{"nope", false, false, true},
	}
	for _, c := range cases {
		got, err := logStreamSetting(c.value, c.defaultEnabled)
		if got != c.want || (err != nil) != c.wantErr {
			t.Errorf("logStreamSetting(%q, %t) = %t, %v", c.value, c.defaultEnabled, got, err)
		}
	}
}
