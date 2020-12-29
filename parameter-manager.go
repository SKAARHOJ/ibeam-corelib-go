package ibeam_corelib

import (
	"net"
	"reflect"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
	"google.golang.org/grpc"
)

// IbeamParameterManager manages parameter changes.
type IbeamParameterManager struct {
	parameterRegistry   *IbeamParameterRegistry
	out                 chan pb.Parameter
	in                  chan pb.Parameter
	clientsSetterStream chan pb.Parameter
	serverClientsStream chan pb.Parameter
	server              *IbeamServer
}

// StartWithServer Starts the ibeam parameter routine and the GRPC server in one call. This is blocking and should be called at the end of main.
// The network must be "tcp", "tcp4", "tcp6", "unix" or "unixpacket".
func (m *IbeamParameterManager) StartWithServer(network, address string) {
	// Start parameter management routine
	m.Start()

	lis, err := net.Listen(network, address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterIbeamCoreServer(grpcServer, m.server)
	grpcServer.Serve(lis)
}

func (m *IbeamParameterManager) checkValidParameter(parameter *pb.Parameter) *pb.Parameter {
	// Check if given Parameter has an DeviceParameterID
	if parameter.Id == nil {
		// Client sees what he has send
		parameter.Error = pb.ParameterError_UnknownID
		log.Errorf("Given Parameter %v has no ID", parameter)

		return &pb.Parameter{
			Id:    parameter.Id,
			Error: pb.ParameterError_UnknownID,
			Value: []*pb.ParameterValue{},
		}
	}

	// Get Index and ID for Device and Parameter and the actual state of all parameters
	parameterID := parameter.Id.Parameter
	parameterIndex := int(parameterID)
	deviceID := parameter.Id.Device
	deviceIndex := int(deviceID - 1)
	modelIndex := m.parameterRegistry.getModelIndex(deviceID)

	// Get State and the Configuration (Details) of the Parameter, assume mutex is locked in outer layers of parameterLoop
	state := m.parameterRegistry.parameterValue
	parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]

	// Check if ID and Index are valid and in the State
	if _, ok := state[deviceIndex][parameterIndex]; !ok {
		log.Errorf("Invalid DeviceID %d for Parameter %d", deviceID, parameterID)

		return &pb.Parameter{
			Id:    parameter.Id,
			Error: pb.ParameterError_UnknownID,
			Value: []*pb.ParameterValue{},
		}
	}

	// Check if the configured type of the Parameter has a value
	if parameterConfig.ValueType == pb.ValueType_NoValue && parameterConfig.ControlStyle == pb.ControlStyle_NoControl {
		log.Errorf("Want to set Parameter with ID %v (%v), but it is configured as Type NoValue with no Control", parameterID, parameterConfig.Name)
		return &pb.Parameter{
			Id:    parameter.Id,
			Error: pb.ParameterError_HasNoValue,
			Value: []*pb.ParameterValue{},
		}
	}

	if parameterConfig.ValueType == pb.ValueType_NoValue && parameterConfig.ControlStyle == pb.ControlStyle_Oneshot {
		if cmd, ok := parameter.Value[0].Value.(*pb.ParameterValue_Cmd); ok {
			if cmd.Cmd != pb.Command_Trigger {
				log.Errorf("Want to set Parameter with ID %v (%v), but it is configured as Type NoValue with ControlStyle OneShot. Accept only Command:Trigger", parameterID, parameterConfig.Name)
				return &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
			}
		} else {
			log.Errorf("Want to set Parameter with ID %v (%v), but it is configured as Type NoValue with ControlStyle OneShot. Accept only Command:Trigger", parameterID, parameterConfig.Name)
			return &pb.Parameter{
				Id:    parameter.Id,
				Error: pb.ParameterError_InvalidType,
				Value: []*pb.ParameterValue{},
			}
		}
	}

	return nil
}

