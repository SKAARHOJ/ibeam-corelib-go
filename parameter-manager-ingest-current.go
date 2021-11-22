package ibeamcorelib

import (
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	b "github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers"
	log "github.com/s00500/env_logger"
	"google.golang.org/protobuf/proto"
)

// OptionFloatCurrentOverridesTargetDuringRetry allows to make the manager accept any value sent by the camera for a float as if it would be the correct target value
// This is needed on devices that do some rounding on their own, making it impossible to get the accurate desired value
// Warning: this option basically disables the manager for floats, in a leter version of corelib this needs replacement with something configurable per parameter
var OptionFloatCurrentOverridesTargetDuringRetry bool = false

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
			log.Warnf("Received nil value for ", m.pName(parameter.Id))
			continue
		}
		// Check if Dimension is Valid
		if !state[deviceID][parameterID].multiIndexHasValue(newParameterValue.DimensionID) {
			log.Errorf("Received invalid dimension id  %v for %s", newParameterValue.DimensionID, m.pName(parameter.Id))
			continue
		}

		parameterDimension, err := state[deviceID][parameterID].multiIndex(newParameterValue.DimensionID)
		if err != nil {
			log.Error(err)
			continue
		}
		parameterBuffer, err := parameterDimension.getValue()
		if err != nil {
			log.Error(err)
			continue
		}

		if parameterBuffer.currentEquals(newParameterValue) {
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

				if parameterBuffer.tryCount == 0 { // Make sure we are not in the process of trying atm
					if time.Since(parameterBuffer.lastUpdate).Milliseconds()+1 > int64(parameterConfig.QuarantineDelayMs) {
						parameterBuffer.targetValue.Invalid = newParameterValue.Invalid
					}
				}
			} else {
				// else set available
				parameterBuffer.available = newParameterValue.Available
			}

			if values := m.parameterRegistry.getInstanceValues(parameter.GetId(), false); values != nil {
				m.serverClientsStream <- b.Param(parameterID, deviceID, values...)
			}
			continue
		}

		// Check Type of Parameter
		switch parameterConfig.ValueType {
		case pb.ValueType_Opt:
			switch v := newParameterValue.Value.(type) {
			case *pb.ParameterValue_Str:
				// If Type of Parameter is Opt and we get a string, find the right Opt
				optionlist := parameterConfig.OptionList
				if parameterConfig.OptionListIsDynamic && parameterBuffer.dynamicOptions != nil {
					optionlist = parameterBuffer.dynamicOptions
				}

				id, err := getIDFromOptionListByElementName(optionlist, v.Str)
				if err != nil {
					log.Error("on optionlist name lookup: ", err)
					continue
				}

				newParameterValue.Value = &pb.ParameterValue_CurrentOption{
					CurrentOption: id,
				}

			case *pb.ParameterValue_OptionListUpdate:
				if !parameterConfig.OptionListIsDynamic {
					log.Errorf("Parameter with ID %v has no Dynamic OptionList", parameterID)
					continue
				}

				parameterBuffer.dynamicOptions = proto.Clone(v.OptionListUpdate).(*pb.OptionList)

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

				newMin := v.MinimumUpdate
				parameterBuffer.dynamicMin = &newMin

				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			case *pb.ParameterValue_MaximumUpdate:
				if !parameterConfig.MinMaxIsDynamic {
					log.Errorf("Parameter with ID %v has no dynamic min / max values", parameterID)
					continue
				}

				newMax := v.MaximumUpdate
				parameterBuffer.dynamicMax = &newMax

				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			}

			minimum := parameterConfig.Minimum
			maximum := parameterConfig.Maximum
			if parameterConfig.MinMaxIsDynamic {
				if parameterBuffer.dynamicMin != nil {
					minimum = *parameterBuffer.dynamicMin
				}
				if parameterBuffer.dynamicMax != nil {
					maximum = *parameterBuffer.dynamicMax
				}
			}

			if _, ok := newParameterValue.Value.(*pb.ParameterValue_Floating); !ok {
				log.Errorf("Parameter with ID %v is Type Float but got %T", parameterID, parameterConfig.ValueType)
				continue
			}

			// Limit the maximum float precision to ensure we do not cause troubles on parsing, // LB: Currently disabled
			//log.Warn("Ingest Current ", newParameterValue.Value.(*pb.ParameterValue_Floating).Floating)
			//newParameterValue.Value.(*pb.ParameterValue_Floating).Floating = math.Round(newParameterValue.Value.(*pb.ParameterValue_Floating).Floating*10000) / 10000
			//log.Warn("Ingest Current round ", math.Round(newParameterValue.Value.(*pb.ParameterValue_Floating).Floating*10000)/10000)

			if newParameterValue.Value.(*pb.ParameterValue_Floating).Floating > maximum {
				if !isDescreteValue(parameterConfig, newParameterValue.Value.(*pb.ParameterValue_Floating).Floating) {
					log.Error("Ingest Current Loop: Max violation for ", m.pName(parameter.Id))
					continue
				}
			}
			if newParameterValue.Value.(*pb.ParameterValue_Floating).Floating < minimum {
				if !isDescreteValue(parameterConfig, newParameterValue.Value.(*pb.ParameterValue_Floating).Floating) {
					log.Error("Ingest Current Loop: Min violation for ", m.pName(parameter.Id))
					continue
				}
			}
		case pb.ValueType_Integer:
			switch v := newParameterValue.Value.(type) {
			case *pb.ParameterValue_MinimumUpdate:
				if !parameterConfig.MinMaxIsDynamic {
					log.Error("No dynamic minmax set on ", m.pName(parameter.Id))
					continue
				}

				newMin := v.MinimumUpdate
				parameterBuffer.dynamicMin = &newMin

				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			case *pb.ParameterValue_MaximumUpdate:
				if !parameterConfig.MinMaxIsDynamic {
					log.Error("No dynamic minmax set on ", m.pName(parameter.Id))
					continue
				}

				newMax := v.MaximumUpdate
				parameterBuffer.dynamicMax = &newMax

				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			}

			if _, ok := newParameterValue.Value.(*pb.ParameterValue_Integer); !ok {
				log.Errorf("%s is Type Integer but got %T", m.pName(parameter.Id), parameterConfig.ValueType)
				continue
			}

			minimum := parameterConfig.Minimum
			maximum := parameterConfig.Maximum
			if parameterConfig.MinMaxIsDynamic {
				if parameterBuffer.dynamicMin != nil {
					minimum = *parameterBuffer.dynamicMin
				}
				if parameterBuffer.dynamicMax != nil {
					maximum = *parameterBuffer.dynamicMax
				}
			}

			if newParameterValue.Value.(*pb.ParameterValue_Integer).Integer > int32(maximum) {
				if !isDescreteValue(parameterConfig, float64(newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)) {
					log.Errorf("Ingest Current Loop: Max violation for %s, got %d", m.pName(parameter.Id), newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)
					continue
				}
			}
			if newParameterValue.Value.(*pb.ParameterValue_Integer).Integer < int32(minimum) {
				if !isDescreteValue(parameterConfig, float64(newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)) {
					log.Errorf("Ingest Current Loop: Min violation for %s, got %d", m.pName(parameter.Id), newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)
					continue
				}
			}

		case pb.ValueType_String:
			if _, ok := newParameterValue.Value.(*pb.ParameterValue_Str); !ok {
				log.Errorf("%s is Type String but got %T", m.pName(parameter.Id), parameterConfig.ValueType)
				continue
			}
		case pb.ValueType_NoValue:
			log.Errorf("%s has No Value but got %T", m.pName(parameter.Id), parameterConfig.ValueType)
			continue
		}
		parameterBuffer.currentValue = proto.Clone(newParameterValue).(*pb.ParameterValue)

		didSetTarget := false
		didScheduleReEval := false

		if (OptionFloatCurrentOverridesTargetDuringRetry && parameterConfig.ValueType == pb.ValueType_Floating) || parameterBuffer.tryCount == 0 { // Make sure we are not in the process of trying atm
			if time.Since(parameterBuffer.lastUpdate).Milliseconds()+1 > int64(parameterConfig.QuarantineDelayMs) {
				didSetTarget = true
				if !proto.Equal(parameterBuffer.targetValue, newParameterValue) {
					parameterBuffer.targetValue = proto.Clone(newParameterValue).(*pb.ParameterValue)
				}
			} else {
				timeForRecheck := int64(parameterConfig.QuarantineDelayMs) - time.Since(parameterBuffer.lastUpdate).Milliseconds()
				m.reevaluateIn(time.Millisecond*time.Duration(timeForRecheck), parameterBuffer, parameterID, deviceID)
				didScheduleReEval = true
			}
		}

		assumed := false
		if didSetTarget {
			assumed = false
		} else {
			assumed = !parameterBuffer.currentEquals(parameterBuffer.targetValue)
		}

		parameterBuffer.isAssumedState.Store(assumed)

		if !assumed {
			parameterBuffer.tryCount = 0
		}

		if values := m.parameterRegistry.getInstanceValues(parameter.GetId(), false); values != nil {
			m.serverClientsStream <- b.Param(parameterID, deviceID, values...)
		}

		if !didScheduleReEval {
			// Trigger processing of the main evaluation

			addr := paramDimensionAddress{
				parameter:   parameterID,
				device:      deviceID,
				dimensionID: parameterBuffer.getParameterValue().DimensionID,
			}
			//Trigger process main, only after control delay has passed

			if parameterConfig.ControlDelayMs != 0 && time.Since(parameterBuffer.lastUpdate).Milliseconds() < int64(parameterConfig.ControlDelayMs) {
				m.reevaluateIn(time.Millisecond*time.Duration(parameterConfig.ControlDelayMs)-time.Since(parameterBuffer.lastUpdate), parameterBuffer, parameterID, deviceID)
				return
			} else {
				m.reEvaluate(addr)
			}
		}
	}
}
