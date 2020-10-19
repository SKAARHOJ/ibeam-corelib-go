package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/SKAARHOJ/ibeam-core-go/ibeam_core"
	log "github.com/s00500/env_logger"
)

func init() {
	log.ConfigureDefaultLogger()
}

type IbeamParameterRegistry struct {
	coreInfo        ibeam_core.CoreInfo
	DeviceInfos     []*ibeam_core.DeviceInfo
	ModelInfos      []*ibeam_core.ModelInfo
	ParameterDetail [][]*ibeam_core.ParameterDetail  //Parameter Value: model, parameter
	parameterValue  [][][]*IBeamParameterValueBuffer //Parameter State: device,parameter,instance
}

type IbeamServer struct {
	parameterRegistry        *IbeamParameterRegistry
	clientsSetterStream      chan ibeam_core.Parameter
	serverClientsStream      chan ibeam_core.Parameter
	serverClientsDistributor map[chan ibeam_core.Parameter]bool
}

type IbeamParameterManager struct {
	parameterRegistry   *IbeamParameterRegistry
	out                 chan ibeam_core.Parameter
	in                  chan ibeam_core.Parameter
	clientsSetterStream chan ibeam_core.Parameter
	serverClientsStream chan ibeam_core.Parameter
}

type IBeamParameterValueBuffer struct {
	instanceID     uint32
	available      bool
	isAssumedState bool
	lastUpdate     time.Time
	tryCount       uint32
	currentValue   ibeam_core.ParameterValue
	targetValue    ibeam_core.ParameterValue
}

func (b *IBeamParameterValueBuffer) getParameterValue() *ibeam_core.ParameterValue {
	return &ibeam_core.ParameterValue{
		InstanceID:     b.instanceID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState,
		Value:          b.targetValue.Value,
	}
}

func (b *IBeamParameterValueBuffer) incrementParameterValue() *ibeam_core.ParameterValue {
	return &ibeam_core.ParameterValue{
		InstanceID:     b.instanceID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState,
		Value: &ibeam_core.ParameterValue_Cmd{
			Cmd: ibeam_core.Command_Increment,
		},
	}
}
func (b *IBeamParameterValueBuffer) decrementParameterValue() *ibeam_core.ParameterValue {
	return &ibeam_core.ParameterValue{
		InstanceID:     b.instanceID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState,
		Value: &ibeam_core.ParameterValue_Cmd{
			Cmd: ibeam_core.Command_Decrement,
		},
	}
}

// device,parameter,instance
func (b *IbeamParameterRegistry) getInstanceValues(dpID ibeam_core.DeviceParameterID) (values []*ibeam_core.ParameterValue) {
	deviceIndex := int(dpID.Device) - 1
	parameterIndex := int(dpID.Parameter) - 1

	if dpID.Device == 0 || dpID.Parameter == 0 || len(b.parameterValue) <= deviceIndex || len(b.parameterValue[deviceIndex]) <= parameterIndex {
		log.Errorf("Could not get instance Values for DeviceParameterID: %v", dpID)
		return nil
	}

	for _, value := range b.parameterValue[deviceIndex][parameterIndex] {
		values = append(values, value.getParameterValue())
	}
	return
}

func (d *IbeamParameterRegistry) getModelIndex(deviceID int) uint32 {
	if len(d.DeviceInfos) <= deviceID {
		log.Panicf("Could not get model for device with id %v", deviceID)
	}
	return d.DeviceInfos[deviceID].ModelID //Todo was -1 before
}

func (s *IbeamServer) GetCoreInfo(_ context.Context, _ *ibeam_core.Empty) (*ibeam_core.CoreInfo, error) {
	return &s.parameterRegistry.coreInfo, nil
}