// Start the communication between client and server.
func (m *IbeamParameterManager) Start() {
	go func() {
		for {
			// ***************
			//  ClientToManagerLoop, inputs change request from GRPC SET to to manager
			// ***************
		clientToManagerLoop:
			for {
				var parameter pb.Parameter
				select {
				case parameter = <-m.clientsSetterStream:
					m.ingestTargetParameter(&parameter)
				default:
					break clientToManagerLoop
				}
			}

			// ***************
			//    Param Ingest loop, inputs changes from a device (f.e. a camera) to manager
			// ***************

		deviceToManagerLoop:
			for {
				var parameter pb.Parameter
				select {
				case parameter = <-m.in:
					m.ingestCurrentParameter(&parameter)
				default:
					break deviceToManagerLoop
				}
			}

			// ***************
			//    Main Parameter Loop, evaluates all parameter value buffers and sends out necessary changes
			// ***************

			m.parameterLoop()
			time.Sleep(time.Microsecond * 800)
		}
	}()
}

func (m *IbeamParameterManager) ingestTargetParameter(parameter *pb.Parameter) {
	if errorParam := m.checkValidParameter(parameter); errorParam != nil {
		m.serverClientsStream <- *errorParam
		return
	}

	// Get Index and ID for Device and Parameter and the actual state of all parameters
	parameterID := parameter.Id.Parameter
	parameterIndex := int(parameterID)
	deviceID := parameter.Id.Device
	deviceIndex := int(deviceID - 1)
	modelIndex := m.parameterRegistry.getModelIndex(deviceID)

	// Get State and the Configuration (Details) of the Parameter
	m.parameterRegistry.muValue.Lock()
	defer m.parameterRegistry.muValue.Unlock()
	state := m.parameterRegistry.parameterValue

	m.parameterRegistry.muDetail.RLock()
	defer m.parameterRegistry.muDetail.RUnlock()
	parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]

	// Handle every Value in that was given for the Parameter
	for _, newParameterValue := range parameter.Value {
		// Check if the NewValue has a Value
		if newParameterValue.Value == nil {
			log.Errorf("Received no value for parameter %d on device %d", parameterID, parameter.Id.Device)
			m.serverClientsStream <- pb.Parameter{
				Id:    parameter.Id,
				Error: pb.ParameterError_HasNoValue,
				Value: []*pb.ParameterValue{},
			}
			continue
		}

		// Check if dimension of the value is valid
		if !state[deviceIndex][parameterIndex].MultiIndexHasValue(newParameterValue.DimensionID) {
			log.Errorf("Received invalid Dimension %d for parameter %d on device %d", newParameterValue.DimensionID, parameterID, parameter.Id.Device)
			m.serverClientsStream <- pb.Parameter{
				Id:    parameter.Id,
				Error: pb.ParameterError_UnknownID,
				Value: []*pb.ParameterValue{},
			}
			continue
		}
		dimension, err := state[deviceIndex][parameterIndex].MultiIndex(newParameterValue.DimensionID)
		if err != nil {
			log.Error(err)
			continue
		}
		parameterBuffer, err := dimension.Value()
		if err != nil {
			log.Error(err)
			continue
		}

		// Check if Value is valid and has the right Type
		switch newValue := newParameterValue.Value.(type) {
		case *pb.ParameterValue_Integer:
			if parameterConfig.ValueType != pb.ValueType_Integer {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
				continue
			}

			if newValue.Integer > int32(parameterConfig.Maximum) {
				log.Errorf("Max violation for parameter %v", parameterID)
				m.serverClientsStream <- pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_MaxViolation,
					Value: []*pb.ParameterValue{},
				}
				continue
			}
			if newValue.Integer < int32(parameterConfig.Minimum) {
				log.Errorf("Min violation for parameter %v", parameterID)
				m.serverClientsStream <- pb.Parameter{
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
				m.serverClientsStream <- pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
				continue
			}

			if newValue.IncDecSteps > parameterConfig.IncDecStepsUpperRange || newValue.IncDecSteps < parameterConfig.IncDecStepsLowerRange {
				log.Errorf("In- or Decrementation Step %v is outside of the range [%v,%v] of the parameter %v", newValue.IncDecSteps, parameterConfig.IncDecStepsLowerRange, parameterConfig.IncDecStepsUpperRange, parameterID)
				m.serverClientsStream <- pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_RangeViolation,
					Value: []*pb.ParameterValue{},
				}
				continue
			}

			if parameterConfig.ValueType == pb.ValueType_Integer {
				newIntVal := parameterBuffer.targetValue.GetInteger() + newValue.IncDecSteps
				log.Infof("Decrement %d by %d", parameterBuffer.targetValue.GetInteger(), newValue.IncDecSteps)
				if newIntVal <= int32(parameterConfig.Maximum) && newIntVal >= int32(parameterConfig.Minimum) {
					parameterBuffer.targetValue.Value = &pb.ParameterValue_Integer{Integer: newIntVal}
					parameterBuffer.targetValue.Invalid = false
					if parameterConfig.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
						parameterBuffer.currentValue.Value = &pb.ParameterValue_Integer{Integer: newIntVal}
						parameterBuffer.isAssumedState = false
					}
					// send out right away
					m.serverClientsStream <- pb.Parameter{
						Value: []*pb.ParameterValue{parameterBuffer.getParameterValue()},
						Id: &pb.DeviceParameterID{
							Device:    deviceID,
							Parameter: parameterID,
						},
					}
					continue // make sure we skip the rest of the logic :-)
				}
			}

		case *pb.ParameterValue_Floating:
			if parameterConfig.ValueType != pb.ValueType_Floating {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
				continue
			}

			if newValue.Floating > parameterConfig.Maximum {
				log.Errorf("Max violation for parameter %v", parameterID)
				m.serverClientsStream <- pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_MaxViolation,
					Value: []*pb.ParameterValue{},
				}
				continue
			}
			if newValue.Floating < parameterConfig.Minimum {
				log.Errorf("Min violation for parameter %v", parameterID)
				m.serverClientsStream <- pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_MinViolation,
					Value: []*pb.ParameterValue{},
				}
				continue
			}
		case *pb.ParameterValue_Str:
			if parameterConfig.ValueType != pb.ValueType_String {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- pb.Parameter{
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
			if newValue.CurrentOption > uint32(len(parameterConfig.OptionList.Options)) {
				log.Errorf("Invalid operation index for parameter %v", parameterID)
				m.serverClientsStream <- pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_UnknownID,
					Value: []*pb.ParameterValue{},
				}
				continue
			}
		case *pb.ParameterValue_Cmd:
			if parameterConfig.ControlStyle == pb.ControlStyle_Normal {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it has ControlStyle Normal and needs a Value", newValue, parameterID, parameterConfig.Name)
				m.serverClientsStream <- pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
				continue
			}
			m.out <- pb.Parameter{
				Id:    parameter.Id,
				Error: 0,
				Value: []*pb.ParameterValue{newParameterValue},
			}
		case *pb.ParameterValue_Binary:

			if parameterConfig.ValueType != pb.ValueType_Binary {
				log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, pb.ValueType_name[int32(parameterConfig.ValueType)])
				m.serverClientsStream <- pb.Parameter{
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

		parameterBuffer.isAssumedState = !reflect.DeepEqual(newParameterValue.Value, parameterBuffer.currentValue.Value)
		parameterBuffer.targetValue = *newParameterValue
		parameterBuffer.tryCount = 0

		if parameterBuffer.isAssumedState {
			log.Debugf("Set new TargetValue '%v', for Parameter %v (%v), Device: %v", newParameterValue.Value, parameterID, parameterConfig.Name, deviceID)
		} else {
			log.Debugf("TargetValue %v is equal to CurrentValue", newParameterValue.Value)
		}

	}

}

