package ibeamcorelib

import (
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
	"google.golang.org/protobuf/proto"
)

func (m *IbeamParameterManager) parameterLoop() {

	m.parameterRegistry.muInfo.RLock()
	defer m.parameterRegistry.muInfo.RUnlock()

	m.parameterRegistry.muDetail.RLock()
	defer m.parameterRegistry.muDetail.RUnlock()

	m.parameterRegistry.muValue.Lock()
	defer m.parameterRegistry.muValue.Unlock()

	// Range over all Devices
	for _, deviceInfo := range m.parameterRegistry.DeviceInfos {

		deviceID := deviceInfo.DeviceID
		modelID := deviceInfo.ModelID

		for _, parameterDetail := range m.parameterRegistry.ParameterDetail[modelID] {
			parameterID := parameterDetail.Id.Parameter
			parameterIndex := parameterID

			// Check if Parameter has a Control Style
			if parameterDetail.ControlStyle == pb.ControlStyle_Undefined {
				continue
			}

			state := m.parameterRegistry.ParameterValue
			parameterDimension := state[deviceID][parameterIndex]
			m.loopDimension(parameterDimension, parameterDetail, deviceID)
		}
	}
}

func (m *IbeamParameterManager) loopDimension(parameterDimension *IBeamParameterDimension, parameterDetail *pb.ParameterDetail, deviceID uint32) {
	if !parameterDimension.isValue() {
		subdimensions, err := parameterDimension.Subdimensions()
		if err != nil {
			log.Errorf("Dimension is No Value, but also have no Subdimension: %v", err.Error())
			return
		}
		for _, parameterDimension = range subdimensions {
			m.loopDimension(parameterDimension, parameterDetail, deviceID)
		}
		return
	}

	parameterBuffer, err := parameterDimension.Value()
	if err != nil {
		log.Errorln("ParameterDimension is Value but returns no Value: ", err.Error())
		return
	}

	// ********************************************************************
	// First Basic Check Pipeline if the Parameter Value can be send to out
	// ********************************************************************

	if proto.Equal(parameterBuffer.currentValue, parameterBuffer.targetValue) {
		return
	}

	// Is is send after Control Delay time
	if parameterDetail.ControlDelayMs != 0 && time.Since(parameterBuffer.lastUpdate).Milliseconds() < int64(parameterDetail.ControlDelayMs) {
		return
	}

	// Is the Retry Limit reached
	if parameterDetail.RetryCount != 0 && parameterDetail.FeedbackStyle != pb.FeedbackStyle_NoFeedback {
		parameterBuffer.tryCount++
		if parameterBuffer.tryCount > parameterDetail.RetryCount {
			log.Errorf("Failed to set parameter %v '%v' in %v tries on device %v", parameterDetail.Id.Parameter, parameterDetail.Name, parameterDetail.RetryCount, deviceID)
			parameterBuffer.targetValue = proto.Clone(parameterBuffer.currentValue).(*pb.ParameterValue)
			m.serverClientsStream <- &pb.Parameter{
				Value: []*pb.ParameterValue{parameterBuffer.getParameterValue()},
				Id: &pb.DeviceParameterID{
					Device:    deviceID,
					Parameter: parameterDetail.Id.Parameter,
				},
				Error: pb.ParameterError_MaxRetrys,
			}
			return
		}
	}

	// Set the lastUpdate Time
	parameterBuffer.lastUpdate = time.Now()

	switch parameterDetail.ControlStyle {
	case pb.ControlStyle_Normal:
		if parameterDetail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
			parameterBuffer.currentValue = proto.Clone(parameterBuffer.targetValue).(*pb.ParameterValue)
		}

		// If we Have a current Option, get the Value for the option from the Option List
		if parameterDetail.ValueType == pb.ValueType_Opt {
			if value, ok := parameterBuffer.targetValue.Value.(*pb.ParameterValue_CurrentOption); ok {
				name, err := getElementNameFromOptionListByID(parameterDetail.OptionList, *value)
				if err != nil {
					log.Error(err)
					return
				}
				//parameterBuffer.targetValue.Value = &pb.ParameterValue_Str{Str: name} // TODO: evaluate if that is ok
				_ = name
				parameterBuffer.targetValue.Value = &pb.ParameterValue_CurrentOption{CurrentOption: value.CurrentOption}
			}
		}

		m.out <- &pb.Parameter{
			Value: []*pb.ParameterValue{parameterBuffer.getParameterValue()},
			Id: &pb.DeviceParameterID{
				Device:    deviceID,
				Parameter: parameterDetail.Id.Parameter,
			},
		}

		if parameterDetail.FeedbackStyle == pb.FeedbackStyle_DelayedFeedback ||
			parameterDetail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
			// send out assumed value immediately
			m.serverClientsStream <- &pb.Parameter{
				Value: []*pb.ParameterValue{parameterBuffer.getParameterValue()},
				Id: &pb.DeviceParameterID{
					Device:    deviceID,
					Parameter: parameterDetail.Id.Parameter,
				},
			}
		}
	case pb.ControlStyle_Incremental:
		m.out <- &pb.Parameter{
			Value: []*pb.ParameterValue{parameterBuffer.getParameterValue()},
			Id: &pb.DeviceParameterID{
				Device:    deviceID,
				Parameter: parameterDetail.Id.Parameter,
			},
		}
	case pb.ControlStyle_ControlledIncremental:
		if parameterDetail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
			return
		}

		type action string
		const (
			// Increment ...
			Increment action = "Increment"
			// Decrement ...
			Decrement action = "Decrement"
			// NoOperation ...
			NoOperation action = "NoOperation"
		)

		var cmdAction action

		switch value := parameterBuffer.currentValue.Value.(type) {
		case *pb.ParameterValue_Integer:
			if targetValue, ok := parameterBuffer.targetValue.Value.(*pb.ParameterValue_Integer); ok {
				if value.Integer < targetValue.Integer {
					cmdAction = Increment
				} else if value.Integer > targetValue.Integer {
					cmdAction = Decrement
				} else {
					cmdAction = NoOperation
				}
			}
		case *pb.ParameterValue_Floating:
			if targetValue, ok := parameterBuffer.targetValue.Value.(*pb.ParameterValue_Floating); ok {
				if value.Floating < targetValue.Floating {
					cmdAction = Increment
				} else if value.Floating > targetValue.Floating {
					cmdAction = Decrement
				} else {
					cmdAction = NoOperation
				}
			}
		case *pb.ParameterValue_CurrentOption:
			if targetValue, ok := parameterBuffer.targetValue.Value.(*pb.ParameterValue_CurrentOption); ok {
				if value.CurrentOption < targetValue.CurrentOption {
					cmdAction = Increment
				} else if value.CurrentOption > targetValue.CurrentOption {
					cmdAction = Decrement
				} else {
					cmdAction = NoOperation
				}
			}
		default:
			log.Errorf("Could not match Valuetype %T", value)
		}

		var cmdValue []*pb.ParameterValue

		switch cmdAction {
		case Increment:
			cmdValue = append(cmdValue, parameterBuffer.incrementParameterValue())
		case Decrement:
			cmdValue = append(cmdValue, parameterBuffer.decrementParameterValue())
		case NoOperation:
			return
		}
		parameterBuffer.lastUpdate = time.Now()
		if parameterDetail.Id == nil {
			log.Errorf("No DeviceParameterID")
			return
		}

		log.Debugf("Send value '%v' to Device", cmdValue)

		m.out <- &pb.Parameter{
			Id: &pb.DeviceParameterID{
				Device:    deviceID,
				Parameter: parameterDetail.Id.Parameter,
			},
			Error: pb.ParameterError_NoError,
			Value: cmdValue,
		}
	case pb.ControlStyle_NoControl, pb.ControlStyle_Oneshot:
		if parameterDetail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
			parameterBuffer.targetValue = proto.Clone(parameterBuffer.currentValue).(*pb.ParameterValue)
		}

	default:
		log.Errorf("Could not match controlltype")
		return
	}
}