// No id -> get everything
func (s *IbeamServer) GetDeviceInfo(_ context.Context, dIDs *ibeam_core.DeviceIDs) (*ibeam_core.DeviceInfos, error) {
	log.Debugf("Client asks for DeviceInfo with ids %v", dIDs.Ids)
	if len(dIDs.Ids) == 0 {
		return &ibeam_core.DeviceInfos{DeviceInfos: s.parameterRegistry.DeviceInfos}, nil
	} else {
		var rDeviceInfos ibeam_core.DeviceInfos

		for _, ID := range dIDs.Ids {
			if device := getDeviceWithID(s, ID); device != nil {
				rDeviceInfos.DeviceInfos = append(rDeviceInfos.DeviceInfos, device)
			}
			// If we have no Device with such a ID, skip
		}
		return &rDeviceInfos, nil
	}
}

func getDeviceWithID(s *IbeamServer, dID uint32) *ibeam_core.DeviceInfo {
	for _, device := range s.parameterRegistry.DeviceInfos {
		if device.DeviceID == dID {
			return device
		}
	}
	return nil
}

func (s *IbeamServer) GetModelInfo(_ context.Context, mIDs *ibeam_core.ModelIDs) (*ibeam_core.ModelInfos, error) {

	if len(mIDs.Ids) == 0 {
		return &ibeam_core.ModelInfos{ModelInfos: s.parameterRegistry.ModelInfos}, nil
	} else {
		var rModelInfos ibeam_core.ModelInfos

		for _, ID := range mIDs.Ids {
			if model := getModelWithID(s, ID); model != nil {
				rModelInfos.ModelInfos = append(rModelInfos.ModelInfos, model)
			}
			// If we have no Model with such an ID, skip
		}
		return &rModelInfos, nil
	}
}

func getModelWithID(s *IbeamServer, mID uint32) *ibeam_core.ModelInfo {
	for _, model := range s.parameterRegistry.ModelInfos {
		if model.Id == mID {
			return model
		}
	}
	return nil
}

