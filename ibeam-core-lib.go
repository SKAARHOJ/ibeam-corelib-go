package ibeam_core_lib

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	ibeam_core "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
	"google.golang.org/grpc"
)

// IbeamParameterRegistry is the storrage of the core.
// It saves all Infos about the Core, Device and Info and stores the Details and current Values of the Parameter.
type IbeamParameterRegistry struct {
	muInfo          sync.RWMutex
	muDetail        sync.RWMutex
	muValue         sync.RWMutex
	coreInfo        ibeam_core.CoreInfo
	DeviceInfos     []*ibeam_core.DeviceInfo
	ModelInfos      []*ibeam_core.ModelInfo
	ParameterDetail []map[int]*ibeam_core.ParameterDetail //Parameter Details: model, parameter
	parameterValue  []map[int][]*IbeamParameterDimension  //Parameter State: device,parameter,dimension
}

// IbeamServer implements the IbeamCoreServer interface of the generated protofile library.
type IbeamServer struct {
	parameterRegistry        *IbeamParameterRegistry
	clientsSetterStream      chan ibeam_core.Parameter
	serverClientsStream      chan ibeam_core.Parameter
	serverClientsDistributor map[chan ibeam_core.Parameter]bool
}

// IbeamParameterManager manages parameter changes.
type IbeamParameterManager struct {
	parameterRegistry   *IbeamParameterRegistry
	out                 chan ibeam_core.Parameter
	in                  chan ibeam_core.Parameter
	clientsSetterStream chan ibeam_core.Parameter
	serverClientsStream chan ibeam_core.Parameter
	server              *IbeamServer
}

// IBeamParameterValueBuffer is used for updating a ParameterValue.
// It holds a current and a target Value.
type IBeamParameterValueBuffer struct {
	instanceID     uint32
	available      bool
	isAssumedState bool
	lastUpdate     time.Time
	tryCount       uint32
	currentValue   ibeam_core.ParameterValue
	targetValue    ibeam_core.ParameterValue
	metaValues     []ibeam_core.ParameterMetaValue
}

type IbeamParameterDimension struct {
	isValue       bool
	subDimensions map[int]*IbeamParameterDimension
	value         *IBeamParameterValueBuffer
}

// TODO Add description
func (pd *IbeamParameterDimension) Value() (*IBeamParameterValueBuffer, error) {
	if pd.isValue {
		return pd.value, nil
	}
	return nil, errors.New("Dimension is not a value")
}

// TODO Add description
func (pd *IbeamParameterDimension) Subdimensions() (map[int]*IbeamParameterDimension, error) {
	if !pd.isValue {
		return pd.subDimensions, nil
	}
	return nil, errors.New("Dimension has no subdimension")
}

func (b *IBeamParameterValueBuffer) getParameterValue() *ibeam_core.ParameterValue {
	return &ibeam_core.ParameterValue{
		InstanceID:     b.instanceID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState,
		Value:          b.targetValue.Value,
		MetaValues:     b.currentValue.MetaValues,
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
		MetaValues: b.currentValue.MetaValues,
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
		MetaValues: b.currentValue.MetaValues,
	}
}

// device,parameter,instance
func (r *IbeamParameterRegistry) getInstanceValues(dpID ibeam_core.DeviceParameterID) (values []*ibeam_core.ParameterValue) {
	deviceIndex := int(dpID.Device) - 1
	parameterIndex := int(dpID.Parameter)

	r.muValue.RLock()
	defer r.muValue.RUnlock()

	if dpID.Device == 0 || dpID.Parameter == 0 || len(r.parameterValue) <= deviceIndex {
		log.Error("Could not get instance values for DeviceParameterID: Device:", dpID.Device, " and param: ", dpID.Parameter)
		r.muValue.RUnlock()
		return nil
	}

	if _, ok := r.parameterValue[deviceIndex][parameterIndex]; !ok {
		log.Error("Could not get instance values for DeviceParameterID: Device:", dpID.Device, " and param: ", dpID.Parameter)
		r.muValue.RUnlock()
		return nil
	}
	for _, value := range r.parameterValue[deviceIndex][parameterIndex] {
		values = append(values, value.value.getParameterValue())
	}

	return
}

func (r *IbeamParameterRegistry) getModelIndex(deviceID int) uint32 {
	r.muInfo.RLock()
	defer r.muInfo.RUnlock()
	if len(r.DeviceInfos) <= deviceID {
		log.Panicf("Could not get model for device with id %v", deviceID)
	}
	return r.DeviceInfos[deviceID].ModelID - 1
}

// GetCoreInfo returns the CoreInfo of the Ibeam-Core
func (s *IbeamServer) GetCoreInfo(_ context.Context, _ *ibeam_core.Empty) (*ibeam_core.CoreInfo, error) {
	return &s.parameterRegistry.coreInfo, nil
}

