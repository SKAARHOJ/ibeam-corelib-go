package ibeamcorelib

import (
	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	b "github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers"
	log "github.com/s00500/env_logger"
	"google.golang.org/protobuf/proto"
)

func (m *IBeamParameterManager) ingestTargetParameter(parameter *pb.Parameter) {
	if errorParam := m.checkValidParameter(parameter); errorParam != nil {
		m.serverClientsStream <- errorParam
		return
	}

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

	// Handle every Value in that was given for the Parameter
valueLoop:
	for _, newParameterValue := range parameter.Value {
		// Check if the NewValue has a Value
		if newParameterValue.Value == nil {
			log.Errorf("Received no value for parameter %d on device %d", parameterID, parameter.Id.Device)
			m.serverClientsStream <- &pb.Parameter{
				Id:    parameter.Id,
				Error: pb.ParameterError_HasNoValue,
				Value: []*pb.ParameterValue{},
			}
			continue
		}

		// Check if dimension of the value is valid
		if !state[deviceID][parameterID].multiIndexHasValue(newParameterValue.DimensionID) {
			log.Errorf("Received invalid Dimension %d for parameter %d on device %d", newParameterValue.DimensionID, parameterID, parameter.Id.Device)
			m.serverClientsStream <- &pb.Parameter{
				Id:    parameter.Id,
				Error: pb.ParameterError_UnknownID,
				Value: []*pb.ParameterValue{},
			}
			continue
		}
		dimension, err := state[deviceID][parameterID].multiIndex(newParameterValue.DimensionID)
		if err != nil {
			log.Error(err)
			continue
		}
		parameterBuffer, err := dimension.getValue()
		if err != nil {
			log.Errorf("Could not get value for dimension id %v,: %v", newParameterValue.DimensionID, err)
			continue
		}

		if !dimension.value.available {
			log.Errorf("Ingest Target Loop: Unavailable for parameter %v, DimensionID: %v", parameterID, newParameterValue.DimensionID)
			m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_Unavailable)
			continue
		}

		minimum := parameterConfig.Minimum
		maximum := parameterConfig.Maximum
		if parameterConfig.MinMaxIsDynamic {
			if parameterBuffer.dynamicMin != nil {
				minimum = *parameterBuffer.dynamicMin
			}
			if parameterBuffer.dynamicMin != nil {
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
					log.Warnf("Received undefined meta value called %s for parameter id %v", name, parameter.Id)
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
						log.Errorf("MaxViolation for parameter meta value %s for parameter id %v : %f > %f", name, parameter.Id, v, mConfig.Maximum)
						m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MaxViolation)
						continue valueLoop
					} else if v < mConfig.Minimum {
						log.Errorf("MinViolation for parameter meta value %s for parameter id %v : %f < %f", name, parameter.Id, v, mConfig.Minimum)
						m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MinViolation)
						continue valueLoop
					}
				}

				if mConfig.MetaType == pb.ParameterMetaType_MetaOption {
					if !containsString(mValue.GetStr(), mConfig.Options) {
						log.Errorf("Received undefined meta option value called %s for metavalue %s for parameter id %v", mValue.GetStr(), name, parameter.Id)
						m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_UnknownID)
						continue valueLoop
					}
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
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}

			if newValue.Integer > int32(maximum) {
				if !isDescreteValue(parameterConfig, float64(newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)) {
					log.Errorf("Ingest Target Loop: Max violation for parameter %v, %d", parameterID)
					m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MaxViolation)
					continue
				}
			}
			if newValue.Integer < int32(minimum) {
				if !isDescreteValue(parameterConfig, float64(newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)) {
					log.Errorf("Ingest Target Loop: Min violation for parameter %v, %d", parameterID, newValue.Integer)
					m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MinViolation)
					continue
				}
			}
		case *pb.ParameterValue_IncDecSteps:
			// inc dec currently only works with integers or no values, float is kind of missing, action lists need to be evaluated
			if parameterConfig.ValueType != pb.ValueType_Integer && parameterConfig.ValueType == pb.ValueType_NoValue {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}

			if newValue.IncDecSteps > parameterConfig.IncDecStepsUpperLimit || newValue.IncDecSteps < parameterConfig.IncDecStepsLowerLimit {
				log.Errorf("In- or Decrementation Step %v is outside of limits [%v,%v] of the parameter %v", newValue.IncDecSteps, parameterConfig.IncDecStepsLowerLimit, parameterConfig.IncDecStepsUpperLimit, parameterID)
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_StepSizeViolation)
				continue
			}

			if parameterConfig.ValueType == pb.ValueType_Integer {
				newIntVal := parameterBuffer.targetValue.GetInteger() + newValue.IncDecSteps
				log.Tracef("Decrement %d by %d", parameterBuffer.targetValue.GetInteger(), newValue.IncDecSteps)
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

		case *pb.ParameterValue_Floating:
			if parameterConfig.ValueType != pb.ValueType_Floating {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}

			if newValue.Floating > maximum {
				if !isDescreteValue(parameterConfig, newParameterValue.Value.(*pb.ParameterValue_Floating).Floating) {
					log.Errorf("Max violation for parameter %v", parameterID)
					m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MaxViolation)
					continue
				}
			}
			if newValue.Floating < minimum {
				if !isDescreteValue(parameterConfig, newParameterValue.Value.(*pb.ParameterValue_Floating).Floating) {
					log.Errorf("Min violation for parameter %v", parameterID)
					m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_MinViolation)
					continue
				}
			}
		case *pb.ParameterValue_Str:
			if parameterConfig.ValueType != pb.ValueType_String {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}

			// String does not need extra check

		case *pb.ParameterValue_CurrentOption:

			if optionlist == nil {
				log.Errorf("No option List found for Parameter %v", newValue)
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
				log.Errorf("Invalid operation index for parameter %v", parameterID)
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_UnknownID)
				continue
			}
		case *pb.ParameterValue_Cmd:
			if parameterConfig.ControlStyle == pb.ControlStyle_Normal {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it has ControlStyle Normal and needs a Value", newValue, parameterID, parameterConfig.Name)
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}
			m.out <- &pb.Parameter{
				Id:    parameter.Id,
				Error: 0,
				Value: []*pb.ParameterValue{newParameterValue},
			}
		case *pb.ParameterValue_Binary:

			if parameterConfig.ValueType != pb.ValueType_Binary {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- paramError(parameterID, deviceID, pb.ParameterError_InvalidType)
				continue
			}

			log.Debugf("Got Set Binary: %v", newValue)
		case *pb.ParameterValue_OptionListUpdate:
			log.Debugf("Got Set Option List: %v", newValue)

		}

		// Safe the momentary saved Value of the Parameter in the state

		if !parameterBuffer.currentEquals(newParameterValue) {
			if parameterConfig.ValueType != pb.ValueType_PNG && parameterConfig.ValueType != pb.ValueType_JPEG {
				log.Debugf("Set new TargetValue '%v', for Parameter %v (%v), Device: %v", newParameterValue.Value, parameterID, parameterConfig.Name, deviceID)
			}
			parameterBuffer.targetValue = proto.Clone(newParameterValue).(*pb.ParameterValue)
			parameterBuffer.isAssumedState = true
			parameterBuffer.tryCount = 0
		} else {
			if parameterConfig.ValueType != pb.ValueType_PNG && parameterConfig.ValueType != pb.ValueType_JPEG {
				log.Debugf("TargetValue %v is equal to CurrentValue", newParameterValue.Value)
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
