package ibeam_core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/s00500/env_logger"
)

func init() {
	log.ConfigureDefaultLogger()
}

type IbeamParameterRegistry struct {
	muInfo          sync.RWMutex
	muDetail        sync.RWMutex
	muValue         sync.RWMutex
	coreInfo        CoreInfo
	DeviceInfos     []*DeviceInfo
	ModelInfos      []*ModelInfo
	ParameterDetail [][]*ParameterDetail             //Parameter Value: model, parameter
	parameterValue  [][][]*IBeamParameterValueBuffer //Parameter State: device,parameter,instance
}

type IbeamServer struct {
	parameterRegistry        *IbeamParameterRegistry
	clientsSetterStream      chan Parameter
	serverClientsStream      chan Parameter
	serverClientsDistributor map[chan Parameter]bool
}

type IbeamParameterManager struct {
	parameterRegistry   *IbeamParameterRegistry
	out                 chan Parameter
	in                  chan Parameter
	clientsSetterStream chan Parameter
	serverClientsStream chan Parameter
}

type IBeamParameterValueBuffer struct {
	instanceID     uint32
	available      bool
	isAssumedState bool
	lastUpdate     time.Time
	tryCount       uint32
	currentValue   ParameterValue
	targetValue    ParameterValue
}

func (b *IBeamParameterValueBuffer) getParameterValue() *ParameterValue {
	return &ParameterValue{
		InstanceID:     b.instanceID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState,
		Value:          b.targetValue.Value,
	}
}

func (b *IBeamParameterValueBuffer) incrementParameterValue() *ParameterValue {
	return &ParameterValue{
		InstanceID:     b.instanceID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState,
		Value: &ParameterValue_Cmd{
			Cmd: Command_Increment,
		},
	}
}
func (b *IBeamParameterValueBuffer) decrementParameterValue() *ParameterValue {
	return &ParameterValue{
		InstanceID:     b.instanceID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState,
		Value: &ParameterValue_Cmd{
			Cmd: Command_Decrement,
		},
	}
}

// device,parameter,instance
func (r *IbeamParameterRegistry) getInstanceValues(dpID DeviceParameterID) (values []*ParameterValue) {
	deviceIndex := int(dpID.Device) - 1
	parameterIndex := int(dpID.Parameter) - 1

	r.muValue.RLock()
	if dpID.Device == 0 || dpID.Parameter == 0 || len(r.parameterValue) <= deviceIndex || len(r.parameterValue[deviceIndex]) <= parameterIndex {
		log.Errorf("Could not get instance Values for DeviceParameterID: %v", dpID)
		r.muValue.RUnlock()
		return nil
	}
	for _, value := range r.parameterValue[deviceIndex][parameterIndex] {
		values = append(values, value.getParameterValue())
	}
	r.muValue.RUnlock()
	return
}

func (r *IbeamParameterRegistry) getModelIndex(deviceID int) uint32 {
	if len(r.DeviceInfos) <= deviceID {
		log.Panicf("Could not get model for device with id %v", deviceID)
	}
	return r.DeviceInfos[deviceID].ModelID //Todo was -1 before
}

func (s *IbeamServer) GetCoreInfo(_ context.Context, _ *Empty) (*CoreInfo, error) {
	return &s.parameterRegistry.coreInfo, nil
}

// No id -> get everything
func (s *IbeamServer) GetDeviceInfo(_ context.Context, dIDs *DeviceIDs) (*DeviceInfos, error) {
	log.Debugf("Client asks for DeviceInfo with ids %v", dIDs.Ids)
	if len(dIDs.Ids) == 0 {
		return &DeviceInfos{DeviceInfos: s.parameterRegistry.DeviceInfos}, nil
	} else {
		var rDeviceInfos DeviceInfos

		for _, ID := range dIDs.Ids {
			if device := getDeviceWithID(s, ID); device != nil {
				rDeviceInfos.DeviceInfos = append(rDeviceInfos.DeviceInfos, device)
			}
			// If we have no Device with such a ID, skip
		}
		return &rDeviceInfos, nil
	}
}

func getDeviceWithID(s *IbeamServer, dID uint32) *DeviceInfo {
	for _, device := range s.parameterRegistry.DeviceInfos {
		if device.DeviceID == dID {
			return device
		}
	}
	return nil
}

