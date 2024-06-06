package ibeamcorelib

import (
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	b "github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers"
	"google.golang.org/protobuf/proto"
)

func (m *IBeamParameterManager) ingestCurrentParameter(parameter *pb.Parameter) {
	parameterID := parameter.Id.Parameter
	deviceID := parameter.Id.Device
	mlog := m.log.WithField("paramID", parameterID).WithField("deviceID", deviceID)

	// Direct Passthrough for custom errors
	if parameter.Error == pb.ParameterError_Custom {
		for _, val := range parameter.GetValue() {
			if val.GetError() == nil || (val.GetError().GetErrortype() != pb.CustomErrorType_Resolve && val.GetError().GetMessage() == "") {
				mlog.Error("Parameter Error without message received, not forwarding to clients")
				return
			}
		}
		handleStatusParam(parameter, m.serverClientsStream)
		return
	}

	// Get State and the Configuration (Details) of the Parameter
	m.parameterRegistry.muValue.Lock()
	defer m.parameterRegistry.muValue.Unlock()
	state := m.parameterRegistry.parameterValue
	m.parameterRegistry.muDetail.RLock()
	defer m.parameterRegistry.muDetail.RUnlock()

	modelID := m.parameterRegistry.getModelID(deviceID)
	parameterConfig := m.parameterRegistry.parameterDetail[modelID][parameterID]

	if err := m.checkValidParameter(parameter); err != nil {
		mlog.Error(err)
		return
	}

	// m.ingestCurrentCounter.Add(1)

	reprocessBuffers := make([]*ibeamParameterValueBuffer, 0)
	sendBuffers := make([]*ibeamParameterValueBuffer, 0)

	for _, newParameterValue := range parameter.Value {
		if newParameterValue == nil {
			mlog.Warn("Received nil value for ", m.pName(parameter.Id))
			continue
		}
		// Check if Dimension is Valid
		if !state[deviceID][parameterID].multiIndexHasValue(newParameterValue.DimensionID) {
			mlog.Errorf("Received invalid dimension id  %v for %s", newParameterValue.DimensionID, m.pName(parameter.Id))
			continue
		}

		parameterDimension, err := state[deviceID][parameterID].multiIndex(newParameterValue.DimensionID)
		if err != nil {
			mlog.Error(err)
			continue
		}
		parameterBuffer, err := parameterDimension.getValue()
		if err != nil {
			mlog.Error(err)
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
				parameterBuffer.currentValue.Value = b.Invalid(newParameterValue.DimensionID...).Value

				if parameterBuffer.tryCount == 0 { // Make sure we are not in the process of trying atm
					if time.Since(parameterBuffer.lastUpdate).Milliseconds()+1 > int64(parameterConfig.QuarantineDelayMs) {
						parameterBuffer.targetValue.Invalid = newParameterValue.Invalid
						parameterBuffer.targetValue.Value = b.Invalid(newParameterValue.DimensionID...).Value
					}
				}
			} else {
				// else set available
				parameterBuffer.available = newParameterValue.Available
			}
			sendBuffers = append(sendBuffers, parameterBuffer)

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
					mlog.Error("on optionlist name lookup: ", err)
					continue
				}

				newParameterValue.Value = &pb.ParameterValue_CurrentOption{
					CurrentOption: id,
				}

			case *pb.ParameterValue_OptionListUpdate:
				if !parameterConfig.OptionListIsDynamic {
					mlog.Errorf("Parameter with ID %v has no Dynamic OptionList", parameterID)
					continue
				}

				parameterBuffer.dynamicOptions = proto.Clone(v.OptionListUpdate).(*pb.OptionList)

				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			case *pb.ParameterValue_CurrentOption:
				// Handled below
			default:
				mlog.Errorf("Valuetype of Parameter is Opt and so we should get a String or Opt or currentOpt, but got %T", newParameterValue)
				continue
			}
		case pb.ValueType_Binary:
			if _, ok := newParameterValue.Value.(*pb.ParameterValue_Binary); !ok {
				mlog.Errorf("Parameter with ID %v is Type Binary but got %s %T", parameterID, parameterConfig.ValueType.String(), newParameterValue.Value)
				continue
			}
		case pb.ValueType_Floating:
			switch v := newParameterValue.Value.(type) {
			case *pb.ParameterValue_MinimumUpdate:
				if !parameterConfig.MinMaxIsDynamic {
					mlog.Errorf("Parameter with ID %v has no dynamic min / max values", parameterID)
					continue
				}

				newMin := v.MinimumUpdate
				parameterBuffer.dynamicMin = &newMin

				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			case *pb.ParameterValue_MaximumUpdate:
				if !parameterConfig.MinMaxIsDynamic {
					mlog.Errorf("Parameter with ID %v has no dynamic min / max values", parameterID)
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
				mlog.Errorf("Parameter with ID %v is Type Float but got %s", parameterID, parameterConfig.ValueType.String())
				continue
			}

			// Limit the maximum float precision to ensure we do not cause troubles on parsing, // LB: Currently disabled
			//log.Warn("Ingest Current ", newParameterValue.Value.(*pb.ParameterValue_Floating).Floating)
			//newParameterValue.Value.(*pb.ParameterValue_Floating).Floating = math.Round(newParameterValue.Value.(*pb.ParameterValue_Floating).Floating*10000) / 10000
			//log.Warn("Ingest Current round ", math.Round(newParameterValue.Value.(*pb.ParameterValue_Floating).Floating*10000)/10000)

			if newParameterValue.Value.(*pb.ParameterValue_Floating).Floating > maximum {
				if !isDescreteValue(parameterConfig, newParameterValue.Value.(*pb.ParameterValue_Floating).Floating) {
					mlog.Errorln("Ingest Current Loop: Max violation for", m.pName(parameter.Id), newParameterValue.Value.(*pb.ParameterValue_Floating).Floating)
					continue
				}
			}
			if newParameterValue.Value.(*pb.ParameterValue_Floating).Floating < minimum {
				if !isDescreteValue(parameterConfig, newParameterValue.Value.(*pb.ParameterValue_Floating).Floating) {
					mlog.Errorln("Ingest Current Loop: Min violation for", m.pName(parameter.Id), newParameterValue.Value.(*pb.ParameterValue_Floating).Floating)
					continue
				}
			}
		case pb.ValueType_Integer:
			switch v := newParameterValue.Value.(type) {
			case *pb.ParameterValue_MinimumUpdate:
				if !parameterConfig.MinMaxIsDynamic {
					mlog.Error("No dynamic minmax set on ", m.pName(parameter.Id))
					continue
				}

				newMin := v.MinimumUpdate
				parameterBuffer.dynamicMin = &newMin

				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			case *pb.ParameterValue_MaximumUpdate:
				if !parameterConfig.MinMaxIsDynamic {
					mlog.Error("No dynamic minmax set on ", m.pName(parameter.Id))
					continue
				}

				newMax := v.MaximumUpdate
				parameterBuffer.dynamicMax = &newMax

				m.serverClientsStream <- b.Param(parameterID, deviceID, newParameterValue)
				continue
			}

			if _, ok := newParameterValue.Value.(*pb.ParameterValue_Integer); !ok {
				mlog.Errorf("%s is Type Integer but got %s", m.pName(parameter.Id), parameterConfig.ValueType.String())
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
					mlog.Errorf("Ingest Current Loop: Max violation for %s, got %d", m.pName(parameter.Id), newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)
					continue
				}
			}
			if newParameterValue.Value.(*pb.ParameterValue_Integer).Integer < int32(minimum) {
				if !isDescreteValue(parameterConfig, float64(newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)) {
					mlog.Errorf("Ingest Current Loop: Min violation for %s, got %d", m.pName(parameter.Id), newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)
					continue
				}
			}

		case pb.ValueType_String:
			if _, ok := newParameterValue.Value.(*pb.ParameterValue_Str); !ok {
				mlog.Errorf("%s is Type String but got %s", m.pName(parameter.Id), parameterConfig.ValueType.String())
				continue
			}
		case pb.ValueType_NoValue:
			mlog.Errorf("%s has No Value but got %s", m.pName(parameter.Id), parameterConfig.ValueType.String())
			continue
		}
		parameterBuffer.currentValue = proto.Clone(newParameterValue).(*pb.ParameterValue)

		didSetTarget := false
		didScheduleReEval := false

		if parameterBuffer.tryCount == 0 { // Make sure we are not in the process of trying atm
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

		if parameterConfig.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
			// always overide full here
			didSetTarget = true
			assumed = false
			if !proto.Equal(parameterBuffer.targetValue, newParameterValue) {
				parameterBuffer.targetValue = proto.Clone(newParameterValue).(*pb.ParameterValue)
			}
		}

		if didSetTarget {
			assumed = false
		} else {
			assumed = !parameterBuffer.currentEquals(parameterBuffer.targetValue)
			if assumed && (parameterConfig.ValueType == pb.ValueType_Floating || parameterConfig.ValueType == pb.ValueType_Integer) && parameterConfig.AcceptanceThreshold != 0 {
				isAcceptanceModeOverride := false
				// ceck for override
				if parameterConfig.ValueType == pb.ValueType_Floating {
					tv := parameterBuffer.targetValue.GetFloating()
					cv := parameterBuffer.currentValue.GetFloating()
					isAcceptanceModeOverride = (tv-parameterConfig.AcceptanceThreshold < cv && cv < tv+parameterConfig.AcceptanceThreshold)
				}

				if parameterConfig.ValueType == pb.ValueType_Integer {
					tv := parameterBuffer.targetValue.GetInteger()
					cv := parameterBuffer.currentValue.GetInteger()
					isAcceptanceModeOverride = (tv-int32(parameterConfig.AcceptanceThreshold) < cv && cv < tv+int32(parameterConfig.AcceptanceThreshold))
				}

				if isAcceptanceModeOverride {
					assumed = false
					didSetTarget = true
					parameterBuffer.targetValue = proto.Clone(newParameterValue).(*pb.ParameterValue)
				}
			}
		}

		parameterBuffer.isAssumedState.Store(assumed)

		if !assumed {
			parameterBuffer.tryCount = 0
		}

		sendBuffers = append(sendBuffers, parameterBuffer)

		if !didScheduleReEval {
			reprocessBuffers = append(reprocessBuffers, parameterBuffer)
		}
	}

	if len(sendBuffers) != 0 {
		values := make([]*pb.ParameterValue, len(sendBuffers))
		for i, sb := range sendBuffers {
			values[i] = sb.getParameterValue()
		}
		m.serverClientsStream <- b.Param(parameterID, deviceID, values...)
	}

	for _, repro := range reprocessBuffers {
		// Trigger processing of the main evaluation
		addr := paramDimensionAddress{
			parameter:   parameterID,
			device:      deviceID,
			dimensionID: repro.getParameterValue().DimensionID,
		}

		//Trigger process main, only after control delay has passed
		if parameterConfig.ControlDelayMs != 0 && time.Since(repro.lastUpdate).Milliseconds() < int64(parameterConfig.ControlDelayMs) {
			m.reevaluateIn(time.Millisecond*time.Duration(parameterConfig.ControlDelayMs)-time.Since(repro.lastUpdate), repro, parameterID, deviceID)
			return
		} else {
			m.reEvaluate(addr)
		}
	}
}
