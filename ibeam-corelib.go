package ibeam_corelib

import (
	"context"
	"errors"
	"fmt"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
)

// IbeamServer implements the IbeamCoreServer interface of the generated protofile library.
type IbeamServer struct {
	parameterRegistry        *IbeamParameterRegistry
	clientsSetterStream      chan pb.Parameter
	serverClientsStream      chan pb.Parameter
	serverClientsDistributor map[chan pb.Parameter]bool
}

// GetCoreInfo returns the CoreInfo of the Ibeam-Core
func (s *IbeamServer) GetCoreInfo(_ context.Context, _ *pb.Empty) (*pb.CoreInfo, error) {
	return &s.parameterRegistry.coreInfo, nil
}

// GetDeviceInfo returns the DeviceInfos for given DeviceIDs.
// If no IDs are given, all DeviceInfos will be returned.
func (s *IbeamServer) GetDeviceInfo(_ context.Context, deviceIDs *pb.DeviceIDs) (*pb.DeviceInfos, error) {

	log.Debugf("Client asks for DeviceInfo with ids %v", deviceIDs.Ids)

	if len(deviceIDs.Ids) == 0 {
		return &pb.DeviceInfos{DeviceInfos: s.parameterRegistry.DeviceInfos}, nil
	}

	var rDeviceInfos pb.DeviceInfos
	for _, deviceID := range deviceIDs.Ids {
		if len(s.parameterRegistry.DeviceInfos) >= int(deviceID) && deviceID > 0 {
			rDeviceInfos.DeviceInfos = append(rDeviceInfos.DeviceInfos, s.parameterRegistry.DeviceInfos[deviceID-1])
		} else {
			// TODO:Decide if we should  send an error and no ID or something different
			// return nil, fmt.Errorf("No device with ID %v", deviceID)
		}
		// If we have no Device with such a ID, skip
	}
	return &rDeviceInfos, nil

}

// GetModelInfo returns the ModelInfos for given ModelIDs.
// If no IDs are given, all ModelInfos will be returned.
func (s *IbeamServer) GetModelInfo(_ context.Context, mIDs *pb.ModelIDs) (*pb.ModelInfos, error) {
	if len(mIDs.Ids) == 0 {
		return &pb.ModelInfos{ModelInfos: s.parameterRegistry.ModelInfos}, nil
	}
	var rModelInfos pb.ModelInfos
	for _, ID := range mIDs.Ids {
		if model := getModelWithID(s, ID); model != nil {
			rModelInfos.ModelInfos = append(rModelInfos.ModelInfos, model)
		}
		// If we have no Model with such an ID, skip
	}
	return &rModelInfos, nil
}

func getModelWithID(s *IbeamServer, mID uint32) *pb.ModelInfo {
	for _, model := range s.parameterRegistry.ModelInfos {
		if model.Id == mID {
			return model
		}
	}
	return nil
}