func (s *IbeamServer) GetModelInfo(_ context.Context, mIDs *ModelIDs) (*ModelInfos, error) {

	if len(mIDs.Ids) == 0 {
		return &ModelInfos{ModelInfos: s.parameterRegistry.ModelInfos}, nil
	} else {
		var rModelInfos ModelInfos

		for _, ID := range mIDs.Ids {
			if model := getModelWithID(s, ID); model != nil {
				rModelInfos.ModelInfos = append(rModelInfos.ModelInfos, model)
			}
			// If we have no Model with such an ID, skip
		}
		return &rModelInfos, nil
	}
}

func getModelWithID(s *IbeamServer, mID uint32) *ModelInfo {
	for _, model := range s.parameterRegistry.ModelInfos {
		if model.Id == mID {
			return model
		}
	}
	return nil
}

// No id -> get everything, as ParamID has 2 dimensions this rule should work
// in both
func (s *IbeamServer) Get(_ context.Context, dpIDs *DeviceParameterIDs) (rParameters *Parameters, err error) {
	rParameters = &Parameters{}
	if len(dpIDs.Ids) == 0 {
		for pid, pState := range s.parameterRegistry.parameterValue {
			for did := range pState {
				dpID := DeviceParameterID{
					Parameter: uint32(pid) + 1,
					Device:    uint32(did) + 1,
				}
				iv := s.parameterRegistry.getInstanceValues(dpID)
				if iv != nil {
					rParameters.Parameters = append(rParameters.Parameters, &Parameter{
						Id:    &dpID,
						Error: 0,
						Value: iv,
					})
				}
			}
		}
		rParameters.Parameters = append(rParameters.Parameters)
	} else {
		for _, dpID := range dpIDs.Ids {
			if dpID.Device == 0 || dpID.Parameter == 0 {
				err = errors.New("Failed to get instance values " + dpID.String())
				return
			}
			iv := s.parameterRegistry.getInstanceValues(*dpID)
			if err != nil || len(iv) == 0 {
				rParameters.Parameters = append(rParameters.Parameters, &Parameter{
					Id:    dpID,
					Error: ParameterError_UnknownError,
					Value: nil,
				})
			} else {
				rParameters.Parameters = append(rParameters.Parameters, &Parameter{
					Id:    dpID,
					Error: ParameterError_NoError,
					Value: iv,
				})
			}
		}
	}
	return
}

func (s *IbeamServer) GetParameterDetails(c context.Context, mpIDs *ModelParameterIDs) (*ParameterDetails, error) {
	log.Debugf("Got a GetParameterDetails from ") //TODO lookup how to get IP
	rParameterDetails := &ParameterDetails{}
	if len(mpIDs.Ids) == 0 {
		for _, modelDetails := range s.parameterRegistry.ParameterDetail {
			rParameterDetails.Details = append(rParameterDetails.Details, modelDetails...)
		}
	} else {
		for _, mpID := range mpIDs.Ids {
			d, err := s.getParameterDetail(mpID)
			if err != nil {
				log.Errorf(err.Error())
				return nil, err
			} else {
				rParameterDetails.Details = append(rParameterDetails.Details, d)
			}
		}
	}
	return rParameterDetails, nil
}

func (s *IbeamServer) getParameterDetail(mpID *ModelParameterID) (*ParameterDetail, error) {
	if mpID.Model == 0 || mpID.Parameter == 0 {
		return nil, errors.New("Failed to get instance values " + mpID.String())
	}
	if len(s.parameterRegistry.ParameterDetail) < int(mpID.Model) {
		return nil, errors.New(fmt.Sprintf("ParamerDetail does not have Model with id %v", mpID.Model))
	}
	for _, parameterDetail := range s.parameterRegistry.ParameterDetail[mpID.Model-1] {
		if parameterDetail.Id.Model == mpID.Model && parameterDetail.Id.Parameter == mpID.Parameter {
			return parameterDetail, nil
		}
	}
	return nil, errors.New("Cannot find ParameterDetail with given ModelParameterID")
}

