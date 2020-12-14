package ibeam_corelib

import (
	"fmt"
	"net"
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

// StartWithServer Starts the ibeam parameter routine and the GRPC server in one call. This is blocking and should be called at the end of main
func (m *IbeamParameterManager) StartWithServer(endPoint string) {
	// Start parameter management routine
	m.Start() // TODO: we could use a waitgroup or something here ? cross goroutine error handling is kinda missing

	lis, err := net.Listen("tcp", endPoint) // TODO: make listen: unix also possible!
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterIbeamCoreServer(grpcServer, m.server)
	grpcServer.Serve(lis)
}

// Start the communication between client and server.
func (m *IbeamParameterManager) Start() {
	go func() {
		for {

			var state parameterStates
			var parameterConfig *pb.ParameterDetail

			var parameter pb.Parameter
			var parameterID uint32
			var parameterIndex int
			var deviceID uint32
			var deviceIndex int
			var modelIndex int

			var set = func() error {

				// Check if given Parameter has an DeviceParameterID
				if parameter.Id == nil {
					// TODO: check if this isn´t better for debug on client side.
					// Client sees what he has send
					parameter.Error = pb.ParameterError_UnknownID
					m.serverClientsStream <- parameter
					/*
						m.serverClientsStream <- pb.Parameter{
							Id:    parameter.Id,
							Error: pb.ParameterError_UnknownID,
							Value: []*pb.ParameterValue{},
						}*/
					return fmt.Errorf("Given Parameter %v has no ID", parameter)
				}

				// Get Index and ID for Device and Parameter and the actual state of all parameters
				parameterID = parameter.Id.Parameter
				parameterIndex = int(parameterID)
				deviceID = parameter.Id.Device
				deviceIndex = int(deviceID - 1)
				modelIndex = m.parameterRegistry.getModelIndex(deviceID)

				// Get State and the Configuration (Details) of the Parameter
				m.parameterRegistry.muValue.RLock()
				state = m.parameterRegistry.parameterValue
				m.parameterRegistry.muValue.RUnlock()

				m.parameterRegistry.muDetail.RLock()
				parameterConfig = m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]
				m.parameterRegistry.muDetail.RUnlock()

				// Check if ID and Index are valid and in the State
				if _, ok := state[deviceIndex][parameterIndex]; !ok {
					m.serverClientsStream <- pb.Parameter{
						Id:    parameter.Id,
						Error: pb.ParameterError_UnknownID,
						Value: []*pb.ParameterValue{},
					}
					return fmt.Errorf("Invalid DeviceID %d for Parameter %d", deviceID, parameterID)
				}

				// Check if the configured type of the Parameter has a value
				if parameterConfig.ValueType == pb.ValueType_NoValue {
					m.serverClientsStream <- pb.Parameter{
						Id:    parameter.Id,
						Error: pb.ParameterError_HasNoValue,
						Value: []*pb.ParameterValue{},
					}
					return fmt.Errorf("Want to set Parameter with ID %v (%v), but it is configured as Type NoValue", parameterID, parameterConfig.Name)
				}

				return nil
			}

			for parameter = range m.clientsSetterStream {
				// Client set loop, inputs set requests from grpc to manager

				if err := set(); err != nil {
					log.Error(err)
					continue
				}

				// Handle every Value in that was given for the Parameter
				for _, newParameterValue := range parameter.Value {
					// Check if the NewValue has a Value
					if newParameterValue.Value == nil {
						// TODO: should this be allowed or will we send an error back?
						continue
					}

					// Check if Instance of the Value is valid
					//FIXME:
					/*
						if len(state[deviceIndex][parameterIndex]) < int(dimensionID) {
							log.Errorf("Received invalid Dimension %d for parameter %d on device %d", dimensionID, parameterID, parameter.Id.Device)
							m.serverClientsStream <- pb.Parameter{
								Id:    parameter.Id,
								Error: pb.ParameterError_UnknownID,
								Value: []*pb.ParameterValue{},
							}
							continue
						}*/

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
						if parameterConfig.ValueType != pb.ValueType_Integer {
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
						// Command can be send directly to the output
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
						//TODO: check if Option list is valid (unique ids etc.)
					}

					// Safe the momentary saved Value of the Parameter in the state
					parameterDimension := state[deviceIndex][parameterIndex]
					parameterBuffer, err := parameterDimension.multiIndex(newParameterValue.DimensionID).Value()
					if err != nil {
						log.Errorf("Trying to get stuff")
						parameterSubdimensions, err := parameterDimension.Subdimensions()
						if err != nil {
							log.Errorf("Parameter %v has no Value and no SubDimension", parameterID)
							continue
						}
						for _, parameterSubdimension := range parameterSubdimensions {
							parameterSubdimension.Value()

						}
					}

					log.Debugf("Set new TargetValue '%v', for Parameter %v (%v)", newParameterValue.Value, parameterID, parameterConfig.Name)

					log.Infof("New val: %v, Buffer: %v ", newParameterValue, parameterBuffer)
					parameterBuffer.isAssumedState = newParameterValue.Value != parameterBuffer.currentValue.Value
					parameterBuffer.targetValue = *newParameterValue

					parameterBuffer.tryCount = 0

				}
			}
			for parameter = range m.in {
				// Param Ingest loop, inputs changes from a device (f.e. a camera) to manager
				if err := set(); err != nil {
					log.Error(err)
					continue
				}
				shouldSend := false
				for _, newParameterValue := range parameter.Value {
					// Check if Dimension is Valid
					// FIXME:
					/*
						if len(state[deviceIndex][parameterIndex]) < int(dimensionID) {
							log.Errorf("Received invalid dimension id %v for parameter %v", dimensionID, parameterID)
							continue
						}*/

					parameterDimension := state[deviceIndex][parameterIndex]
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
									// TODO:get new option list maybe
									continue
								}

								newValue := pb.ParameterValue{
									Value: &pb.ParameterValue_CurrentOption{
										CurrentOption: id,
									},
								}
								if time.Since(parameterBuffer.lastUpdate).Milliseconds() > int64(parameterConfig.QuarantineDelayMs) {
									parameterBuffer.targetValue = newValue
								}
								parameterBuffer.currentValue = newValue

								didSet = true

							case *pb.ParameterValue_OptionList:
								if !parameterConfig.OptionListIsDynamic {
									log.Errorf("Parameter with ID %v has no Dynamic OptionList", parameter.Id.Parameter)
									continue
								}
								m.parameterRegistry.muDetail.Lock()
								m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex].OptionList = v.OptionList
								m.parameterRegistry.muDetail.Unlock()

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
								parameterBuffer.targetValue = *newParameterValue
							}
							parameterBuffer.currentValue = *newParameterValue
						}

						parameterBuffer.isAssumedState = parameterBuffer.currentValue.Value != parameterBuffer.targetValue.Value
					} else {
						parameterBuffer.available = newParameterValue.Available
					}
					shouldSend = true
				}

				if !shouldSend {
					continue
				}

				if values := m.parameterRegistry.getInstanceValues(*parameter.GetId()); values != nil {
					m.serverClientsStream <- pb.Parameter{Value: values, Id: parameter.Id, Error: 0}
				}
			}
			// ***************
			//    Main Loop
			// ***************

			m.loop()
		}
	}()
}

