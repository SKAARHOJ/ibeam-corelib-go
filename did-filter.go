package ibeamcorelib

import (
	"os"
	"slices"
	"strconv"
	"strings"

	log "github.com/s00500/env_logger"
)

type HasDeviceID interface {
	GetDeviceID() uint32
}

func ParseDIDFilter() []int {
	raw := os.Getenv("DID")
	if raw == "" {
		return nil
	}
	var result []int
	for sV := range strings.SplitSeq(raw, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(sV))
		if log.ShouldWrap(err, "on parsing DID filter: ") {
			continue
		}
		result = append(result, v)
	}
	return result
}

func FilterDevicesByDID[T HasDeviceID](devices []T) []T {
	filter := ParseDIDFilter()
	if len(filter) == 0 {
		return devices
	}
	return slices.DeleteFunc(slices.Clone(devices), func(d T) bool {
		return !slices.Contains(filter, int(d.GetDeviceID()))
	})
}