func (s *IbeamServer) Set(_ context.Context, ps *Parameters) (*Empty, error) {
	for _, parameter := range ps.Parameters {
		s.clientsSetterStream <- *parameter
	}
	return &Empty{}, nil
}

// No id -> subscribe to everything
// On subscribe all current values should be sent back!
func (s *IbeamServer) Subscribe(dpIDs *DeviceParameterIDs, stream IbeamCore_SubscribeServer) error {
	log.Info("New Client subscribed")
	// Fist send all parameters
	parameters, err := s.Get(nil, dpIDs)
	if err != nil {
		return err
	}
	for _, parameter := range parameters.Parameters {
		log.Debugf("Send Parameter with ID '%v' to client", parameter.Id)
		stream.Send(parameter)
	}

	distributor := make(chan Parameter, 100)
	s.serverClientsDistributor[distributor] = true

	log.Debugf("Added distributor number %v", len(s.serverClientsDistributor))

	ping := time.NewTicker(time.Second)

	go func() {
		for {
			select {

			// TODO check if Stream is closed
			case <-ping.C:
				err := stream.Context().Err()
				if err != nil {
					log.Info("lost client")
					s.serverClientsDistributor[distributor] = false
					log.Warn("Connection to client for subscription lost")
					break
				}

				break

			case parameter := <-distributor:
				if parameter.Id == nil || parameter.Id.Device == 0 || parameter.Id.Parameter == 0 {
					continue
				}
				// Check if Device is Subscribed
				if len(dpIDs.Ids) != 0 && !containsDeviceParameter(parameter.Id, dpIDs) {
					continue
				}
				log.Debugf("Send Parameter with ID '%v' to client from ServerClientsStream", parameter.Id)
				stream.Send(&parameter)
			}
		}
	}()
	return nil
}
func containsDeviceParameter(dpID *DeviceParameterID, dpIDs *DeviceParameterIDs) bool {
	for _, ids := range dpIDs.Ids {
		if ids == dpID {
			return true
		}
	}
	return false
}

func CreateServer(coreInfo CoreInfo, defaultModel ModelInfo) (server IbeamServer, manager *IbeamParameterManager, registry *IbeamParameterRegistry, set chan Parameter, getFromGRCP chan Parameter) {

	clientsSetter := make(chan Parameter, 100)
	getFromGRCP = make(chan Parameter, 100)
	set = make(chan Parameter, 100)

	fistParameter := Parameter{}
	fistParameter.Id = &DeviceParameterID{
		Device:    0,
		Parameter: 0,
	}

	watcher := make(chan Parameter)

	registry = &IbeamParameterRegistry{
		coreInfo:        coreInfo,
		DeviceInfos:     []*DeviceInfo{},
		ModelInfos:      []*ModelInfo{},
		ParameterDetail: [][]*ParameterDetail{},
		parameterValue:  [][][]*IBeamParameterValueBuffer{},
	}

	server = IbeamServer{
		parameterRegistry:        registry,
		clientsSetterStream:      clientsSetter,
		serverClientsStream:      watcher,
		serverClientsDistributor: make(map[chan Parameter]bool),
	}

	manager = &IbeamParameterManager{
		parameterRegistry:   registry,
		out:                 getFromGRCP,
		in:                  set,
		clientsSetterStream: clientsSetter,
		serverClientsStream: watcher,
	}

	go func() {
		for {
			parameter := <-watcher
			for channel, isOpen := range server.serverClientsDistributor {
				if isOpen {
					channel <- parameter
				} else {
					log.Debugf("Deleted Channel %v", channel)
					delete(server.serverClientsDistributor, channel)
				}
			}
		}
	}()

	log.Info("Server created")
	registry.RegisterModel(&defaultModel)
	return
}

