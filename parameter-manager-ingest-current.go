package ibeamcorelib

import (
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	b "github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers"
	log "github.com/s00500/env_logger"
	"google.golang.org/protobuf/proto"
)

func (m *IBeamParameterManager) ingestCurrentParameter(parameter *pb.Parameter) {

	if err := m.checkValidParameter(parameter); err != nil {
		log.Error(err)
		return
	}

	parameterID := parameter.Id.Parameter
	deviceID := parameter.Id.Device
	modelID := m.parameterRegistry.getModelID(deviceID)

	// Get State and the Configuration (Details) of the Parameter
	m.parameterRegistry.muValue.Lock()
	defer m.parameterRegistry.muValue.Unlock()
	state := m.parameterRegistry.parameterValue
	m.parameterRegistry.muDetail.RLock()
	defer m.parameterRegistry.muDetail.RUnlock()
	parameterConfig := m.parameterRegistry.parameterDetail[modelID][parameterID]

	for _, newParameterValue := range parameter.Value {

		if newParameterValue == nil {
			log.Warnf("Received nil value for parameter %d from device %d", parameterID, deviceID)
			continue
		}
		// Check if Dimension is Valid
		if !state[deviceID][parameterID].MultiIndexHasValue(newParameterValue.DimensionID) {
			log.Errorf("Received invalid dimension id  %v for parameter %d from device %d", newParameterValue.DimensionID, parameterID, deviceID)
			continue
		}

		parameterDimension, err := state[deviceID][parameterID].MultiIndex(newParameterValue.DimensionID)
		if err != nil {
			log.Error(err)
			continue
		}
		parameterBuffer, err := parameterDimension.Value()
		if err != nil {
			log.Error(err)
			continue
		}

		if proto.Equal(parameterBuffer.currentValue, newParameterValue) {
			// if values are equal no need to do anything
			continue
		}

		parameterBuffer.reEvaluationTimerMu.Lock()
		if parameterBuffer.reEvaluationTimer != nil {
			parameterBuffer.reEvaluationTimer.timer.Stop()
			parameterBuffer.reEvaluationTimer = nil
		}
		parameterBuffer.reEvaluationTimerMu.Unlock()

		if newParameterValue.Value == nil {
			// Got empty value, need to update available or invalid
			if newParameterValue.Invalid {
				// if invalid is true set it
				parameterBuffer.currentValue.Invalid = newParameterValue.Invalid
			} else {
				// else set available
				parameterBuffer.available = newParameterValue.Available
			}

			if values := m.parameterRegistry.getInstanceValues(parameter.GetId()); values != nil {
				m.serverClientsStream <- b.Param(parameterID, deviceID, values...)
			}
			continue
		}

		// Check Type of Parameter
		switch parameterConfig.ValueType {
		case pb.ValueType_Opt:
			// If Type of Parameter is Opt, find the right Opt
			switch v := newParameterValue.Value.(type) {
			case *pb.ParameterValue_Str:
				id, err := getIDFromOptionListByElementName(parameterConfig.OptionList, v.Str)
				if err != nil {
					log.Error(err)
					continue
				}
				// Currently a option list can be set by firering in a currentoption or a string,
				// the manager will always output a currentoption value though

				newParameterValue.Value = &pb.ParameterValue_CurrentOption{
					CurrentOption: id,
				}

				if proto.Equal(parameterBuffer.currentValue, newParameterValue) {
					// In this one special case we could recheck if the values are still equal or not, and abort if they are now
					continue
				}

			case *pb.ParameterValue_OptionListUpdate:
				if !parameterConfig.OptionListIsDynamic {
					log.Errorf("Parameter with ID %v has no Dynamic OptionList", parameterID)
					continue
				}
				m.parameterRegistry.muDetail.RUnlock()
				m.parameterRegistry.muDetail.Lock()
				m.parameterRegistry.parameterDetail[modelID][parameterID].OptionList = v.OptionListUpdate
				m.parameterRegistry.muDetail.Unlock()
				m.parameterRegistry.muDetail.RLock()

				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			case *pb.ParameterValue_CurrentOption:
				// Handled below
			default:
				log.Errorf("Valuetype of Parameter is Opt and so we should get a String or Opt or currentOpt, but got %T", newParameterValue)
				continue
			}
		case pb.ValueType_Binary:
			if _, ok := newParameterValue.Value.(*pb.ParameterValue_Binary); !ok {
				log.Errorf("Parameter with ID %v is Type Binary but got %T", parameterID, parameterConfig.ValueType)
				continue
			}
		case pb.ValueType_Floating:
			switch v := newParameterValue.Value.(type) {
			case *pb.ParameterValue_MinimumUpdate:
				if !parameterConfig.MinMaxIsDynamic {
					log.Errorf("Parameter with ID %v has no dynamic min / max values", parameterID)
					continue
				}
				m.parameterRegistry.muDetail.RUnlock()

				m.parameterRegistry.muDetail.Lock()
				m.parameterRegistry.parameterDetail[modelID][parameterID].Minimum = v.MinimumUpdate
				m.parameterRegistry.muDetail.Unlock()

				m.parameterRegistry.muDetail.RLock()
				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			case *pb.ParameterValue_MaximumUpdate:
				if !parameterConfig.MinMaxIsDynamic {
					log.Errorf("Parameter with ID %v has no dynamic min / max values", parameterID)
					continue
				}
				m.parameterRegistry.muDetail.RUnlock()

				m.parameterRegistry.muDetail.Lock()
				m.parameterRegistry.parameterDetail[modelID][parameterID].Maximum = v.MaximumUpdate
				m.parameterRegistry.muDetail.Unlock()

				m.parameterRegistry.muDetail.RLock()
				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			}

			if _, ok := newParameterValue.Value.(*pb.ParameterValue_Floating); !ok {
				log.Errorf("Parameter with ID %v is Type Float but got %T", parameterID, parameterConfig.ValueType)
				continue
			}

			if newParameterValue.Value.(*pb.ParameterValue_Floating).Floating > parameterConfig.Maximum {
				log.Errorf("Ingest Current Loop: Max violation for parameter %v", parameterID)
				continue
			}
			if newParameterValue.Value.(*pb.ParameterValue_Floating).Floating < parameterConfig.Minimum {
				log.Errorf("Ingest Current Loop: Min violation for parameter %v", parameterID)
				continue
			}
		case pb.ValueType_Integer:
			switch v := newParameterValue.Value.(type) {
			case *pb.ParameterValue_MinimumUpdate:
				if !parameterConfig.MinMaxIsDynamic {
					log.Errorf("Parameter with ID %v has no dynamic min / max values", parameterID)
					continue
				}
				m.parameterRegistry.muDetail.RUnlock()

				m.parameterRegistry.muDetail.Lock()
				m.parameterRegistry.parameterDetail[modelID][parameterID].Minimum = v.MinimumUpdate
				m.parameterRegistry.muDetail.Unlock()

				m.parameterRegistry.muDetail.RLock()
				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			case *pb.ParameterValue_MaximumUpdate:
				if !parameterConfig.MinMaxIsDynamic {
					log.Errorf("Parameter with ID %v has no dynamic min / max values", parameterID)
					continue
				}
				m.parameterRegistry.muDetail.RUnlock()

				m.parameterRegistry.muDetail.Lock()
				m.parameterRegistry.parameterDetail[modelID][parameterID].Maximum = v.MaximumUpdate
				m.parameterRegistry.muDetail.Unlock()

				m.parameterRegistry.muDetail.RLock()
				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			}

			if _, ok := newParameterValue.Value.(*pb.ParameterValue_Integer); !ok {
				log.Errorf("Parameter with ID %v is Type Integer but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
				continue
			}

			if newParameterValue.Value.(*pb.ParameterValue_Integer).Integer > int32(parameterConfig.Maximum) {
				log.Errorf("Ingest Current Loop: Max violation for parameter %v, got %d", parameterID, newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)
				continue
			}
			if newParameterValue.Value.(*pb.ParameterValue_Integer).Integer < int32(parameterConfig.Minimum) {
				log.Errorf("Ingest Current Loop: Min violation for parameter %v, got %d", parameterID, newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)
				continue
			}

		case pb.ValueType_String:
			if _, ok := newParameterValue.Value.(*pb.ParameterValue_Str); !ok {
				log.Errorf("Parameter with ID %v is Type String but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
				continue
			}
		case pb.ValueType_NoValue:
			log.Errorf("Parameter with ID %v has No Value but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
			continue
		}

		assumed := false
		setTarget := false

		if time.Since(parameterBuffer.lastUpdate).Milliseconds()+1 > int64(parameterConfig.QuarantineDelayMs) {
			if parameterBuffer.isAssumedState { // earlier it was assumed, so now the current value has changed, so lets update target anyway
				parameterBuffer.targetValue = proto.Clone(newParameterValue).(*pb.ParameterValue)
				setTarget = true
			}
		} else {
			timeForRecheck := int64(parameterConfig.QuarantineDelayMs) - time.Since(parameterBuffer.lastUpdate).Milliseconds()
			m.reevaluateIn(time.Duration(time.Millisecond*time.Duration(timeForRecheck)), parameterBuffer, parameterID, deviceID)
		}

		parameterBuffer.currentValue = proto.Clone(newParameterValue).(*pb.ParameterValue)

		if !setTarget { // if both values where just set we do already know that it will be not assumed
			assumed = !proto.Equal(parameterBuffer.currentValue, parameterBuffer.targetValue)
		}

		if !assumed {
			parameterBuffer.tryCount = 0
		}

		if values := m.parameterRegistry.getInstanceValues(parameter.GetId()); values != nil {
			m.serverClientsStream <- b.Param(parameterID, deviceID, values...)
		}

		addr := paramDimensionAddress{
			parameter:   parameterID,
			device:      deviceID,
			dimensionID: parameterBuffer.getParameterValue().DimensionID,
		}
		m.reEvaluate(addr)
		// Trigger processing of the main evaluation
	}
}
