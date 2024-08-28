package ibeamcorelib

import (
	"fmt"
	"strings"
	"sync"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
)

var activeStatusMessages map[string]*pb.Parameter = make(map[string]*pb.Parameter) // Keep a list of active notifications
var activeStatusMessagesMu sync.RWMutex

func getActiveStatuses() (params []*pb.Parameter) {
	params = make([]*pb.Parameter, 0)
	activeStatusMessagesMu.RLock()
	defer activeStatusMessagesMu.RUnlock()

	for _, msg := range activeStatusMessages {
		params = append(params, msg)
	}

	return params
}

// ID needs prefixes so we avoid clashes, like on reactor side

func handleStatusParam(msg *pb.Parameter, toClients chan *pb.Parameter) {
	statusID, statusType := idFromParameter(msg)
	if statusID == "" {
		log.Error("invalid ID for error param")
		return
	}

	if statusType == pb.CustomErrorType_Resolve {
		hasChanged := false
		activeStatusMessagesMu.Lock()
		for key := range activeStatusMessages {
			if strings.HasPrefix(key, statusID) {
				delete(activeStatusMessages, key)
				hasChanged = true
			}
		}
		activeStatusMessagesMu.Unlock()
		if !hasChanged {
			return // nothing changed, no need to spam the system
		}
	} else {
		activeStatusMessagesMu.Lock()
		_, exists := activeStatusMessages[statusID]
		if exists { // only get new IDs in, no need to be spaming here
			activeStatusMessagesMu.Unlock()
			return
		}
		activeStatusMessages[statusID] = msg
		activeStatusMessagesMu.Unlock()
	}

	toClients <- msg
}

// Warning: only taking the last one... so this does not really support batching yet...
func idFromParameter(param *pb.Parameter) (statusID string, statusType pb.CustomErrorType) {
	if param == nil {
		return "", 0
	}

	vals := param.GetValue()
	if len(vals) == 0 {
		return "", 0
	}

	if vals[0].GetError().GetID() == "" {
		return "", 0
	}
	prefix := ""
	if param.Id.Parameter == 0 && param.Id.Device != 0 {
		prefix = fmt.Sprintf("device:%d:", param.Id.Device)
	} else if param.Id.Parameter != 0 && param.Id.Device != 0 {
		prefix = fmt.Sprintf("device:%d:parameter:%d:", param.Id.Device, param.Id.Parameter)
	}

	return prefix + vals[0].GetError().GetID(), vals[0].GetError().GetErrortype()
}