// Add desc
func (m *IbeamParameterRegistry) RegisterParameter(detail *ParameterDetail) uint32 {
	mid := uint32(1)
	if detail.Id != nil {
		if detail.Id.Model != 0 {
			mid = detail.Id.Model
		}
	}
	m.muDetail.RLock()
	if uint32(len(m.ParameterDetail)) <= (mid - 1) {
		log.Panic("Could not register parameter for nonexistent model ", mid)
		return 0
	}

	modelconfig := &m.ParameterDetail[mid-1]
	paramIndex := uint32(len(*modelconfig) + 1)
	m.muDetail.RUnlock()
	detail.Id = &ModelParameterID{
		Parameter: paramIndex,
		Model:     mid,
	}
	m.muDetail.Lock()
	*modelconfig = append(*modelconfig, detail)
	m.muDetail.Unlock()
	log.Debugf("ParameterDetail '%v' registered with ID: %v for Model %v", detail.Name, detail.Id.Parameter, detail.Id.Model)
	return paramIndex
}

// Add desc
func (m *IbeamParameterRegistry) RegisterModel(model *ModelInfo) uint32 {
	m.muDetail.RLock()
	model.Id = uint32(len(m.ParameterDetail) + 1)
	m.muDetail.RUnlock()
	m.muInfo.Lock()
	m.ModelInfos = append(m.ModelInfos, model)
	m.muInfo.Unlock()
	m.muDetail.Lock()
	m.ParameterDetail = append(m.ParameterDetail, []*ParameterDetail{})
	m.muDetail.Unlock()
	log.Debugf("Model '%v' registered with ID: %v ", model.Name, model.Id)
	return model.Id
}

// Add desc
func (m *IbeamParameterRegistry) RegisterDevice(modelID uint32) uint32 { //DeviceInfo) {
	mid := uint32(1)
	if modelID != 0 {
		mid = modelID
	}
	m.muDetail.RLock()
	if uint32(len(m.ParameterDetail)) <= (mid - 1) {
		log.Panicf("Could not register device for nonexistent model with id: %v", mid)
		return 0
	}

	modelConfig := m.ParameterDetail[mid-1]
	m.muDetail.RUnlock()

	// create device info
	// take all params from model and generate a value buffer array for all instances
	// add value buffers to the state array

	parameterValuesBuffer := &[][]*IBeamParameterValueBuffer{}
	for _, parameterDetail := range modelConfig {
		valueInstances := []*IBeamParameterValueBuffer{}
		initialValue := ParameterValue{Value: &ParameterValue_Integer{Integer: 0}}

		switch parameterDetail.ValueType.Type() {
		case ValueType_NoValue.Type():
			initialValue.Value = &ParameterValue_Cmd{Cmd: Command_Trigger}
			// This is redundant:
		//case ValueType_Integer.Type():
		//	initialValue.Value = &ParameterValue_Integer{Integer: 0}
		case ValueType_Floating.Type():
			initialValue.Value = &ParameterValue_Floating{Floating: 0.0}
		case ValueType_Opt.Type():
			initialValue.Value = &ParameterValue_CurrentOption{CurrentOption: 0}
		case ValueType_String.Type():
			initialValue.Value = &ParameterValue_Str{Str: ""}
		case ValueType_Binary.Type():
			initialValue.Value = &ParameterValue_Binary{Binary: false}
		}

		for i := uint32(0); i < parameterDetail.Instances; i++ {
			valueInstances = append(valueInstances, &IBeamParameterValueBuffer{
				instanceID:     i + 1,
				available:      true,
				isAssumedState: true,
				lastUpdate:     time.Now(),
				currentValue:   initialValue,
				targetValue:    initialValue,
			})
		}

		for i := uint32(0); i < m.coreInfo.MaxDevices; i++ {
			*parameterValuesBuffer = append(*parameterValuesBuffer, valueInstances)
		}
	}

	m.muInfo.RLock()
	deviceIndex := uint32(len(m.DeviceInfos) + 1)
	m.muInfo.RUnlock()
	m.muInfo.Lock()
	m.DeviceInfos = append(m.DeviceInfos, &DeviceInfo{
		DeviceID: deviceIndex,
		ModelID:  modelID,
	})
	m.muInfo.Unlock()
	m.muValue.Lock()
	m.parameterValue = append(m.parameterValue, *parameterValuesBuffer)
	m.muValue.Unlock()

	log.Debugf("Device '%v' registered with model: %v (%v)", deviceIndex, mid, m.ModelInfos[mid-1].Name)
	return deviceIndex
}

