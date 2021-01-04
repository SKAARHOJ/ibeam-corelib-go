package ibeam_corelib

import (
	"context"
	"errors"
	"fmt"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"
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
	coreInfo := proto.Clone(s.parameterRegistry.coreInfo.ProtoReflect().Interface()).(*pb.CoreInfo)
	coreInfo.ConnectedClients = uint32(len(s.serverClientsDistributor))
	return coreInfo, nil
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
				iv := s.parameterRegistry.getInstanceValues(&dpID)
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
			iv := s.parameterRegistry.getInstanceValues(&dpID)
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
			iv := s.parameterRegistry.getInstanceValues(dpID)
			if len(iv) == 0 {
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
	p, _ := peer.FromContext(c)
	clientIP := p.Addr.String()

	log.Debugf("Got a GetParameterDetails from ", clientIP)
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
	p, _ := peer.FromContext(stream.Context())
	clientIP := p.Addr.String()
	log.Debug("New Client subscribed from ", clientIP)

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

	ping := time.NewTicker(time.Second / 2)
	for {
		select {
		case <-ping.C:
			err := stream.Context().Err()
			if err != nil {
				delete(s.serverClientsDistributor, distributor)
				log.Warn("Connection to client for subscription lost")
				return nil
			}
		case parameter := <-distributor:
			if parameter.Id == nil || parameter.Id.Device == 0 || parameter.Id.Parameter == 0 {
				continue
			}
			// Check if Device is Subscribed
			if len(dpIDs.Ids) == 1 && dpIDs.Ids[0].Parameter == 0 && dpIDs.Ids[0].Device != parameter.Id.Device {
				log.Tracef("Blocked sending out of change because of devicefilter Want: %d, Got: %d", dpIDs.Ids[0].Device, parameter.Id.Device)
				continue
			}

			// Check for parameter filtering
			if len(dpIDs.Ids) >= 1 && dpIDs.Ids[0].Parameter != 0 && !containsDeviceParameter(parameter.Id, dpIDs) {
				log.Tracef("Blocked sending out of change of parameter %d (D: %d) because of device parameter id filter, %v", parameter.Id.Parameter, parameter.Id.Device, dpIDs)
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
func CreateServer(coreInfo *pb.CoreInfo) (manager *IbeamParameterManager, registry *IbeamParameterRegistry, settoManager chan pb.Parameter, getfromManager chan pb.Parameter) {
	defaultModelInfo := &pb.ModelInfo{
		Name:        "Generic Model",
		Description: "default model of the implementation",
	}
	return CreateServerWithDefaultModel(coreInfo, defaultModelInfo)
}

// CreateServerWithDefaultModel sets up the ibeam server, parameter manager and parameter registry and allows to specify a default model
func CreateServerWithDefaultModel(coreInfo *pb.CoreInfo, defaultModel *pb.ModelInfo) (manager *IbeamParameterManager, registry *IbeamParameterRegistry, settoManager chan pb.Parameter, getfromManager chan pb.Parameter) {
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
		coreInfo:        *coreInfo,
		DeviceInfos:     []*pb.DeviceInfo{},
		ModelInfos:      []*pb.ModelInfo{},
		ParameterDetail: []map[int]*pb.ParameterDetail{},
		parameterValue:  []map[int]*IbeamParameterDimension{},
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
	registry.RegisterModel(defaultModel)
	return
}
