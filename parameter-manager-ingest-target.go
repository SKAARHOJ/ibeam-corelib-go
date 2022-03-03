package ibeamcorelib

import (
	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	b "github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers"
	"google.golang.org/protobuf/proto"
)

func (m *IBeamParameterManager) ingestTargetParameter(parameter *pb.Parameter) {
	mlog := m.log

	// Get Index and ID for Device and Parameter and the actual state of all parameters
	parameterID := parameter.Id.Parameter
	deviceID := parameter.Id.Device
	modelIndex := m.parameterRegistry.getModelID(deviceID)

	// Get State and the Configuration (Details) of the Parameter
	m.parameterRegistry.muValue.Lock()
	defer m.parameterRegistry.muValue.Unlock()
	state := m.parameterRegistry.parameterValue

	m.parameterRegistry.muDetail.RLock()
	defer m.parameterRegistry.muDetail.RUnlock()
	parameterConfig := m.parameterRegistry.parameterDetail[modelIndex][parameterID]

	if errorParam := m.checkValidParameter(parameter); errorParam != nil {
		m.serverClientsStream <- errorParam
		return
	}

	//m.ingestTargetCounter.Add(1)

	// Handle every Value in that was given for the Parameter
valueLoop:
	for _, newParameterValue := range parameter.Value {
		// Check if the NewValue has a Value
		if newParameterValue.Value == nil {
			mlog.Error("Received no value for ", m.pName(parameter.Id))
			m.serverClientsStream <- &pb.Parameter{
				Id:    parameter.Id,
				Error: pb.ParameterError_HasNoValue,
				Value: []*pb.ParameterValue{},
			}
			continue
		}

		// Check if dimension of the value is valid
		if !state[deviceID][parameterID].multiIndexHasValue(newParameterValue.DimensionID) {
			mlog.Errorf("Received invalid Dimension %d for %s", newParameterValue.DimensionID, m.pName(parameter.Id))
			m.serverClientsStream <- &pb.Parameter{
				Id:    parameter.Id,
				Error: pb.ParameterError_UnknownID,
				Value: []*pb.ParameterValue{},
			}
			continue
		}
		dimension, err := state[deviceID][parameterID].multiIndex(newParameterValue.DimensionID)
		if err != nil {
			mlog.Error(err)
			continue
		}
		parameterBuffer, err := dimension.getValue()
		if err != nil {
			mlog.Errorf("Could not get value for dimension id %v,: %v", newParameterValue.DimensionID, err)
			continue
		}

		if !dimension.value.available {
			mlog.Errorf("Ingest Target Loop: Unavailable for %s, DimensionID: %v", m.pName(parameter.Id), newParameterValue.DimensionID)
			m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_Unavailable)
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

		optionlist := parameterConfig.OptionList
		if parameterConfig.OptionListIsDynamic && parameterBuffer.dynamicOptions != nil {
			optionlist = parameterBuffer.dynamicOptions
		}

		// Check if meta values are set check min/max and options
		if len(newParameterValue.MetaValues) > 0 {
			for name, mValue := range newParameterValue.MetaValues {

				mConfig, exists := parameterConfig.MetaDetails[name]
				if !exists {
					mlog.Warnf("Received undefined meta value called %s for %s", name, m.pName(parameter.Id))
					continue // accept invalid options
				}

				if mConfig.Minimum != 0 && mConfig.Maximum != 0 {
					v := float64(0)
					if mConfig.MetaType == pb.ParameterMetaType_MetaFloating {
						v = mValue.GetFloating()

					} else if mConfig.MetaType == pb.ParameterMetaType_MetaInteger {
						v = float64(mValue.GetInteger())
					}
					if v > mConfig.Maximum {
						mlog.Errorf("MaxViolation for meta value %s of %s : %f > %f", name, m.pName(parameter.Id), v, mConfig.Maximum)
						m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MaxViolation)
						continue valueLoop
					} else if v < mConfig.Minimum {
						mlog.Errorf("MinViolation for parameter meta value %s for %s : %f < %f", name, m.pName(parameter.Id), v, mConfig.Minimum)
						m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MinViolation)
						continue valueLoop
					}
				}

				if mConfig.MetaType == pb.ParameterMetaType_MetaOption {
					if !containsString(mValue.GetStr(), mConfig.Options) {
						mlog.Errorf("Received undefined meta option value called %s for metavalue %s for %s", mValue.GetStr(), name, m.pName(parameter.Id))
						m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_UnknownID)
						continue valueLoop
					}
				}
			}
		}

		// Check if required meta are set
		for mName, meta := range parameterConfig.MetaDetails {
			if meta.Required {
				_, exists := newParameterValue.MetaValues[mName]
				if !exists {
					mlog.Errorf("Required MetaValue missing for parameter meta value %s for %s", mName, m.pName(parameter.Id))
					m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_HasNoValue)
					continue valueLoop
				}
			}
		}

		// Check if th evalue is already set!
		newParameterValue.Available = parameterBuffer.currentValue.Available // on an incoming request we can savely ignore the available, it will likely be false anyways
		if parameterBuffer.currentEquals(newParameterValue) {
			// if values are equal no need to do anything
			continue
		}

		// Check if Value is valid and has the right Type
		switch newValue := newParameterValue.Value.(type) {
		case *pb.ParameterValue_Integer:
			if parameterConfig.ValueType != pb.ValueType_Integer {
				mlog.Errorf("Got Value with Type %T for %s, but it needs %v", newValue, m.pName(parameter.Id), pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}

			if newValue.Integer > int32(maximum) {
				if !isDescreteValue(parameterConfig, float64(newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)) {
					mlog.Errorf("Ingest Target Loop: Max violation for %s, %d", m.pName(parameter.Id), newValue.Integer)
					m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MaxViolation)
					continue
				}
			}
			if newValue.Integer < int32(minimum) {
				if !isDescreteValue(parameterConfig, float64(newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)) {
					mlog.Errorf("Ingest Target Loop: Min violation for %s, %d", m.pName(parameter.Id), newValue.Integer)
					m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MinViolation)
					continue
				}
			}
		case *pb.ParameterValue_IncDecSteps:
			if newValue.IncDecSteps > parameterConfig.IncDecStepsUpperLimit || newValue.IncDecSteps < parameterConfig.IncDecStepsLowerLimit {
				mlog.Errorf("In- or Decrementation Step %v is outside of limits [%v,%v] of %s", newValue.IncDecSteps, parameterConfig.IncDecStepsLowerLimit, parameterConfig.IncDecStepsUpperLimit, m.pName(parameter.Id))
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_StepSizeViolation)
				continue
			}

			if parameterBuffer.hasFlag(FlagIncrementalPassthrough) {
				// basically we just send this out without caring too much...
				m.out <- &pb.Parameter{
					Id:    parameter.Id,
					Error: 0,
					Value: []*pb.ParameterValue{newParameterValue},
				}
				continue
			}

			// inc dec currently only works with integers or no values, float is strange but implemented, action lists need to be evaluated
			if parameterConfig.ValueType != pb.ValueType_Floating && parameterConfig.ValueType != pb.ValueType_Integer && parameterConfig.ValueType != pb.ValueType_NoValue {
				mlog.Errorf("Got Value with Type %T for %s, but it needs %v", newValue, m.pName(parameter.Id), pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}

			if parameterConfig.ValueType == pb.ValueType_Integer {
				newIntVal := parameterBuffer.targetValue.GetInteger() + newValue.IncDecSteps
				mlog.Tracef("Decrement %d by %d", parameterBuffer.targetValue.GetInteger(), newValue.IncDecSteps)
				if newIntVal <= int32(maximum) && newIntVal >= int32(minimum) {
					parameterBuffer.targetValue.Value = &pb.ParameterValue_Integer{Integer: newIntVal}
					parameterBuffer.targetValue.Invalid = false
					if parameterConfig.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
						parameterBuffer.currentValue.Value = &pb.ParameterValue_Integer{Integer: newIntVal}
					}
					// send out right away
					m.serverClientsStream <- b.Param(parameterID, deviceID, parameterBuffer.getParameterValue())
					continue // make sure we skip the rest of the logic :-)
				}
			}

			if parameterConfig.ValueType == pb.ValueType_Floating {
				newFloatVal := parameterBuffer.targetValue.GetFloating() + float64(newValue.IncDecSteps)
				mlog.Tracef("Decrement %d by %d", parameterBuffer.targetValue.GetFloating(), newValue.IncDecSteps)
				if newFloatVal <= maximum && newFloatVal >= minimum {
					parameterBuffer.targetValue.Value = &pb.ParameterValue_Floating{Floating: newFloatVal}
					parameterBuffer.targetValue.Invalid = false
					if parameterConfig.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
						parameterBuffer.currentValue.Value = &pb.ParameterValue_Floating{Floating: newFloatVal}
					}
					// send out right away
					m.serverClientsStream <- b.Param(parameterID, deviceID, parameterBuffer.getParameterValue())
					continue // make sure we skip the rest of the logic :-)
				}
			}

			m.out <- &pb.Parameter{
				Id:    parameter.Id,
				Error: 0,
				Value: []*pb.ParameterValue{newParameterValue},
			}
			continue

		case *pb.ParameterValue_Floating:
			if parameterConfig.ValueType != pb.ValueType_Floating {
				mlog.Errorf("Got Value with Type %T for %s, but it needs %v", newValue, m.pName(parameter.Id), parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}

			if newValue.Floating > maximum {
				if !isDescreteValue(parameterConfig, newParameterValue.Value.(*pb.ParameterValue_Floating).Floating) {
					mlog.Errorln("Max violation for", m.pName(parameter.Id))
					m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MaxViolation)
					continue
				}
			}
			if newValue.Floating < minimum {
				if !isDescreteValue(parameterConfig, newParameterValue.Value.(*pb.ParameterValue_Floating).Floating) {
					mlog.Errorln("Min violation for", m.pName(parameter.Id))
					m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MinViolation)
					continue
				}
			}
		case *pb.ParameterValue_Str:
			if parameterConfig.ValueType != pb.ValueType_String {
				mlog.Errorf("Got Value with Type %T for %s, but it needs %v", newValue, m.pName(parameter.Id), pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}

			// String does not need extra check

		case *pb.ParameterValue_CurrentOption:

			if optionlist == nil {
				mlog.Errorln("No option List found", m.pName(parameter.Id))
				continue
			}

			// check if id is valid in optionlist
			found := false
			for _, o := range optionlist.Options {
				if o.Id == newValue.CurrentOption {
					found = true
					break
				}
			}

			if !found {
				mlog.Errorln("Invalid operation index for", m.pName(parameter.Id))
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_UnknownID)
				continue
			}
		case *pb.ParameterValue_Cmd:
			if parameterConfig.ControlStyle == pb.ControlStyle_Normal {
				mlog.Errorf("Got Value with Type %T for %s, but it has ControlStyle Normal and needs a Value", newValue, m.pName(parameter.Id))
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}
			m.out <- &pb.Parameter{
				Id:    parameter.Id,
				Error: 0,
				Value: []*pb.ParameterValue{newParameterValue},
			}
			continue
		case *pb.ParameterValue_Binary:
			if parameterConfig.ValueType != pb.ValueType_Binary {
				mlog.Errorf("Got Value with Type %T for %s, but it needs %v", newValue, m.pName(parameter.Id), pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}

			mlog.Debugf("Got Set Binary: %v", newValue)
		case *pb.ParameterValue_OptionListUpdate:
			mlog.Debugf("Got Set Option List: %v", newValue)

		}

		// Safe the momentary saved Value of the Parameter in the state

		if !parameterBuffer.currentEquals(newParameterValue) {
			if parameterConfig.ValueType != pb.ValueType_PNG && parameterConfig.ValueType != pb.ValueType_JPEG {
				mlog.Debugf("Set new TargetValue '%v', for %s, Device: %v", newParameterValue.Value, m.pName(parameter.Id), deviceID)
			}
			parameterBuffer.targetValue = proto.Clone(newParameterValue).(*pb.ParameterValue)
			parameterBuffer.isAssumedState.Store(true)
			parameterBuffer.tryCount = 0
		} else {
			if parameterConfig.ValueType != pb.ValueType_PNG && parameterConfig.ValueType != pb.ValueType_JPEG {
				mlog.Debugf("TargetValue %v is equal to CurrentValue", newParameterValue.Value)
			}
		}
		addr := paramDimensionAddress{
			parameter:   parameterID,
			device:      deviceID,
			dimensionID: parameterBuffer.getParameterValue().DimensionID,
		}
		m.reEvaluate(addr) // Trigger processing of the main evaluation
	}
}

func containsString(value string, slice []string) bool {
	for _, sValue := range slice {
		if value == sValue {
			return true
		}
	}
	return false
}