func (m *IbeamParameterRegistry) GetIDMap() map[string]ParameterDetail {
	idMap := make(map[string]ParameterDetail)
	m.muDetail.RLock()
	for _, parameter := range m.ParameterDetail[0] {
		idMap[parameter.Name] = *parameter
	}
	m.muDetail.RUnlock()
	return idMap
}

// Add desc
func (m *IbeamParameterManager) Start() {

	go func() {
		for {
			select {
			case parameter := <-m.clientsSetterStream:
				// Client set loop, inputs set requests from grpc to manager
				//parameter := <-m.clientsSetterStream
				state := m.parameterRegistry.parameterValue
				deviceIndex := int(parameter.Id.Device) - 1
				parameterIndex := int(parameter.Id.Parameter) - 1

				for _, value := range parameter.Value {
					if len(state) <= deviceIndex || len(state[deviceIndex]) <= parameterIndex {
						log.Errorf("Client tried to set invalid device index %v for parameter %v", deviceIndex, parameterIndex+1)
						continue
					}
					if value.InstanceID == 0 || len(state[deviceIndex][parameterIndex]) < int(value.InstanceID) {
						log.Errorf("Received invalid instance id %v for parameter %v", value.InstanceID, parameterIndex+1)
						continue
					}
					parameterBuffer := state[deviceIndex][parameterIndex][value.InstanceID-1]
					if value.Value == nil {
						//Maybe it is a trigger button
						/*
							log.Debugf("Got No Value, but maybe it is a Trigger Button:")
							value.Value = &ParameterValue_Cmd{
								Cmd: Command_Trigger,
							}
							m.out <- Parameter{
								Id:    parameter.Id,
								Error: 0,
								Value: []*ParameterValue{value},
							}*/
						continue
					}
					switch v := value.Value.(type) {
					case *ParameterValue_Cmd:
						log.Debugf("Got Set CMD: %v", v)
						m.out <- Parameter{
							Id:    parameter.Id,
							Error: 0,
							Value: []*ParameterValue{value},
						}
					case *ParameterValue_Binary:
						log.Debugf("Got Set Binary: %v", v)
						if v != parameterBuffer.currentValue.Value {
							parameterBuffer.isAssumedState = true
						}
						parameterBuffer.targetValue.Value = v
						parameterBuffer.tryCount = 0
					case *ParameterValue_Floating:
						log.Debugf("Got Set Float: %v", v)
						modelIndex := m.parameterRegistry.getModelIndex(deviceIndex)
						parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]
						if v.Floating > parameterConfig.Maximum {
							log.Debugf("Max violation for parameter %v", parameterIndex+1)
							m.serverClientsStream <- Parameter{
								Id:    parameter.Id,
								Error: ParameterError_MaxViolation,
								Value: []*ParameterValue{},
							}
							continue
						}
						if v.Floating < parameterConfig.Minimum {
							log.Debugf("Min violation for parameter %v", parameterIndex+1)
							m.serverClientsStream <- Parameter{
								Id:    parameter.Id,
								Error: ParameterError_MinViolation,
								Value: []*ParameterValue{},
							}
							continue
						}
						parameterBuffer.targetValue.Value = v
						parameterBuffer.tryCount = 0

					case *ParameterValue_Integer:
						log.Debugf("Got Set Integer: %v", v)
						modelIndex := m.parameterRegistry.getModelIndex(deviceIndex)
						parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]
						if v.Integer > int32(parameterConfig.Maximum) {
							log.Debugf("Max violation for parameter %v", parameterIndex+1)
							m.serverClientsStream <- Parameter{
								Id:    parameter.Id,
								Error: ParameterError_MaxViolation,
								Value: []*ParameterValue{},
							}
							continue
						}
						if v.Integer < int32(parameterConfig.Minimum) {
							log.Debugf("Min violation for parameter %v", parameterIndex+1)
							m.serverClientsStream <- Parameter{
								Id:    parameter.Id,
								Error: ParameterError_MinViolation,
								Value: []*ParameterValue{},
							}
							continue
						}

						parameterBuffer.targetValue.Value = v
						parameterBuffer.tryCount = 0

					case *ParameterValue_OptionList:
						log.Debugf("Got Set Option List: %v", v)

					case *ParameterValue_Str:
						log.Debugf("Got Set String: %v", v)
					case *ParameterValue_CurrentOption:
						log.Debugf("Got Set Current Option: %v", v)

						modelIndex := m.parameterRegistry.getModelIndex(deviceIndex)
						parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]
						if parameterConfig.OptionList == nil {
							log.Errorf("No option List found for Parameter %v", v)
							continue
						}
						if v.CurrentOption > uint32(len(parameterConfig.OptionList.Options)) {
							log.Errorf("Invalid operation index for parameter %v", parameterIndex+1)
							m.serverClientsStream <- Parameter{
								Id:    parameter.Id,
								Error: ParameterError_UnknownID,
								Value: []*ParameterValue{},
							}
							continue
						}

						parameterBuffer.targetValue.Value = v
						parameterBuffer.tryCount = 0

					}
				}

			case parameter := <-m.in:
				// Param Ingest loop, inputs changes from camera to manager
				if parameter.Id == nil {
					continue
				}
				state := m.parameterRegistry.parameterValue
				deviceIndex := int(parameter.Id.Device) - 1
				parameterIndex := int(parameter.Id.Parameter) - 1
				modelIndex := int(m.parameterRegistry.getModelIndex(deviceIndex))
				parameterConfig := *m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]
				shouldSend := false
				for _, value := range parameter.Value {
					if len(state) <= deviceIndex || len(state[deviceIndex]) <= parameterIndex {
						log.Errorf("Client tried to set invalid device %v for parameter %v", deviceIndex+1, parameterIndex+1)
						continue
					}
					if len(state[deviceIndex][parameterIndex]) <= int(value.InstanceID-1) {
						log.Errorf("Received invalid instance id %v for parameter %v", value.InstanceID, parameterIndex)
						continue
					}
					parameterBuffer := state[deviceIndex][parameterIndex][value.InstanceID-1]
					if value.Value != nil {
						if time.Until(parameterBuffer.lastUpdate).Milliseconds() > int64(parameterConfig.QuarantineDelayMs) {
							parameterBuffer.targetValue.Value = value.Value
						}
						parameterBuffer.currentValue.Value = value.Value
						parameterBuffer.isAssumedState = parameterBuffer.currentValue.Value != parameterBuffer.targetValue.Value
					} else {
						parameterBuffer.available = value.Available
					}
					shouldSend = true
				}

				if !shouldSend {
					continue
				}

				if values := m.parameterRegistry.getInstanceValues(*parameter.Id); values != nil {
					m.serverClientsStream <- Parameter{Value: values, Id: parameter.Id, Error: 0}
				}

			}

			// Main Loop
			state := m.parameterRegistry.parameterValue
			for _, device := range m.parameterRegistry.ParameterDetail {
				for _, parameterDetail := range device {
					if ControlStyle_Undefined == parameterDetail.ControlStyle {
						continue
					}

					devices := m.parameterRegistry.parameterValue
					for did := 0; did < len(devices); did++ {
						for iid := 0; iid < int(parameterDetail.Instances); iid++ {
							if parameterDetail.Id == nil {
								continue
							}

							parameterBuffer := state[did][parameterDetail.Id.Parameter-1][iid]

							switch parameterDetail.ControlStyle {
							case ControlStyle_Normal:
								if parameterBuffer.currentValue.Value == parameterBuffer.targetValue.Value {
									log.Infof("Failed to set Parameter %v, cause current and target are same", parameterDetail.Id.Parameter)
									continue
								}
								if time.Until(parameterBuffer.lastUpdate).Milliseconds()*(-1) < int64(parameterDetail.ControlDelayMs) {
									log.Infof("Failed to set Parameter %v, cause ControlDelay time | Last Update: %v, Time in ms since this: %v, ControlDelayMs: %v", parameterDetail.Id.Parameter, parameterBuffer.lastUpdate, time.Until(parameterBuffer.lastUpdate).Milliseconds(), parameterDetail.ControlDelayMs)
									continue
								}
								if parameterDetail.FeedbackStyle == FeedbackStyle_NoFeedback {
									log.Infof("Failed to set Parameter %v, cause not necessary", parameterDetail.Id.Parameter)
									continue
								}
								if parameterDetail.RetryCount != 0 {
									parameterBuffer.tryCount++
									if parameterBuffer.tryCount > parameterDetail.RetryCount {
										log.Errorf("Failed to set parameter %v in %v tries on device %v", parameterDetail.Id.Parameter, parameterDetail.RetryCount, did+1)
										parameterBuffer.targetValue = parameterBuffer.currentValue
										parameterBuffer.isAssumedState = false

										m.serverClientsStream <- Parameter{
											Value: []*ParameterValue{parameterBuffer.getParameterValue()},
											Id: &DeviceParameterID{
												Device:    uint32(did + 1),
												Parameter: parameterDetail.Id.Parameter,
											},
											Error: ParameterError_MaxRetrys,
										}
										continue
									}
								}

								parameterBuffer.lastUpdate = time.Now()
								parameterBuffer.isAssumedState = true
								if parameterDetail.Id == nil {
									log.Error("No Parameter provided")
									continue
								}

								m.out <- Parameter{
									Value: []*ParameterValue{parameterBuffer.getParameterValue()},
									Id: &DeviceParameterID{
										Device:    uint32(did + 1),
										Parameter: parameterDetail.Id.Parameter,
									},
									Error: 0,
								}

							case ControlStyle_ControlledIncremental:
								if parameterDetail.FeedbackStyle == FeedbackStyle_NoFeedback ||
									parameterBuffer.currentValue.Value == parameterBuffer.targetValue.Value ||
									time.Until(parameterBuffer.lastUpdate).Milliseconds() < int64(parameterDetail.ControlDelayMs) {
									continue
								}

								if parameterDetail.RetryCount != 0 {
									parameterBuffer.tryCount++
									if parameterBuffer.tryCount > parameterDetail.RetryCount {
										log.Errorf("Failed to set parameter %v in %v tries on device %v", parameterDetail.Id.Parameter, parameterDetail.RetryCount, did+1)
										parameterBuffer.targetValue = parameterBuffer.currentValue
										continue
									}
								}
								type action string
								const (
									Increment   action = "Increment"
									Decrement   action = "Decrement"
									NoOperation action = "NoOperation"
								)

								var cmdAction action

								switch value := parameterBuffer.currentValue.Value.(type) {
								case *ParameterValue_Integer:
									if targetValue, ok := parameterBuffer.targetValue.Value.(*ParameterValue_Integer); ok {
										if value.Integer < targetValue.Integer {
											cmdAction = Increment
										} else if value.Integer > targetValue.Integer {
											cmdAction = Decrement
										}
									}
								case *ParameterValue_Floating:
									if targetValue, ok := parameterBuffer.targetValue.Value.(*ParameterValue_Floating); ok {
										if value.Floating < targetValue.Floating {
											cmdAction = Increment
										} else if value.Floating > targetValue.Floating {
											cmdAction = Decrement
										}
									}
								case *ParameterValue_CurrentOption:
									if targetValue, ok := parameterBuffer.targetValue.Value.(*ParameterValue_CurrentOption); ok {
										if value.CurrentOption < targetValue.CurrentOption {
											cmdAction = Increment
										} else if value.CurrentOption > targetValue.CurrentOption {
											cmdAction = Decrement
										}
									}
								default:
									log.Errorf("Could not match Valuetype %T", value)
								}

								var cmdValue []*ParameterValue

								switch cmdAction {
								case Increment:
									cmdValue = append(cmdValue, parameterBuffer.incrementParameterValue())
								case Decrement:
									cmdValue = append(cmdValue, parameterBuffer.decrementParameterValue())
								case NoOperation:
									continue
								}
								parameterBuffer.lastUpdate = time.Now()
								if parameterDetail.Id == nil {
									log.Errorf("No DeviceParameterID")
									continue
								}

								m.out <- Parameter{
									Id: &DeviceParameterID{
										Device:    uint32(did + 1),
										Parameter: parameterDetail.Id.Parameter,
									},
									Error: 0,
									Value: cmdValue,
								}

							case ControlStyle_NoControl, ControlStyle_Incremental, ControlStyle_Oneshot:
								// DO Nothing
							default:
								log.Errorf("Could not match controlltype")
								continue
							}
						}
					}
				}

			}
		}
	}()
}