// GetDeviceInfo returns the DeviceInfos for given DeviceIDs.
// If no IDs are given, all DeviceInfos will be returned.
func (s *IbeamServer) GetDeviceInfo(_ context.Context, dIDs *ibeam_core.DeviceIDs) (*ibeam_core.DeviceInfos, error) {
	log.Debugf("Client asks for DeviceInfo with ids %v", dIDs.Ids)
	if len(dIDs.Ids) == 0 {
		return &ibeam_core.DeviceInfos{DeviceInfos: s.parameterRegistry.DeviceInfos}, nil
	}
	var rDeviceInfos ibeam_core.DeviceInfos
	for _, ID := range dIDs.Ids {
		if device := getDeviceWithID(s, ID); device != nil {
			rDeviceInfos.DeviceInfos = append(rDeviceInfos.DeviceInfos, device)
		}
		// If we have no Device with such a ID, skip
	}
	return &rDeviceInfos, nil

}

func getDeviceWithID(s *IbeamServer, dID uint32) *ibeam_core.DeviceInfo {
	for _, device := range s.parameterRegistry.DeviceInfos {
		if device.DeviceID == dID {
			return device
		}
	}
	return nil
}

// GetModelInfo returns the ModelInfos for given ModelIDs.
// If no IDs are given, all ModelInfos will be returned.
func (s *IbeamServer) GetModelInfo(_ context.Context, mIDs *ibeam_core.ModelIDs) (*ibeam_core.ModelInfos, error) {
	if len(mIDs.Ids) == 0 {
		return &ibeam_core.ModelInfos{ModelInfos: s.parameterRegistry.ModelInfos}, nil
	}
	var rModelInfos ibeam_core.ModelInfos
	for _, ID := range mIDs.Ids {
		if model := getModelWithID(s, ID); model != nil {
			rModelInfos.ModelInfos = append(rModelInfos.ModelInfos, model)
		}
		// If we have no Model with such an ID, skip
	}
	return &rModelInfos, nil
}

func getModelWithID(s *IbeamServer, mID uint32) *ibeam_core.ModelInfo {
	for _, model := range s.parameterRegistry.ModelInfos {
		if model.Id == mID {
			return model
		}
	}
	return nil
}