func (m *IbeamParameterManager) ingestCurrentParameter(parameter *pb.Parameter) {
	if err := m.checkValidParameter(parameter); err != nil {
		log.Error(err)
		return
	}

	// Get Index and ID for Device and Parameter and the actual state of all parameters
	parameterID := parameter.Id.Parameter
	parameterIndex := int(parameterID)
	deviceID := parameter.Id.Device
	deviceIndex := int(deviceID - 1)
	modelIndex := m.parameterRegistry.getModelIndex(deviceID)

	// Get State and the Configuration (Details) of the Parameter
	m.parameterRegistry.muValue.Lock()
	defer m.parameterRegistry.muValue.Unlock()
	state := m.parameterRegistry.parameterValue
	m.parameterRegistry.muDetail.RLock()
	defer m.parameterRegistry.muDetail.RUnlock()
	parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]

	shouldSend := false
	for _, newParameterValue := range parameter.Value {
		if newParameterValue == nil {
			log.Warnf("Received nil value for parameter %d from device %d", parameterID, deviceID)
			continue
		}
		// Check if Dimension is Valid
		if !state[deviceIndex][parameterIndex].MultiIndexHasValue(newParameterValue.DimensionID) {
			log.Errorf("Received invalid dimension id  %v for parameter %d from device %d", newParameterValue.DimensionID, parameterID, deviceID)
			continue
		}

		parameterDimension, err := state[deviceIndex][parameterIndex].MultiIndex(newParameterValue.DimensionID)
		if err != nil {
			log.Error(err)
			continue
		}
		parameterBuffer, err := parameterDimension.Value()
		if err != nil {
			log.Error(err)
			continue
		}
		if newParameterValue.Value != nil {
			didSet := false // flag to to handle custom cases

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

					newValue := pb.ParameterValue{
						Value: &pb.ParameterValue_CurrentOption{
							CurrentOption: id,
						},
					}

					if time.Since(parameterBuffer.lastUpdate).Milliseconds() > int64(parameterConfig.QuarantineDelayMs) {
						if !reflect.DeepEqual(parameterBuffer.targetValue, newValue) {
							parameterBuffer.targetValue = newValue
							shouldSend = true
						}
					}

					if !reflect.DeepEqual(parameterBuffer.currentValue, newValue) {
						parameterBuffer.currentValue = newValue
						shouldSend = true
					}

					didSet = true

				case *pb.ParameterValue_OptionList:
					if !parameterConfig.OptionListIsDynamic {
						log.Errorf("Parameter with ID %v has no Dynamic OptionList", parameter.Id.Parameter)
						continue
					}
					m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex].OptionList = v.OptionList

					m.serverClientsStream <- pb.Parameter{
						Value: []*pb.ParameterValue{newParameterValue},
						Id:    parameter.Id,
						Error: pb.ParameterError_NoError,
					}
					continue
				case *pb.ParameterValue_CurrentOption:
					// Handled below
				default:
					log.Errorf("Valuetype of Parameter is Opt and so we should get a String or Opt or currentOpt, but got %T", newParameterValue)
					continue
				}
			case pb.ValueType_Binary:
				if _, ok := newParameterValue.Value.(*pb.ParameterValue_Binary); !ok {
					log.Errorf("Parameter with ID %v is Type Binary but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
					continue
				}
			case pb.ValueType_Floating:
				if _, ok := newParameterValue.Value.(*pb.ParameterValue_Floating); !ok {
					log.Errorf("Parameter with ID %v is Type Float but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
					continue
				}
			case pb.ValueType_Integer:
				if _, ok := newParameterValue.Value.(*pb.ParameterValue_Integer); !ok {
					log.Errorf("Parameter with ID %v is Type Integer but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
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

			if !didSet {
				if time.Since(parameterBuffer.lastUpdate).Milliseconds() > int64(parameterConfig.QuarantineDelayMs) {
					if !reflect.DeepEqual(parameterBuffer.targetValue, *newParameterValue) {
						parameterBuffer.targetValue = *newParameterValue
						shouldSend = true
					}
				}

				if !reflect.DeepEqual(parameterBuffer.currentValue, *newParameterValue) {
					parameterBuffer.currentValue = *newParameterValue
					shouldSend = true
				}
			}

			if parameterBuffer.isAssumedState != (parameterBuffer.currentValue.Value != parameterBuffer.targetValue.Value) {
				parameterBuffer.isAssumedState = parameterBuffer.currentValue.Value != parameterBuffer.targetValue.Value
				shouldSend = true
			}
		} else {
			if parameterBuffer.available != newParameterValue.Available {
				parameterBuffer.available = newParameterValue.Available
				shouldSend = true
			}

		}
	}

	if !shouldSend {
		return
	}

	if values := m.parameterRegistry.getInstanceValues(parameter.GetId()); values != nil {
		m.serverClientsStream <- pb.Parameter{Value: values, Id: parameter.Id, Error: 0}
	}
}

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
		deviceIndex := int(deviceID - 1)

		modelID := deviceInfo.ModelID
		modelIndex := int(modelID - 1)

		for _, parameterDetail := range m.parameterRegistry.ParameterDetail[modelIndex] {

			parameterID := parameterDetail.Id.Parameter
			parameterIndex := int(parameterID)

			// Check if Parameter has a Control Style
			if parameterDetail.ControlStyle == pb.ControlStyle_Undefined {
				continue
			}

			state := m.parameterRegistry.parameterValue
			parameterDimension := state[deviceIndex][parameterIndex]
			m.loopDimension(parameterDimension, parameterDetail, deviceID)
		}
	}
}