func (m *IbeamParameterManager) loop() {
	m.parameterRegistry.muValue.RLock()
	state := m.parameterRegistry.parameterValue
	m.parameterRegistry.muValue.RUnlock()

	m.parameterRegistry.muInfo.RLock()

	// Range over all Devices
	for _, deviceInfo := range m.parameterRegistry.DeviceInfos {

		deviceID := deviceInfo.DeviceID
		deviceIndex := int(deviceID - 1)

		modelID := deviceInfo.ModelID
		modelIndex := int(modelID - 1)

		m.parameterRegistry.muDetail.RLock()
		for _, parameterDetail := range m.parameterRegistry.ParameterDetail[modelIndex] {

			parameterID := parameterDetail.Id.Parameter
			parameterIndex := int(parameterID)

			// Check if Parameter has a Control Style
			if parameterDetail.ControlStyle == pb.ControlStyle_Undefined {
				continue
			}

			parameterDimension := state[deviceIndex][parameterIndex]
			m.loopDimension(parameterDimension, parameterDetail, deviceID)
		}
	}
}

func (m *IbeamParameterManager) loopDimension(parameterDimension *IbeamParameterDimension, parameterDetail *pb.ParameterDetail, deviceID uint32) {
	if parameterDimension.isValue() {
		parameterBuffer, err := parameterDimension.Value()
		if err != nil {
			log.Errorf("ParameterDimension is Value but returns no Value: ", err.Error())
			return
		}
		// ********************************************************************
		// First Basic Check Pipeline if the Parameter Value can be send to out
		// ********************************************************************

		// Is the Value the Same like stored?

		// TODO:ask how to
		newMetaValues := false
		/*
			for _, targetMetaValue := range parameterBuffer.targetValue.MetaValues {
				for _, currentMetaValue := range parameterBuffer.currentValue.MetaValues {
					if targetMetaValue. == currentMetaValue {

					}
				}
			}*/

		if parameterBuffer.currentValue.Value == parameterBuffer.targetValue.Value ||
			newMetaValues {
			//log.Errorf("Failed to set parameter %v '%v' for device %v, CurrentValue '%v' and TargetValue '%v' are the same", parameterConfig.Id.Parameter, parameterConfig.Name, deviceID+1, parameterBuffer.currentValue.Value, parameterBuffer.targetValue.Value)
			return
		}

		// Is is send after Control Delay time
		if time.Until(parameterBuffer.lastUpdate).Milliseconds() > int64(parameterDetail.ControlDelayMs) {
			//log.Errorf("Failed to set parameter %v '%v' for device %v, ControlDelayTime", parameterConfig.Id.Parameter, parameterConfig.Name, deviceID+1, parameterConfig.ControlDelayMs)
			return
		}

		// Is is send after Quarantine Delay time
		if time.Until(parameterBuffer.lastUpdate).Milliseconds() > int64(parameterDetail.QuarantineDelayMs) {
			//log.Errorf("Failed to set parameter %v '%v' for device %v, QuarantineDelayTime", parameterConfig.Id.Parameter, parameterConfig.Name, deviceID+1, parameterConfig.QuarantineDelayMs)
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
			/* TODO
			case pb.ControlStyle_IncrementalWithSteps:

				m.out <- pb.Parameter{
					Value: []*pb.ParameterValue{parameterBuffer.getParameterValue()},
					Id: &pb.DeviceParameterID{
						Device:    deviceID,
						Parameter: parameterDetail.Id.Parameter,
					},
				}
			*/
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
				Error: 0,
				Value: cmdValue,
			}
		case pb.ControlStyle_NoControl, pb.ControlStyle_Incremental, pb.ControlStyle_Oneshot:
			// Do Nothing
		default:
			log.Errorf("Could not match controlltype")
			return
		}
	} else {
		subdimensions, err := parameterDimension.Subdimensions()
		if err != nil {
			log.Errorf("Dimension is No Value, but also have no Subdimension: %v", err.Error())
			return
		}
		for _, parameterDimension = range subdimensions {
			m.loopDimension(parameterDimension, parameterDetail, deviceID)
		}
	}
}