// Get returns the Parameters with their current state for given DeviceParameterIDs.
// If no IDs are given, all Parameters will be returned.
func (s *IbeamServer) Get(_ context.Context, dpIDs *ibeam_core.DeviceParameterIDs) (rParameters *ibeam_core.Parameters, err error) {
	rParameters = &ibeam_core.Parameters{}
	if len(dpIDs.Ids) == 0 {
		for did, dState := range s.parameterRegistry.parameterValue {
			for pid := range dState {
				dpID := ibeam_core.DeviceParameterID{
					Parameter: uint32(pid),
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
	} else if len(dpIDs.Ids) == 1 && dpIDs.Ids[0].Parameter == 0 && dpIDs.Ids[0].Device != 0 {
		did := dpIDs.Ids[0].Device - 1
		if len(s.parameterRegistry.parameterValue) <= int(did) {
			rParameters.Parameters = append(rParameters.Parameters, &ibeam_core.Parameter{
				Id:    dpIDs.Ids[0],
				Error: ibeam_core.ParameterError_UnknownID,
				Value: nil,
			})
			return
		}

		for pid := range s.parameterRegistry.parameterValue[did] {
			dpID := ibeam_core.DeviceParameterID{
				Parameter: uint32(pid),
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

// GetParameterDetails returns the Details for given ModelParameterIDs.
// If no IDs are given, the Details from all Parameters will be returned.
func (s *IbeamServer) GetParameterDetails(c context.Context, mpIDs *ibeam_core.ModelParameterIDs) (*ibeam_core.ParameterDetails, error) {
	log.Debugf("Got a GetParameterDetails from ") //TODO lookup how to get IP
	rParameterDetails := &ibeam_core.ParameterDetails{}
	s.parameterRegistry.muInfo.RLock()
	defer s.parameterRegistry.muInfo.RUnlock()

	if len(mpIDs.Ids) == 0 {
		for _, modelDetails := range s.parameterRegistry.ParameterDetail {
			for _, modelDetail := range modelDetails {
				rParameterDetails.Details = append(rParameterDetails.Details, modelDetail)
			}
		}
	} else if len(mpIDs.Ids) == 1 && int(mpIDs.Ids[0].Parameter) == 0 {
		// Return all parameters for model
		if int(mpIDs.Ids[0].Model) != 0 && len(s.parameterRegistry.ParameterDetail) >= int(mpIDs.Ids[0].Model) {
			for _, modelDetail := range s.parameterRegistry.ParameterDetail[mpIDs.Ids[0].Model-1] {
				rParameterDetails.Details = append(rParameterDetails.Details, modelDetail)
			}
		} else {
			log.Error("Invalid model ID specified")
		}
	} else {
		for _, mpID := range mpIDs.Ids {
			d, err := s.getParameterDetail(mpID)
			if err != nil {
				log.Errorf(err.Error())
				return nil, err
			}
			rParameterDetails.Details = append(rParameterDetails.Details, d)

		}
	}
	log.Debugf("Send ParameterDetails for %v parameters", rParameterDetails.Details[0].Name)
	return rParameterDetails, nil
}

func (s *IbeamServer) getParameterDetail(mpID *ibeam_core.ModelParameterID) (*ibeam_core.ParameterDetail, error) {
	if mpID.Model == 0 || mpID.Parameter == 0 {
		return nil, errors.New("Failed to get instance values " + mpID.String())
	}
	if len(s.parameterRegistry.ParameterDetail) < int(mpID.Model) {
		return nil, fmt.Errorf("ParamerDetail does not have Model with id %v", mpID.Model)
	}
	for _, parameterDetail := range s.parameterRegistry.ParameterDetail[mpID.Model-1] {
		if parameterDetail.Id.Model == mpID.Model && parameterDetail.Id.Parameter == mpID.Parameter {
			return parameterDetail, nil
		}
	}
	return nil, errors.New("Cannot find ParameterDetail with given ModelParameterID")
}

// Set will change the Value for the given Parameter.
func (s *IbeamServer) Set(_ context.Context, ps *ibeam_core.Parameters) (*ibeam_core.Empty, error) {
	for _, parameter := range ps.Parameters {
		s.clientsSetterStream <- *parameter
	}
	return &ibeam_core.Empty{}, nil
}

// Subscribe starts a ServerStream and send updates if a Parameter changes.
// On subscribe all current values should be sent back.
// If no IDs are given, subscribe to every Parameter.
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

	ping := time.NewTicker(time.Second)
	for {
		select {
		// TODO check if Stream is closed
		case <-ping.C:
			err := stream.Context().Err()
			if err != nil {
				s.serverClientsDistributor[distributor] = false
				log.Warn("Connection to client for subscription lost")
				return nil
			}
		case parameter := <-distributor:
			if parameter.Id == nil || parameter.Id.Device == 0 || parameter.Id.Parameter == 0 {
				continue
			}
			// Check if Device is Subscribed
			if len(dpIDs.Ids) == 1 && dpIDs.Ids[0].Device != parameter.Id.Device {
				continue
			}

			// Check for parameter filtering
			if len(dpIDs.Ids) > 1 && !containsDeviceParameter(parameter.Id, dpIDs) {
				continue
			}
			log.Debugf("Send Parameter with ID '%v' to client from ServerClientsStream", parameter.Id)
			stream.Send(&parameter)
		}
	}
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

// CreateServer sets up the ibeam server, parameter manager and parameter registry
func CreateServer(coreInfo ibeam_core.CoreInfo, defaultModel ibeam_core.ModelInfo) (manager *IbeamParameterManager, registry *IbeamParameterRegistry, setToGRPC chan ibeam_core.Parameter, getFromGRPC chan ibeam_core.Parameter) {
	clientsSetter := make(chan ibeam_core.Parameter, 100)
	getFromGRPC = make(chan ibeam_core.Parameter, 100)
	setToGRPC = make(chan ibeam_core.Parameter, 100)

	fistParameter := ibeam_core.Parameter{}
	fistParameter.Id = &ibeam_core.DeviceParameterID{
		Device:    0,
		Parameter: 0,
	}

	watcher := make(chan ibeam_core.Parameter)

	coreInfo.IbeamVersion = ibeam_core.File_ibeam_core_proto.Options().ProtoReflect().Get(ibeam_core.E_IbeamVersion.TypeDescriptor()).String()

	registry = &IbeamParameterRegistry{
		coreInfo:        coreInfo,
		DeviceInfos:     []*ibeam_core.DeviceInfo{},
		ModelInfos:      []*ibeam_core.ModelInfo{},
		ParameterDetail: []map[int]*ibeam_core.ParameterDetail{},
		parameterValue:  []map[int][]*IbeamParameterDimension{},
	}

	server := IbeamServer{
		parameterRegistry:        registry,
		clientsSetterStream:      clientsSetter,
		serverClientsStream:      watcher,
		serverClientsDistributor: make(map[chan ibeam_core.Parameter]bool),
	}

	manager = &IbeamParameterManager{
		parameterRegistry:   registry,
		out:                 getFromGRPC,
		in:                  setToGRPC,
		clientsSetterStream: clientsSetter,
		serverClientsStream: watcher,
		server:              &server,
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

// RegisterParameter registers a Parameter and his Details in the Registry.
func (r *IbeamParameterRegistry) RegisterParameter(detail *ibeam_core.ParameterDetail) (parameterIndex uint32) {
	mid := uint32(1)
	parameterIndex = uint32(0)
	if detail.Id != nil {
		if detail.Id.Model != 0 {
			mid = detail.Id.Model
		}
		parameterIndex = detail.Id.Parameter
	}
	r.muDetail.RLock()
	if uint32(len(r.ParameterDetail)) <= (mid - 1) {
		log.Panic("Could not register parameter for nonexistent model ", mid)
		return 0
	}

	if mid == 1 {
		// append to all models, need to check for ids
		defaultModelConfig := &r.ParameterDetail[0]
		if parameterIndex == 0 {
			parameterIndex = uint32(len(*defaultModelConfig) + 1)
		}
		r.muDetail.RUnlock()
		detail.Id = &ibeam_core.ModelParameterID{
			Parameter: parameterIndex,
			Model:     mid,
		}
		r.muDetail.Lock()
		for _, modelconfig := range r.ParameterDetail {
			(modelconfig)[int(parameterIndex)] = detail
		}
		r.muDetail.Unlock()

	} else {
		modelconfig := &r.ParameterDetail[mid-1]
		if parameterIndex == 0 {
			parameterIndex = uint32(len(*modelconfig) + 1)
		}
		r.muDetail.RUnlock()
		detail.Id = &ibeam_core.ModelParameterID{
			Parameter: parameterIndex,
			Model:     mid,
		}
		r.muDetail.Lock()
		(*modelconfig)[int(parameterIndex)] = detail
		r.muDetail.Unlock()
	}

	log.Debugf("ParameterDetail '%v' registered with ID: %v for Model %v", detail.Name, detail.Id.Parameter, detail.Id.Model)
	return
}

// RegisterParameters registers multiple Parameter and their Details in the Registry
func (r *IbeamParameterRegistry) RegisterParameters(details *ibeam_core.ParameterDetails) (ids []uint32) {
	for _, detail := range details.Details {
		ids = append(ids, r.RegisterParameter(detail))
	}
	return
}

// RegisterModel registers a new Model in the Registry with given ModelInfo
func (r *IbeamParameterRegistry) RegisterModel(model *ibeam_core.ModelInfo) uint32 {
	r.muDetail.RLock()
	model.Id = uint32(len(r.ParameterDetail) + 1)
	r.muDetail.RUnlock()

	r.muInfo.Lock()
	r.ModelInfos = append(r.ModelInfos, model)
	r.muInfo.Unlock()

	r.muDetail.Lock()
	r.ParameterDetail = append(r.ParameterDetail, map[int]*ibeam_core.ParameterDetail{})
	r.muDetail.Unlock()

	log.Debugf("Model '%v' registered with ID: %v ", model.Name, model.Id)
	return model.Id
}

// RegisterDevice registers a new Device in the Registry with given ModelID
func (r *IbeamParameterRegistry) RegisterDevice(modelID uint32) (deviceIndex uint32) {
	if modelID == 0 {
		modelID = uint32(1)
	}
	r.muDetail.RLock()
	if uint32(len(r.ParameterDetail)) <= (modelID - 1) {
		log.Panicf("Could not register device for nonexistent model with id: %v", modelID)
		return 0
	}

	modelConfig := r.ParameterDetail[modelID-1]
	r.muDetail.RUnlock()

	// create device info
	// take all params from model and generate a value buffer array for all instances
	// add value buffers to the state array

	parameterDimensions := &map[int][]*IbeamParameterDimension{}
	for _, parameterDetail := range modelConfig {
		valueDimensions := []*IbeamParameterDimension{}

		// Integer is default
		initialValue := ibeam_core.ParameterValue{Value: &ibeam_core.ParameterValue_Integer{Integer: 0}}

		switch parameterDetail.ValueType {
		case ibeam_core.ValueType_NoValue:
			initialValue.Value = &ibeam_core.ParameterValue_Cmd{Cmd: ibeam_core.Command_Trigger}
		case ibeam_core.ValueType_Floating:
			initialValue.Value = &ibeam_core.ParameterValue_Floating{Floating: 0.0}
		case ibeam_core.ValueType_Opt:
			initialValue.Value = &ibeam_core.ParameterValue_CurrentOption{CurrentOption: 0}
		case ibeam_core.ValueType_String:
			initialValue.Value = &ibeam_core.ParameterValue_Str{Str: ""}
		case ibeam_core.ValueType_Binary:
			initialValue.Value = &ibeam_core.ParameterValue_Binary{Binary: false}
		}

		// TODO Replace instances

		for i := uint32(0); i < parameterDetail.Instances; i++ {
			valueDimensions = append(valueDimensions, &IbeamParameterDimension{
				isValue: true,
				value: &IBeamParameterValueBuffer{
					instanceID:     i + 1, // TODO
					available:      true,
					isAssumedState: true,
					lastUpdate:     time.Now(),
					currentValue:   initialValue,
					targetValue:    initialValue,
				},
			})
		}

		(*parameterDimensions)[int(parameterDetail.Id.Parameter)] = valueDimensions
	}

	r.muInfo.RLock()
	deviceIndex = uint32(len(r.DeviceInfos) + 1)
	r.muInfo.RUnlock()
	r.muInfo.Lock()
	r.DeviceInfos = append(r.DeviceInfos, &ibeam_core.DeviceInfo{
		DeviceID: deviceIndex,
		ModelID:  modelID,
	})
	r.muInfo.Unlock()
	r.muValue.Lock()
	r.parameterValue = append(r.parameterValue, *parameterDimensions)
	r.muValue.Unlock()

	log.Debugf("Device '%v' registered with model: %v (%v)", deviceIndex, modelID, r.ModelInfos[modelID-1].Name)
	return
}

// GetIDMaps returns a Map witch maps the Name of all Parameters with their ID for each model
func (r *IbeamParameterRegistry) GetIDMaps() []map[string]uint32 {
	idMaps := make([]map[string]uint32, 0)
	r.muDetail.RLock()

	for mIndex := range r.ModelInfos {
		idMap := make(map[string]uint32)
		for _, parameter := range r.ParameterDetail[mIndex] {
			idMap[parameter.Name] = parameter.Id.Parameter
		}
		idMaps = append(idMaps, idMap)
	}
	r.muDetail.RUnlock()
	return idMaps
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
	ibeam_core.RegisterIbeamCoreServer(grpcServer, m.server)
	grpcServer.Serve(lis)
}

// Start the communication between client and server.
func (m *IbeamParameterManager) Start() {
	go func() {
		for {
			select {
			case parameter := <-m.clientsSetterStream:
				// Client set loop, inputs set requests from grpc to manager

				// Check if given Parameter has an DeviceParameterID
				if parameter.Id == nil {
					log.Errorf("Client tries to set a parameter without an ID")
					// Todo check if this isn´t better for debug on client side.
					// Client sees what he has send
					parameter.Error = ibeam_core.ParameterError_UnknownID
					m.serverClientsStream <- parameter
					/*
						m.serverClientsStream <- ibeam_core.Parameter{
							Id:    parameter.Id,
							Error: ibeam_core.ParameterError_UnknownID,
							Value: []*ibeam_core.ParameterValue{},
						}*/
					continue
				}

				// Get Index and ID for Device and Parameter and the actual state of all parameters
				m.parameterRegistry.muValue.RLock()
				deviceIndex := int(parameter.Id.Device) - 1
				parameterID := int(parameter.Id.Parameter)
				state := m.parameterRegistry.parameterValue
				m.parameterRegistry.muValue.RUnlock()

				// Check if ID and Index are valid and in the State
				if _, ok := state[deviceIndex][parameterID]; !ok {
					log.Errorf("Client uses an invalid DeviceID %d for Parameter %d", parameter.Id.Device, parameterID)
					m.serverClientsStream <- ibeam_core.Parameter{
						Id:    parameter.Id,
						Error: ibeam_core.ParameterError_UnknownID,
						Value: []*ibeam_core.ParameterValue{},
					}
					continue
				}

				// Get Index of the Model and the Configuration (Details) of the Parameter
				m.parameterRegistry.muDetail.RLock()
				modelIndex := m.parameterRegistry.getModelIndex(deviceIndex)
				parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterID]
				m.parameterRegistry.muDetail.RUnlock()

				// Check if the configured type of the Parameter has a value
				if parameterConfig.ValueType == ibeam_core.ValueType_NoValue {
					log.Errorf("Client want to set Parameter with ID %v (%v), but it is configured as NoValue Type", parameterID, parameterConfig.Name)
					m.serverClientsStream <- ibeam_core.Parameter{
						Id:    parameter.Id,
						Error: ibeam_core.ParameterError_HasNoValue,
						Value: []*ibeam_core.ParameterValue{},
					}
					continue
				}

				// Handle every Value in that was given for the Parameter
				for _, newParameterValue := range parameter.Value {

					// Check if the NewValue has a Value
					if newParameterValue.Value == nil {
						// TODO should this be allowed or will we send an error back?
						continue
					}

					// Check if Instance of the Value is valid
					if newParameterValue.InstanceID == 0 || len(state[deviceIndex][parameterID]) < int(newParameterValue.InstanceID) {
						log.Errorf("Received invalid InstanceId %d for parameter %d on device %d", newParameterValue.InstanceID, parameterID, parameter.Id.Device)
						m.serverClientsStream <- ibeam_core.Parameter{
							Id:    parameter.Id,
							Error: ibeam_core.ParameterError_UnknownID,
							Value: []*ibeam_core.ParameterValue{},
						}
						continue
					}

					// Check if Value is valid and has the right Type
					switch newValue := newParameterValue.Value.(type) {
					case *ibeam_core.ParameterValue_Integer:

						if parameterConfig.ValueType != ibeam_core.ValueType_Integer {
							log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, ibeam_core.ValueType_name[int32(parameterConfig.ValueType)])
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_InvalidType,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}

						log.Debugf("Set new Value Type Integer: %v", *newValue)

						if newValue.Integer > int32(parameterConfig.Maximum) {
							log.Errorf("Max violation for parameter %v", parameterID)
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_MaxViolation,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}
						if newValue.Integer < int32(parameterConfig.Minimum) {
							log.Errorf("Min violation for parameter %v", parameterID)
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_MinViolation,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}
					case *ibeam_core.ParameterValue_IncDecSteps:
						if parameterConfig.ValueType != ibeam_core.ValueType_Integer {
							log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, ibeam_core.ValueType_name[int32(parameterConfig.ValueType)])
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_InvalidType,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}

						log.Debugf("Set new Value Type IncDecSteps: %v", *newValue)

						if newValue.IncDecSteps > parameterConfig.IncDecStepsUpperRange || newValue.IncDecSteps < parameterConfig.IncDecStepsLowerRange {
							log.Errorf("In- or Decrementation Step %v is outside of the range [%v,%v] of the parameter %v", newValue.IncDecSteps, parameterConfig.IncDecStepsLowerRange, parameterConfig.IncDecStepsUpperRange, parameterID)
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_RangeViolation,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}
					case *ibeam_core.ParameterValue_Floating:
						if parameterConfig.ValueType != ibeam_core.ValueType_Floating {
							log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, ibeam_core.ValueType_name[int32(parameterConfig.ValueType)])
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_InvalidType,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}

						log.Debugf("Set new Value Type Float: %v", newValue)

						if newValue.Floating > parameterConfig.Maximum {
							log.Errorf("Max violation for parameter %v", parameterID)
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_MaxViolation,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}
						if newValue.Floating < parameterConfig.Minimum {
							log.Errorf("Min violation for parameter %v", parameterID)
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_MinViolation,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}
					case *ibeam_core.ParameterValue_Str:
						if parameterConfig.ValueType != ibeam_core.ValueType_String {
							log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, ibeam_core.ValueType_name[int32(parameterConfig.ValueType)])
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_InvalidType,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}

						log.Debugf("Set new Value Type String: %v", newValue)

						// String does not need extra check

					case *ibeam_core.ParameterValue_CurrentOption:

						// TODO not shure...
						if parameterConfig.ValueType != ibeam_core.ValueType_Opt {
							log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, ibeam_core.ValueType_name[int32(parameterConfig.ValueType)])
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_InvalidType,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}

						log.Debugf("Set new Value Type Current Option: %v", newValue)
						if parameterConfig.OptionList == nil {
							log.Errorf("No option List found for Parameter %v", newValue)
							continue
						}
						if newValue.CurrentOption > uint32(len(parameterConfig.OptionList.Options)) {
							log.Errorf("Invalid operation index for parameter %v", parameterID)
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_UnknownID,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}
					case *ibeam_core.ParameterValue_Cmd:
						// Command can be send directly to the output
						log.Debugf("Set new Value Type CMD: %v", newValue)
						m.out <- ibeam_core.Parameter{
							Id:    parameter.Id,
							Error: 0,
							Value: []*ibeam_core.ParameterValue{newParameterValue},
						}
					case *ibeam_core.ParameterValue_Binary:

						if parameterConfig.ValueType != ibeam_core.ValueType_Binary {
							log.Errorf("Got Value with Type %T for Parameter %v (%v), but it needs %v", newValue, parameterID, parameterConfig.Name, ibeam_core.ValueType_name[int32(parameterConfig.ValueType)])
							m.serverClientsStream <- ibeam_core.Parameter{
								Id:    parameter.Id,
								Error: ibeam_core.ParameterError_InvalidType,
								Value: []*ibeam_core.ParameterValue{},
							}
							continue
						}

						log.Debugf("Got Set Binary: %v", newValue)
					case *ibeam_core.ParameterValue_OptionList:
						log.Debugf("Got Set Option List: %v", newValue)
						//TODO check if Option list is valid (unique ids etc.)
					}

					// Safe the momentary saved Value of the Parameter in the state
					parameterDimension := state[deviceIndex][parameterID][newParameterValue.InstanceID-1]
					parameterBuffer, err := parameterDimension.Value()
					if err != nil {
						parameterSubdimensions, err := parameterDimension.Subdimensions()
						if err != nil {
							log.Errorf("Parameter %v has no Value and no SubDimension", parameterID)
						}
						for _, parameterSubdimension := range parameterSubdimensions {
							parameterSubdimension.Value()
							// TODO
						}
					}

					log.Debugf("Set new TargetValue '%v', for Parameter %v (%v)", newParameterValue.Value, parameterID, parameterConfig.Name)

					parameterBuffer.isAssumedState = newParameterValue.Value != parameterBuffer.currentValue.Value
					parameterBuffer.targetValue.Value = newParameterValue.Value
					parameterBuffer.tryCount = 0
				}

			case parameter := <-m.in:
				// Param Ingest loop, inputs changes from a device (f.e. a camera) to manager
				m.parameterRegistry.muValue.RLock()
				m.parameterRegistry.muDetail.RLock()
				state := m.parameterRegistry.parameterValue
				// Get Index and ID for Device and Parameter
				deviceIndex := int(parameter.Id.Device) - 1
				parameterID := int(parameter.Id.Parameter)
				// Get Index of the Model and the Configuration of the Parameter
				modelIndex := m.parameterRegistry.getModelIndex(deviceIndex)
				parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterID]

				shouldSend := false
				m.parameterRegistry.muValue.RUnlock()
				m.parameterRegistry.muDetail.RUnlock()

				// Check if given Parameter has an DeviceParameterID
				if parameter.Id == nil {
					log.Errorf("Device tries to set parameter without id")
					continue
				}

				// Check if ID and Index are Valid and in the State
				if _, ok := state[deviceIndex][parameterID]; !ok || deviceIndex < 0 || len(state) <= deviceIndex {
					log.Errorf("Device uses an invalid DeviceID %d for Parameter %d", parameter.Id.Device, parameterID)
					continue
				}

				for _, value := range parameter.Value {
					if value.InstanceID == 0 || len(state[deviceIndex][parameterID]) < int(value.InstanceID) {
						log.Errorf("Received invalid instance id %v for parameter %v", value.InstanceID, parameterID)
						continue
					}
					parameterDimension := state[deviceIndex][parameterID][value.InstanceID-1]
					parameterBuffer, err := parameterDimension.Value()
					if err != nil {
						// TODO
					}
					if value.Value != nil {
						didSet := false // flag to to handle custom cases
						// If Type of Parameter is Opt, find the right Opt
						switch parameterConfig.ValueType {
						case ibeam_core.ValueType_Opt:
							switch v := value.Value.(type) {
							case *ibeam_core.ParameterValue_Str:
								id, err := getIDFromOptionListByElementName(parameterConfig.OptionList, v.Str)
								if err != nil {
									log.Error(err)
									// TODO get new option list maybe
									continue
								}

								newValue := ibeam_core.ParameterValue{
									Value: &ibeam_core.ParameterValue_CurrentOption{
										CurrentOption: id,
									},
								}
								if time.Since(parameterBuffer.lastUpdate).Milliseconds() > int64(parameterConfig.QuarantineDelayMs) {
									parameterBuffer.targetValue = newValue
								}
								parameterBuffer.currentValue = newValue

								didSet = true
							case *ibeam_core.ParameterValue_OptionList:
								if !parameterConfig.OptionListIsDynamic {
									log.Errorf("Parameter with ID %v has no Dynamic OptionList", parameter.Id.Parameter)
									continue
								}
								m.parameterRegistry.muDetail.Lock()
								m.parameterRegistry.ParameterDetail[modelIndex][parameterID].OptionList = v.OptionList
								m.parameterRegistry.muDetail.Unlock()

								m.serverClientsStream <- ibeam_core.Parameter{Value: []*ibeam_core.ParameterValue{value}, Id: parameter.Id, Error: 0}
								continue
							case *ibeam_core.ParameterValue_CurrentOption:
								// Handled below
							default:
								log.Errorf("Valuetype of Parameter is Opt and so we should get a String or Opt or currentOpt, but got %T", value)
								continue
							}
						case ibeam_core.ValueType_Binary:
							if _, ok := value.Value.(*ibeam_core.ParameterValue_Binary); !ok {
								log.Errorf("Parameter with ID %v is Type Binary but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
								continue
							}
						case ibeam_core.ValueType_Floating:
							if _, ok := value.Value.(*ibeam_core.ParameterValue_Floating); !ok {
								log.Errorf("Parameter with ID %v is Type Float but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
								continue
							}
						case ibeam_core.ValueType_Integer:
							if _, ok := value.Value.(*ibeam_core.ParameterValue_Integer); !ok {
								log.Errorf("Parameter with ID %v is Type Integer but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
								continue
							}
						case ibeam_core.ValueType_String:
							if _, ok := value.Value.(*ibeam_core.ParameterValue_Str); !ok {
								log.Errorf("Parameter with ID %v is Type String but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
								continue
							}
						case ibeam_core.ValueType_NoValue:
							log.Errorf("Parameter with ID %v has No Value but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
							continue
						}

						if !didSet {
							if time.Since(parameterBuffer.lastUpdate).Milliseconds() > int64(parameterConfig.QuarantineDelayMs) {
								parameterBuffer.targetValue.Value = value.Value
							}
							parameterBuffer.currentValue.Value = value.Value
						}

						parameterBuffer.isAssumedState = parameterBuffer.currentValue.Value != parameterBuffer.targetValue.Value
					} else {
						parameterBuffer.available = value.Available
					}
					shouldSend = true
				}

				if !shouldSend {
					continue
				}

				if values := m.parameterRegistry.getInstanceValues(*parameter.GetId()); values != nil {
					m.serverClientsStream <- ibeam_core.Parameter{Value: values, Id: parameter.Id, Error: 0}
				}
			}

			// Main Loop
			m.parameterRegistry.muValue.RLock()
			state := m.parameterRegistry.parameterValue
			m.parameterRegistry.muValue.RUnlock()

			m.parameterRegistry.muInfo.RLock()
			for _, deviceInfo := range m.parameterRegistry.DeviceInfos {

				deviceID := deviceInfo.DeviceID

				m.parameterRegistry.muDetail.RLock()
				for _, parameterDetail := range m.parameterRegistry.ParameterDetail[deviceInfo.ModelID-1] {

					// Check if Parameter has a Control Style
					if parameterDetail.ControlStyle == ibeam_core.ControlStyle_Undefined {
						continue
					}

					// Check if Parameter ID is given
					if parameterDetail.Id == nil {
						//log.Error("No Parameter ID provided")
						continue
					}

					for instanceID := 0; instanceID < int(parameterDetail.Instances); instanceID++ {

						parameterDimension := state[deviceID-1][int(parameterDetail.Id.Parameter)][instanceID]
						parameterBuffer, err := parameterDimension.Value()
						if err != nil {
							//TODO
						}
						// ***************************************************************
						// First Basic Check Pipeline if the Parameter Value can be send to out
						// ***************************************************************

						// Is the Value the Same like stored?
						if parameterBuffer.currentValue.Value == parameterBuffer.targetValue.Value {
							//log.Errorf("Failed to set parameter %v '%v' for device %v, CurrentValue '%v' and TargetValue '%v' are the same", parameterDetail.Id.Parameter, parameterDetail.Name, deviceID+1, parameterBuffer.currentValue.Value, parameterBuffer.targetValue.Value)
							continue
						}

						// Is is send after Control Delay time
						if time.Until(parameterBuffer.lastUpdate).Milliseconds()*-1 < int64(parameterDetail.ControlDelayMs) {
							//log.Errorf("Failed to set parameter %v '%v' for device %v, ControlDelayTime", parameterDetail.Id.Parameter, parameterDetail.Name, deviceID+1, parameterDetail.ControlDelayMs)
							continue
						}

						// Is is send after Quarantine Delay time
						if time.Until(parameterBuffer.lastUpdate).Milliseconds()*-1 < int64(parameterDetail.QuarantineDelayMs) {
							//log.Errorf("Failed to set parameter %v '%v' for device %v, QuarantineDelayTime", parameterDetail.Id.Parameter, parameterDetail.Name, deviceID+1, parameterDetail.QuarantineDelayMs)
							continue
						}

						// Is the Retry Limit reached
						if parameterDetail.RetryCount != 0 && parameterDetail.FeedbackStyle != ibeam_core.FeedbackStyle_NoFeedback {
							parameterBuffer.tryCount++
							if parameterBuffer.tryCount > parameterDetail.RetryCount {
								log.Errorf("Failed to set parameter %v '%v' in %v tries on device %v", parameterDetail.Id.Parameter, parameterDetail.Name, parameterDetail.RetryCount, deviceID+1)
								parameterBuffer.targetValue = parameterBuffer.currentValue
								parameterBuffer.isAssumedState = false

								m.serverClientsStream <- ibeam_core.Parameter{
									Value: []*ibeam_core.ParameterValue{parameterBuffer.getParameterValue()},
									Id: &ibeam_core.DeviceParameterID{
										Device:    deviceID,
										Parameter: parameterDetail.Id.Parameter,
									},
									Error: ibeam_core.ParameterError_MaxRetrys,
								}
								continue
							}
						}

						// Set the lastUpdate Time
						parameterBuffer.lastUpdate = time.Now()

						switch parameterDetail.ControlStyle {
						case ibeam_core.ControlStyle_Normal:

							parameterBuffer.isAssumedState = true

							if parameterDetail.FeedbackStyle == ibeam_core.FeedbackStyle_NoFeedback {
								parameterBuffer.currentValue = parameterBuffer.targetValue
								parameterBuffer.isAssumedState = false
							}

							// If we Have a current Option, get the Value for the option from the Option List
							if parameterDetail.ValueType == ibeam_core.ValueType_Opt {
								if value, ok := parameterBuffer.targetValue.Value.(*ibeam_core.ParameterValue_CurrentOption); ok {
									name, err := getElementNameFromOptionListByID(parameterDetail.OptionList, *value)
									if err != nil {
										log.Error(err)
										continue
									}
									parameterBuffer.targetValue.Value = &ibeam_core.ParameterValue_Str{Str: name}
								}
							}

							m.out <- ibeam_core.Parameter{
								Value: []*ibeam_core.ParameterValue{parameterBuffer.getParameterValue()},
								Id: &ibeam_core.DeviceParameterID{
									Device:    deviceID,
									Parameter: parameterDetail.Id.Parameter,
								},
							}

							if parameterDetail.FeedbackStyle == ibeam_core.FeedbackStyle_DelayedFeedback ||
								parameterDetail.FeedbackStyle == ibeam_core.FeedbackStyle_NoFeedback {
								// send out assumed value immediately
								m.serverClientsStream <- ibeam_core.Parameter{
									Value: []*ibeam_core.ParameterValue{parameterBuffer.getParameterValue()},
									Id: &ibeam_core.DeviceParameterID{
										Device:    deviceID,
										Parameter: parameterDetail.Id.Parameter,
									},
								}
							}

						case ibeam_core.ControlStyle_IncrementalWithSteps:

							m.out <- ibeam_core.Parameter{
								Value: []*ibeam_core.ParameterValue{parameterBuffer.getParameterValue()},
								Id: &ibeam_core.DeviceParameterID{
									Device:    deviceID,
									Parameter: parameterDetail.Id.Parameter,
								},
							}
						case ibeam_core.ControlStyle_ControlledIncremental:
							if parameterDetail.FeedbackStyle == ibeam_core.FeedbackStyle_NoFeedback {
								continue
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
							case *ibeam_core.ParameterValue_Integer:
								if targetValue, ok := parameterBuffer.targetValue.Value.(*ibeam_core.ParameterValue_Integer); ok {
									if value.Integer < targetValue.Integer {
										cmdAction = Increment
									} else if value.Integer > targetValue.Integer {
										cmdAction = Decrement
									} else {
										cmdAction = NoOperation
									}
								}
							case *ibeam_core.ParameterValue_Floating:
								if targetValue, ok := parameterBuffer.targetValue.Value.(*ibeam_core.ParameterValue_Floating); ok {
									if value.Floating < targetValue.Floating {
										cmdAction = Increment
									} else if value.Floating > targetValue.Floating {
										cmdAction = Decrement
									} else {
										cmdAction = NoOperation
									}
								}
							case *ibeam_core.ParameterValue_CurrentOption:
								if targetValue, ok := parameterBuffer.targetValue.Value.(*ibeam_core.ParameterValue_CurrentOption); ok {
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

							log.Debugf("Send value '%v' to Device", cmdValue)

							m.out <- ibeam_core.Parameter{
								Id: &ibeam_core.DeviceParameterID{
									Device:    deviceID,
									Parameter: parameterDetail.Id.Parameter,
								},
								Error: 0,
								Value: cmdValue,
							}
						case ibeam_core.ControlStyle_NoControl, ibeam_core.ControlStyle_Incremental, ibeam_core.ControlStyle_Oneshot:
							// Do Nothing
						default:
							log.Errorf("Could not match controlltype")
							continue
						}
					}

				}
				m.parameterRegistry.muDetail.RUnlock()
			}
			m.parameterRegistry.muInfo.RUnlock()
		}
	}()
}