func (m *IbeamParameterManager) loopDimension(parameterDimension *IbeamParameterDimension, parameterDetail *pb.ParameterDetail, deviceID uint32) {
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
		log.Errorf("ParameterDimension is Value but returns no Value: ", err.Error())
		return
	}

	// ********************************************************************
	// First Basic Check Pipeline if the Parameter Value can be send to out
	// ********************************************************************

	// Old:if parameterBuffer.currentValue.Value == parameterBuffer.targetValue.Value
	if reflect.DeepEqual(parameterBuffer.currentValue.Value, parameterBuffer.targetValue.Value) {
		return
	}

	// Is is send after Control Delay time
	if parameterDetail.ControlDelayMs != 0 && time.Until(parameterBuffer.lastUpdate).Milliseconds()*-1 < int64(parameterDetail.ControlDelayMs) {
		//log.Infof("Failed to set parameter cause control delay time %v", time.Until(parameterBuffer.lastUpdate).Milliseconds()*-1)
		return
	}

	// Is the Retry Limit reached
	if parameterDetail.RetryCount != 0 && parameterDetail.FeedbackStyle != pb.FeedbackStyle_NoFeedback {
		parameterBuffer.tryCount++
		if parameterBuffer.tryCount > parameterDetail.RetryCount {
			log.Errorf("Failed to set parameter %v '%v' in %v tries on device %v", parameterDetail.Id.Parameter, parameterDetail.Name, parameterDetail.RetryCount, deviceID+1)
			parameterBuffer.targetValue = parameterBuffer.currentValue
			parameterBuffer.isAssumedState = false

			m.serverClientsStream <- pb.Parameter{
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
		parameterBuffer.isAssumedState = true

		if parameterDetail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
			parameterBuffer.currentValue = parameterBuffer.targetValue
			parameterBuffer.isAssumedState = false
		}

		// If we Have a current Option, get the Value for the option from the Option List
		if parameterDetail.ValueType == pb.ValueType_Opt {
			if value, ok := parameterBuffer.targetValue.Value.(*pb.ParameterValue_CurrentOption); ok {
				name, err := getElementNameFromOptionListByID(parameterDetail.OptionList, *value)
				if err != nil {
					log.Error(err)
					return
				}
				parameterBuffer.targetValue.Value = &pb.ParameterValue_Str{Str: name}
			}
		}

		m.out <- pb.Parameter{
			Value: []*pb.ParameterValue{parameterBuffer.getParameterValue()},
			Id: &pb.DeviceParameterID{
				Device:    deviceID,
				Parameter: parameterDetail.Id.Parameter,
			},
		}

		if parameterDetail.FeedbackStyle == pb.FeedbackStyle_DelayedFeedback ||
			parameterDetail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
			// send out assumed value immediately
			m.serverClientsStream <- pb.Parameter{
				Value: []*pb.ParameterValue{parameterBuffer.getParameterValue()},
				Id: &pb.DeviceParameterID{
					Device:    deviceID,
					Parameter: parameterDetail.Id.Parameter,
				},
			}
		}
	case pb.ControlStyle_Incremental:
		m.out <- pb.Parameter{
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

		m.out <- pb.Parameter{
			Id: &pb.DeviceParameterID{
				Device:    deviceID,
				Parameter: parameterDetail.Id.Parameter,
			},
			Error: pb.ParameterError_NoError,
			Value: cmdValue,
		}
	case pb.ControlStyle_NoControl, pb.ControlStyle_Oneshot:
		// Do Nothing
	default:
		log.Errorf("Could not match controlltype")
		return
	}
}