// No id -> get everything, as ParamID has 2 dimensions this rule should work
// in both
func (s *IbeamServer) Get(_ context.Context, dpIDs *ibeam_core.DeviceParameterIDs) (rParameters *ibeam_core.Parameters, err error) {
	rParameters = &ibeam_core.Parameters{}
	if len(dpIDs.Ids) == 0 {
		for pid, pState := range s.parameterRegistry.parameterValue {
			for did := range pState {
				dpID := ibeam_core.DeviceParameterID{
					Parameter: uint32(pid) + 1,
					Device:    uint32(did) + 1,
				}
				iv := s.parameterRegistry.getInstanceValues(dpID)
				if iv != nil {
					rParameters.Parameters = append(rParameters.Parameters, &ibeam_core.Parameter{
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
				rParameters.Parameters = append(rParameters.Parameters, &ibeam_core.Parameter{
					Id:    dpID,
					Error: ibeam_core.ParameterError_UnknownError,
					Value: nil,
				})
			} else {
				rParameters.Parameters = append(rParameters.Parameters, &ibeam_core.Parameter{
					Id:    dpID,
					Error: ibeam_core.ParameterError_NoError,
					Value: iv,
				})
			}
		}
	}
	return
}

func (s *IbeamServer) GetParameterDetails(c context.Context, mpIDs *ibeam_core.ModelParameterIDs) (*ibeam_core.ParameterDetails, error) {
	log.Debugf("Got a GetParameterDetails from ") //TODO lookup how to get IP
	rParameterDetails := &ibeam_core.ParameterDetails{}
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

func (s *IbeamServer) getParameterDetail(mpID *ibeam_core.ModelParameterID) (*ibeam_core.ParameterDetail, error) {
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

func (s *IbeamServer) Set(_ context.Context, ps *ibeam_core.Parameters) (*ibeam_core.Empty, error) {
	for _, parameter := range ps.Parameters {
		s.clientsSetterStream <- *parameter
	}
	return &ibeam_core.Empty{}, nil
}

// No id -> subscribe to everything
// On subscribe all current values should be sent back!
func (s *IbeamServer) Subscribe(dpIDs *ibeam_core.DeviceParameterIDs, stream ibeam_core.IbeamCore_SubscribeServer) error {
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

	distributor := make(chan ibeam_core.Parameter, 100)
	s.serverClientsDistributor[distributor] = true

	log.Debugf("Added distributor number %v", len(s.serverClientsDistributor))

	go func() {
		for {
			select {
			/*
				// TODO check if Stream is closed
					case <-stream:
						s.serverClientsDistributor[distributor] = false
						log.Warn("Connection to client for subscription lost")
						break
			*/
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
func containsDeviceParameter(dpID *ibeam_core.DeviceParameterID, dpIDs *ibeam_core.DeviceParameterIDs) bool {
	for _, ids := range dpIDs.Ids {
		if ids == dpID {
			return true
		}
	}
	return false
}

func CreateServer(coreInfo ibeam_core.CoreInfo, defaultModel ibeam_core.ModelInfo) (server IbeamServer, manager *IbeamParameterManager, registry *IbeamParameterRegistry, set chan ibeam_core.Parameter, get chan ibeam_core.Parameter) {

	clientsSetter := make(chan ibeam_core.Parameter, 100)
	get = make(chan ibeam_core.Parameter, 100)
	set = make(chan ibeam_core.Parameter, 100)

	fistParameter := ibeam_core.Parameter{}
	fistParameter.Id = &ibeam_core.DeviceParameterID{
		Device:    0,
		Parameter: 0,
	}

	watcher := make(chan ibeam_core.Parameter)

	registry = &IbeamParameterRegistry{
		coreInfo:        coreInfo,
		DeviceInfos:     []*ibeam_core.DeviceInfo{},
		ModelInfos:      []*ibeam_core.ModelInfo{},
		ParameterDetail: [][]*ibeam_core.ParameterDetail{},
		parameterValue:  [][][]*IBeamParameterValueBuffer{},
	}

	server = IbeamServer{
		parameterRegistry:        registry,
		clientsSetterStream:      clientsSetter,
		serverClientsStream:      watcher,
		serverClientsDistributor: make(map[chan ibeam_core.Parameter]bool),
	}

	manager = &IbeamParameterManager{
		parameterRegistry:   registry,
		out:                 get,
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

	log.Debug("Server created")
	registry.RegisterModel(&defaultModel)
	return
}

// Add desc
func (m *IbeamParameterRegistry) RegisterParameter(detail *ibeam_core.ParameterDetail) uint32 {
	mid := uint32(1)
	if detail.Id != nil {
		if detail.Id.Model != 0 {
			mid = detail.Id.Model
		}
	}
	if uint32(len(m.ParameterDetail)) <= (mid - 1) {
		log.Panic("Could not register parameter for nonexistent model ", mid)
		return 0
	}

	modelconfig := &m.ParameterDetail[mid-1]
	paramIndex := uint32(len(*modelconfig) + 1)

	detail.Id = &ibeam_core.ModelParameterID{
		Parameter: paramIndex,
		Model:     mid,
	}
	*modelconfig = append(*modelconfig, detail)
	log.Debugf("ParameterDetail '%v' registered with ID: %v for Model %v", detail.Name, detail.Id.Parameter, detail.Id.Model)
	return paramIndex
}

// Add desc
func (m *IbeamParameterRegistry) RegisterModel(model *ibeam_core.ModelInfo) uint32 {
	model.Id = uint32(len(m.ParameterDetail) + 1)
	m.ModelInfos = append(m.ModelInfos, model)
	m.ParameterDetail = append(m.ParameterDetail, []*ibeam_core.ParameterDetail{})
	log.Debugf("Model '%v' registered with ID: %v ", model.Name, model.Id)
	return model.Id
}

// Add desc
func (m *IbeamParameterRegistry) RegisterDevice(modelID uint32) uint32 { //ibeam_core.DeviceInfo) {
	mid := uint32(1)
	if modelID != 0 {
		mid = modelID
	}
	if uint32(len(m.ParameterDetail)) <= (mid - 1) {
		log.Panicf("Could not register device for nonexistent model with id: %v", mid)
		return 0
	}

	modelConfig := &m.ParameterDetail[mid-1]

	// create device info
	// take all params from model and generate a value buffer array for all instances
	// add value buffers to the state array

	parameterValuesBuffer := &[][]*IBeamParameterValueBuffer{}

	for _, parameterDetail := range *modelConfig {
		valueInstances := []*IBeamParameterValueBuffer{}
		initialValue := ibeam_core.ParameterValue{Value: &ibeam_core.ParameterValue_Integer{Integer: 0}}

		switch parameterDetail.ValueType.Type() {
		// This is redundant:
		//case ibeam_core.ValueType_NoValue.Type():
		//	initialValue.Value = &ibeam_core.ParameterValue_Integer{Integer: 0}
		//case ibeam_core.ValueType_Integer.Type():
		//	initialValue.Value = &ibeam_core.ParameterValue_Integer{Integer: 0}
		case ibeam_core.ValueType_Floating.Type():
			initialValue.Value = &ibeam_core.ParameterValue_Floating{Floating: 0.0}
		case ibeam_core.ValueType_Opt.Type():
			initialValue.Value = &ibeam_core.ParameterValue_CurrentOption{CurrentOption: 0}
		case ibeam_core.ValueType_String.Type():
			initialValue.Value = &ibeam_core.ParameterValue_Str{Str: ""}
		case ibeam_core.ValueType_Binary.Type():
			initialValue.Value = &ibeam_core.ParameterValue_Binary{Binary: false}
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

	deviceIndex := uint32(len(m.DeviceInfos) + 1)

	m.DeviceInfos = append(m.DeviceInfos, &ibeam_core.DeviceInfo{
		DeviceID: deviceIndex,
		ModelID:  modelID,
	})
	m.parameterValue = append(m.parameterValue, *parameterValuesBuffer)

	log.Debugf("Device '%v' registered with model: %v (%v)", deviceIndex, mid, m.ModelInfos[mid-1].Name)
	return deviceIndex
}

func (m *IbeamParameterRegistry) GetIdMap() map[string]uint32 {
	idMap := make(map[string]uint32)
	for _, parameter := range m.ParameterDetail[0] {
		idMap[parameter.Name] = parameter.Id.Parameter
	}
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
					if value.Value == nil {
						continue
					}
					parameterBuffer := state[deviceIndex][parameterIndex][value.InstanceID-1]
					switch v := value.Value.(type) {
					case *ibeam_core.ParameterValue_Cmd:
						log.Debugf("Got Set CMD: %v", v)
						m.out <- ibeam_core.Parameter{
							Id:    parameter.Id,
							Error: 0,
							Value: []*ibeam_core.ParameterValue{value},
						}
					case *ibeam_core.ParameterValue_Binary:
						log.Debugf("Got Set Binary: %v", v)
					case *ibeam_core.ParameterValue_Floating:
						log.Debugf("Got Set Float: %v", v)
						modelIndex := m.parameterRegistry.getModelIndex(deviceIndex)
						parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]
						if v.Floating > parameterConfig.Maximum {
							log.Debugf("Max violation for parameter %v", parameterIndex+1)
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_MaxViolation,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}
						if v.Floating < parameterConfig.Minimum {
							log.Debugf("Min violation for parameter %v", parameterIndex+1)
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_MinViolation,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}
						if v != parameterBuffer.currentValue.Value {
							parameterBuffer.isAssumedState = true
						}
						parameterBuffer.targetValue.Value = v
						parameterBuffer.tryCount = 0

					case *ibeam_core.ParameterValue_Integer:
						log.Debugf("Got Set Integer: %v", v)
						modelIndex := m.parameterRegistry.getModelIndex(deviceIndex)
						parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]
						if v.Integer > int32(parameterConfig.Maximum) {
							log.Debugf("Max violation for parameter %v", parameterIndex+1)
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_MaxViolation,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}
						if v.Integer < int32(parameterConfig.Minimum) {
							log.Debugf("Min violation for parameter %v", parameterIndex+1)
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_MinViolation,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}
						if v != parameterBuffer.currentValue.Value {
							parameterBuffer.isAssumedState = true
						}
						parameterBuffer.targetValue.Value = v
						parameterBuffer.tryCount = 0

					case *ibeam_core.ParameterValue_OptionList:
						log.Debugf("Got Set Option List: %v", v)

					case *ibeam_core.ParameterValue_Str:
						log.Debugf("Got Set String: %v", v)
					case *ibeam_core.ParameterValue_CurrentOption:
						log.Debugf("Got Set Current Option: %v", v)

						modelIndex := m.parameterRegistry.getModelIndex(deviceIndex)
						parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]
						if parameterConfig.OptionList == nil {
							log.Errorf("No option List found for Parameter %v", v)
							continue
						}
						if v.CurrentOption > uint32(len(parameterConfig.OptionList.Options)) {
							log.Errorf("Invalid operation index for parameter %v", parameterIndex+1)
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_UnknownID,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}
						if v != parameterBuffer.currentValue.Value {
							parameterBuffer.isAssumedState = true
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
							parameterBuffer.targetValue = *value
						}
						parameterBuffer.currentValue = *value
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
					m.serverClientsStream <- ibeam_core.Parameter{Value: values, Id: parameter.Id, Error: 0}
				}

			}

			// Main Loop
			state := m.parameterRegistry.parameterValue
			for _, device := range m.parameterRegistry.ParameterDetail {
				for _, parameterDetail := range device {
					if ibeam_core.ControlStyle_Undefined == parameterDetail.ControlStyle {
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
							case ibeam_core.ControlStyle_Normal:
								if parameterDetail.FeedbackStyle == ibeam_core.FeedbackStyle_NoFeedback ||
									parameterBuffer.currentValue.Value == parameterBuffer.targetValue.Value ||
									time.Until(parameterBuffer.lastUpdate).Milliseconds() < int64(parameterDetail.ControlDelayMs) {
									continue
								}
								if parameterDetail.RetryCount != 0 {
									parameterBuffer.tryCount++
									if parameterBuffer.tryCount > parameterDetail.RetryCount {
										log.Errorf("Failed to set parameter %v in %v tries on device %v", parameterDetail.Id.Parameter, parameterDetail.RetryCount, did+1)
										parameterBuffer.targetValue = parameterBuffer.currentValue
										parameterBuffer.isAssumedState = false
										m.serverClientsStream <- ibeam_core.Parameter{
											Value: []*ibeam_core.ParameterValue{parameterBuffer.getParameterValue()},
											Id: &ibeam_core.DeviceParameterID{
												Device:    uint32(did + 1),
												Parameter: parameterDetail.Id.Parameter,
											},
											Error: ibeam_core.ParameterError_MaxRetrys,
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

								m.out <- ibeam_core.Parameter{
									Value: []*ibeam_core.ParameterValue{parameterBuffer.getParameterValue()},
									Id: &ibeam_core.DeviceParameterID{
										Device:    uint32(did + 1),
										Parameter: parameterDetail.Id.Parameter,
									},
									Error: 0,
								}

							case ibeam_core.ControlStyle_ControlledIncremental:
								if parameterDetail.FeedbackStyle == ibeam_core.FeedbackStyle_NoFeedback ||
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
								case *ibeam_core.ParameterValue_Integer:
									if targetValue, ok := parameterBuffer.targetValue.Value.(*ibeam_core.ParameterValue_Integer); ok {
										if value.Integer < targetValue.Integer {
											cmdAction = Increment
										} else if value.Integer > targetValue.Integer {
											cmdAction = Decrement
										}
									}
								case *ibeam_core.ParameterValue_Floating:
									if targetValue, ok := parameterBuffer.targetValue.Value.(*ibeam_core.ParameterValue_Floating); ok {
										if value.Floating < targetValue.Floating {
											cmdAction = Increment
										} else if value.Floating > targetValue.Floating {
											cmdAction = Decrement
										}
									}
								case *ibeam_core.ParameterValue_CurrentOption:
									if targetValue, ok := parameterBuffer.targetValue.Value.(*ibeam_core.ParameterValue_CurrentOption); ok {
										if value.CurrentOption < targetValue.CurrentOption {
											cmdAction = Increment
										} else if value.CurrentOption > targetValue.CurrentOption {
											cmdAction = Decrement
										}
									}
								default:
									log.Errorf("Could not match Valuetype %T", value)
								}

								var cmdValue []*ibeam_core.ParameterValue

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
								param := ibeam_core.Parameter{
									Id: &ibeam_core.DeviceParameterID{
										Device:    uint32(did + 1),
										Parameter: parameterDetail.Id.Parameter,
									},
									Error: 0,
									Value: cmdValue,
								}

								m.out <- param

							case ibeam_core.ControlStyle_NoControl, ibeam_core.ControlStyle_Incremental, ibeam_core.ControlStyle_Oneshot:
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

/*

Just for Quick testing



const (
	iBeamVersion = "1.0b"
	coreVersion  = "1.1a"
	manufacturer = "Sony"
	productName  = "PXW-FS7"
	description  = "Camera Sony PXW-FS7"
	releaseDate  = "01.10.2020"
)

func main() {
	log.Info("Starting Application")

	coreInfo := ibeam_core.CoreInfo{
		IbeamVersion:     iBeamVersion,
		CoreVersion:      coreVersion,
		Description:      "",
		Name:             "",
		MaxDevices:       1,
		ConnectedClients: 0,
	}

	modelInfo := ibeam_core.ModelInfo{
		Id:             0,
		Name:           productName,
		Description:    description,
		ConnectionType: ibeam_core.ConnectionType_Network,
	}

	server, manager, registry, _, _ := CreateServer(coreInfo, modelInfo)
	manager.Start()

	registry.RegisterParameter(&ibeam_core.ParameterDetail{
		Id:                  nil, // choose default device
		Name:                "Iris",
		Instances:           1,
		Label:               "Iris",
		ShortLabel:          "Iris",
		Description:         "The Iris of the Camera",
		GenericType:         ibeam_core.GenericType_Iris,
		IsSpeedValue:        true,
		ControlStyle:        ibeam_core.ControlStyle_Normal,
		FeedbackStyle:       ibeam_core.FeedbackStyle_NormalFeedback,
		ControlDelayMs:      100,
		QuarantineDelayMs:   100,
		RetryCount:          10,
		ValueType:           ibeam_core.ValueType_Opt,
		DisplayStyle:        ibeam_core.DisplayStyle_DisplayFstop,
		OptionList:          generateOptionList([]string{"4", "4.5", "5", "5.6", "6.3", "7.1", "8", "9", "10", "11", "13", "14", "16", "18", "20", "22"}),
		OptionListIsDynamic: false,
	})

	registry.RegisterParameter(&ibeam_core.ParameterDetail{
		Id:                  nil, // choose default device
		Name:                "Test Parameter",
		Instances:           1,
		Label:               "Test",
		ShortLabel:          "Test",
		Description:         "The Test of the Camera",
		IsSpeedValue:        true,
		ControlStyle:        ibeam_core.ControlStyle_Normal,
		FeedbackStyle:       ibeam_core.FeedbackStyle_NormalFeedback,
		ControlDelayMs:      100,
		QuarantineDelayMs:   100,
		RetryCount:          10,
		ValueType:           ibeam_core.ValueType_Floating,
		DisplayStyle:        ibeam_core.DisplayStyle_DisplayFstop,
		OptionListIsDynamic: false,
	})

	registry.RegisterDevice(modelInfo.Id)

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", 5000))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	ibeam_core.RegisterIbeamCoreServer(grpcServer, &server)
	grpcServer.Serve(lis)
}

func generateOptionList(options []string) (OptionList *ibeam_core.OptionList) {
	OptionList = &ibeam_core.OptionList{}
	for index, option := range options {
		OptionList.Options = append(OptionList.Options, &ibeam_core.ParameterOption{
			Id:   uint32(index),
			Name: option,
		})
	}
	return
}
*/
