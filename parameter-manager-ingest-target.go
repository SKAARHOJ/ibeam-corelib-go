package ibeamcorelib

import (
	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
	"google.golang.org/protobuf/proto"
)

func (m *IbeamParameterManager) ingestTargetParameter(parameter *pb.Parameter) {
	if errorParam := m.checkValidParameter(parameter); errorParam != nil {
		m.serverClientsStream <- errorParam
		return
	}

	// Get Index and ID for Device and Parameter and the actual state of all parameters
	parameterID := int(parameter.Id.Parameter)
	deviceID := parameter.Id.Device
	deviceIndex := int(deviceID - 1)
	modelIndex := m.parameterRegistry.getModelIndex(deviceID)

	// Get State and the Configuration (Details) of the Parameter
	m.parameterRegistry.muValue.Lock()
	defer m.parameterRegistry.muValue.Unlock()
	state := m.parameterRegistry.parameterValue

	m.parameterRegistry.muDetail.RLock()
	defer m.parameterRegistry.muDetail.RUnlock()
	parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterID]

	// Handle every Value in that was given for the Parameter
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
		if !state[deviceIndex][parameterID].MultiIndexHasValue(newParameterValue.DimensionID) {
			log.Errorf("Received invalid Dimension %d for parameter %d on device %d", newParameterValue.DimensionID, parameterID, parameter.Id.Device)
			m.serverClientsStream <- &pb.Parameter{
				Id:    parameter.Id,
				Error: pb.ParameterError_UnknownID,
				Value: []*pb.ParameterValue{},
			}
			continue
		}
		dimension, err := state[deviceIndex][parameterID].MultiIndex(newParameterValue.DimensionID)
		if err != nil {
			log.Error(err)
			continue
		}
		parameterBuffer, err := dimension.Value()
		if err != nil {
			log.Errorf("Could not get value for dimension id %v,: %v", newParameterValue.DimensionID, err)
			continue
		}

		// Check if Value is valid and has the right Type
		switch newValue := newParameterValue.Value.(type) {
		case *pb.ParameterValue_Integer:
			if parameterConfig.ValueType != pb.ValueType_Integer {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
				continue
			}

			if newValue.Integer > int32(parameterConfig.Maximum) {
				log.Errorf("Max violation for parameter %v", parameterID)
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_MaxViolation,
					Value: []*pb.ParameterValue{},
				}
				continue
			}
			if newValue.Integer < int32(parameterConfig.Minimum) {
				log.Errorf("Min violation for parameter %v", parameterID)
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_MinViolation,
					Value: []*pb.ParameterValue{},
				}
				continue
			}
		case *pb.ParameterValue_IncDecSteps:
			// inc dec currently only works with integers or no values, float is kind of missing, action lists need to be evaluated
			if parameterConfig.ValueType != pb.ValueType_Integer && parameterConfig.ValueType == pb.ValueType_NoValue {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
				continue
			}

			if newValue.IncDecSteps > parameterConfig.IncDecStepsUpperLimit || newValue.IncDecSteps < parameterConfig.IncDecStepsLowerLimit {
				log.Errorf("In- or Decrementation Step %v is outside of limits [%v,%v] of the parameter %v", newValue.IncDecSteps, parameterConfig.IncDecStepsLowerLimit, parameterConfig.IncDecStepsUpperLimit, parameterID)
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_StepSizeViolation,
					Value: []*pb.ParameterValue{},
				}
				continue
			}

			if parameterConfig.ValueType == pb.ValueType_Integer {
				newIntVal := parameterBuffer.targetValue.GetInteger() + newValue.IncDecSteps
				log.Tracef("Decrement %d by %d", parameterBuffer.targetValue.GetInteger(), newValue.IncDecSteps)
				if newIntVal <= int32(parameterConfig.Maximum) && newIntVal >= int32(parameterConfig.Minimum) {
					parameterBuffer.targetValue.Value = &pb.ParameterValue_Integer{Integer: newIntVal}
					parameterBuffer.targetValue.Invalid = false
					if parameterConfig.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
						parameterBuffer.currentValue.Value = &pb.ParameterValue_Integer{Integer: newIntVal}
					}
					// send out right away
					m.serverClientsStream <- &pb.Parameter{
						Value: []*pb.ParameterValue{parameterBuffer.getParameterValue()},
						Id: &pb.DeviceParameterID{
							Device:    deviceID,
							Parameter: uint32(parameterID),
						},
					}
					continue // make sure we skip the rest of the logic :-)
				}
			}

		case *pb.ParameterValue_Floating:
			if parameterConfig.ValueType != pb.ValueType_Floating {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
				continue
			}

			if newValue.Floating > parameterConfig.Maximum {
				log.Errorf("Max violation for parameter %v", parameterID)
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_MaxViolation,
					Value: []*pb.ParameterValue{},
				}
				continue
			}
			if newValue.Floating < parameterConfig.Minimum {
				log.Errorf("Min violation for parameter %v", parameterID)
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_MinViolation,
					Value: []*pb.ParameterValue{},
				}
				continue
			}
		case *pb.ParameterValue_Str:
			if parameterConfig.ValueType != pb.ValueType_String {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
				continue
			}

			// String does not need extra check

		case *pb.ParameterValue_CurrentOption:

			if parameterConfig.OptionList == nil {
				log.Errorf("No option List found for Parameter %v", newValue)
				continue
			}

			// check if id is valid in optionlist
			found := false
			for _, o := range parameterConfig.OptionList.Options {
				if o.Id == newValue.CurrentOption {
					found = true
					break
				}
			}

			if !found {
				log.Errorf("Invalid operation index for parameter %v", parameterID)
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_UnknownID,
					Value: []*pb.ParameterValue{},
				}
				continue
			}
		case *pb.ParameterValue_Cmd:
			if parameterConfig.ControlStyle == pb.ControlStyle_Normal {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it has ControlStyle Normal and needs a Value", newValue, parameterID, parameterConfig.Name)
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
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
				m.serverClientsStream <- &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
				continue
			}

			log.Debugf("Got Set Binary: %v", newValue)
		case *pb.ParameterValue_OptionList:
			log.Debugf("Got Set Option List: %v", newValue)

		}

		// Safe the momentary saved Value of the Parameter in the state

		if !proto.Equal(newParameterValue, parameterBuffer.currentValue) {
			log.Debugf("Set new TargetValue '%v', for Parameter %v (%v), Device: %v", newParameterValue.Value, parameterID, parameterConfig.Name, deviceID)
			parameterBuffer.targetValue = proto.Clone(newParameterValue).(*pb.ParameterValue)
			parameterBuffer.tryCount = 0

		} else {
			log.Debugf("TargetValue %v is equal to CurrentValue", newParameterValue.Value)
		}

	}

}