// Get returns the Parameters with their current state for given DeviceParameterIDs.
// If no IDs are given, all Parameters will be returned.
func (s *IbeamServer) Get(_ context.Context, dpIDs *pb.DeviceParameterIDs) (rParameters *pb.Parameters, err error) {
	rParameters = &pb.Parameters{}
	if len(dpIDs.Ids) == 0 {
		for did, dState := range s.parameterRegistry.parameterValue {
			for pid := range dState {
				dpID := pb.DeviceParameterID{
					Parameter: uint32(pid),
					Device:    uint32(did) + 1,
				}
				iv := s.parameterRegistry.getInstanceValues(dpID)
				if iv != nil {
					rParameters.Parameters = append(rParameters.Parameters, &pb.Parameter{
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
			rParameters.Parameters = append(rParameters.Parameters, &pb.Parameter{
				Id:    dpIDs.Ids[0],
				Error: pb.ParameterError_UnknownID,
				Value: nil,
			})
			return
		}

		for pid := range s.parameterRegistry.parameterValue[did] {
			dpID := pb.DeviceParameterID{
				Parameter: uint32(pid),
				Device:    uint32(did) + 1,
			}
			iv := s.parameterRegistry.getInstanceValues(dpID)
			if iv != nil {
				rParameters.Parameters = append(rParameters.Parameters, &pb.Parameter{
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
				rParameters.Parameters = append(rParameters.Parameters, &pb.Parameter{
					Id:    dpID,
					Error: pb.ParameterError_UnknownError,
					Value: nil,
				})
			} else {
				rParameters.Parameters = append(rParameters.Parameters, &pb.Parameter{
					Id:    dpID,
					Error: pb.ParameterError_NoError,
					Value: iv,
				})
			}
		}
	}
	return
}

// GetParameterDetails returns the Details for given ModelParameterIDs.
// If no IDs are given, the Details from all Parameters will be returned.
func (s *IbeamServer) GetParameterDetails(c context.Context, mpIDs *pb.ModelParameterIDs) (*pb.ParameterDetails, error) {
	log.Debugf("Got a GetParameterDetails from ") //TODO:lookup how to get IP
	rParameterDetails := &pb.ParameterDetails{}
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

func (s *IbeamServer) getParameterDetail(mpID *pb.ModelParameterID) (*pb.ParameterDetail, error) {
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
func (s *IbeamServer) Set(_ context.Context, ps *pb.Parameters) (*pb.Empty, error) {
	for _, parameter := range ps.Parameters {
		s.clientsSetterStream <- *parameter
	}
	return &pb.Empty{}, nil
}

// Subscribe starts a ServerStream and send updates if a Parameter changes.
// On subscribe all current values should be sent back.
// If no IDs are given, subscribe to every Parameter.
func (s *IbeamServer) Subscribe(dpIDs *pb.DeviceParameterIDs, stream pb.IbeamCore_SubscribeServer) error {
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

	distributor := make(chan pb.Parameter, 100)
	s.serverClientsDistributor[distributor] = true

	log.Debugf("Added distributor number %v", len(s.serverClientsDistributor))

	ping := time.NewTicker(time.Second)
	for {
		select {
		// TODO:check if Stream is closed
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
			if len(dpIDs.Ids) == 1 && dpIDs.Ids[0].Parameter == 0 && dpIDs.Ids[0].Device != parameter.Id.Device {
				continue
			}

			// Check for parameter filtering
			if len(dpIDs.Ids) >= 1 && !containsDeviceParameter(parameter.Id, dpIDs) {
				continue
			}
			log.Debugf("Send Parameter with ID '%v' to client from ServerClientsStream", parameter.Id)
			stream.Send(&parameter)
		}
	}
}

func containsDeviceParameter(dpID *pb.DeviceParameterID, dpIDs *pb.DeviceParameterIDs) bool {
	for _, ids := range dpIDs.Ids {
		if ids.Device == dpID.Device && ids.Parameter == dpID.Parameter {
			return true
		}
	}
	return false
}

// CreateServer sets up the ibeam server, parameter manager and parameter registry
func CreateServer(coreInfo pb.CoreInfo, defaultModel pb.ModelInfo) (manager *IbeamParameterManager, registry *IbeamParameterRegistry, settoManager chan pb.Parameter, getfromManager chan pb.Parameter) {
	clientsSetter := make(chan pb.Parameter, 100)
	getfromManager = make(chan pb.Parameter, 100)
	settoManager = make(chan pb.Parameter, 100)

	fistParameter := pb.Parameter{}
	fistParameter.Id = &pb.DeviceParameterID{
		Device:    0,
		Parameter: 0,
	}

	watcher := make(chan pb.Parameter)

	coreInfo.IbeamVersion = pb.File_ibeam_core_proto.Options().ProtoReflect().Get(pb.E_IbeamVersion.TypeDescriptor()).String()

	registry = &IbeamParameterRegistry{
		coreInfo:        coreInfo,
		DeviceInfos:     []*pb.DeviceInfo{},
		ModelInfos:      []*pb.ModelInfo{},
		ParameterDetail: []map[int]*pb.ParameterDetail{},
		parameterValue:  []map[int][]*IbeamParameterDimension{},
	}

	server := IbeamServer{
		parameterRegistry:        registry,
		clientsSetterStream:      clientsSetter,
		serverClientsStream:      watcher,
		serverClientsDistributor: make(map[chan pb.Parameter]bool),
	}

	manager = &IbeamParameterManager{
		parameterRegistry:   registry,
		out:                 getfromManager,
		in:                  settoManager,
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
func (r *IbeamParameterRegistry) RegisterParameter(detail *pb.ParameterDetail) (parameterIndex uint32) {
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
		detail.Id = &pb.ModelParameterID{
			Parameter: parameterIndex,
			Model:     mid,
		}
		r.muDetail.Lock()
		for _, modelconfig := range r.ParameterDetail {
			modelconfig[int(parameterIndex)] = detail
		}
		r.muDetail.Unlock()

	} else {
		modelconfig := &r.ParameterDetail[mid-1]
		if parameterIndex == 0 {
			parameterIndex = uint32(len(*modelconfig) + 1)
		}
		r.muDetail.RUnlock()
		detail.Id = &pb.ModelParameterID{
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
func (r *IbeamParameterRegistry) RegisterParameters(details *pb.ParameterDetails) (ids []uint32) {
	for _, detail := range details.Details {
		ids = append(ids, r.RegisterParameter(detail))
	}
	return
}

// RegisterModel registers a new Model in the Registry with given ModelInfo
func (r *IbeamParameterRegistry) RegisterModel(model *pb.ModelInfo) uint32 {
	r.muDetail.RLock()
	model.Id = uint32(len(r.ParameterDetail) + 1)
	r.muDetail.RUnlock()

	r.muInfo.Lock()
	r.ModelInfos = append(r.ModelInfos, model)
	r.muInfo.Unlock()

	r.muDetail.Lock()
	r.ParameterDetail = append(r.ParameterDetail, map[int]*pb.ParameterDetail{})
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

	parameterDimensions := map[int][]*IbeamParameterDimension{}
	for _, parameterDetail := range modelConfig {
		parameterID := parameterDetail.Id.Parameter
		valueDimensions := []*IbeamParameterDimension{}

		// Integer is default
		initialValue := pb.ParameterValue{Value: &pb.ParameterValue_Integer{Integer: 0}}

		switch parameterDetail.ValueType {
		case pb.ValueType_NoValue:
			initialValue.Value = &pb.ParameterValue_Cmd{Cmd: pb.Command_Trigger}
		case pb.ValueType_Floating:
			initialValue.Value = &pb.ParameterValue_Floating{Floating: 0.0}
		case pb.ValueType_Opt:
			initialValue.Value = &pb.ParameterValue_CurrentOption{CurrentOption: 0}
		case pb.ValueType_String:
			initialValue.Value = &pb.ParameterValue_Str{Str: ""}
		case pb.ValueType_Binary:
			initialValue.Value = &pb.ParameterValue_Binary{Binary: false}
		}

		if len(parameterDetail.Dimensions) > 3 {
			log.Println("It is not recommended to use more than 3 dimensions, if needed please contact the maintainer")
		}

		var generateDimensions func([]uint32, IbeamParameterDimension) *IbeamParameterDimension

		generateDimensions = func(dimensionConfig []uint32, initialValueDimension IbeamParameterDimension) *IbeamParameterDimension {
			if len(dimensionConfig) == 0 {
				initialValueDimension.value.dimensionID = []uint32{1}
				return &initialValueDimension
			}

			dimensions := make(map[int]*IbeamParameterDimension, 0)

			for count := 0; count < int(dimensionConfig[0]); count++ {
				valueWithID := IbeamParameterDimension{}
				valueWithID.value = &IBeamParameterValueBuffer{}
				dimValue, err := initialValueDimension.Value()
				if err != nil {
					log.Fatalf("Initial Value is not set %v", err)
				}
				*valueWithID.value = *dimValue

				valueWithID.value.dimensionID = make([]uint32, len(dimValue.dimensionID))
				copy(valueWithID.value.dimensionID, dimValue.dimensionID)
				valueWithID.value.dimensionID = append(valueWithID.value.dimensionID, uint32(count+1))

				if len(dimensionConfig) == 1 {
					dimensions[count] = &valueWithID
				} else {
					subDim := generateDimensions(dimensionConfig[1:], valueWithID)
					dimensions[count] = subDim
				}
			}
			//fmt.Println(dimensions[1].value.dimensionID)
			return &IbeamParameterDimension{
				subDimensions: dimensions,
			}
		}

		dimensionConfig := []uint32{}
		initialValueDimension := IbeamParameterDimension{
			value: &IBeamParameterValueBuffer{
				dimensionID:    make([]uint32, 0),
				available:      true,
				isAssumedState: true,
				lastUpdate:     time.Now(),
				currentValue:   initialValue,
				targetValue:    initialValue,
			},
		}

		for _, dimension := range parameterDetail.Dimensions {
			dimensionConfig = append(dimensionConfig, dimension.Count)
		}

		valueDimensions = append(valueDimensions, generateDimensions(dimensionConfig, initialValueDimension))

		parameterDimensions[int(parameterID)] = valueDimensions
	}

	r.muInfo.RLock()
	deviceIndex = uint32(len(r.DeviceInfos) + 1)
	r.muInfo.RUnlock()
	r.muInfo.Lock()
	r.DeviceInfos = append(r.DeviceInfos, &pb.DeviceInfo{
		DeviceID: deviceIndex,
		ModelID:  modelID,
	})
	r.muInfo.Unlock()
	r.muValue.Lock()
	r.parameterValue = append(r.parameterValue, parameterDimensions)
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
