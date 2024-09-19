package ibeamcorelib

import (
	"bytes"
	"context"
	"crypto/md5"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/s00500/env_logger"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	"github.com/SKAARHOJ/ibeam-corelib-go/syncmap"
	skconfig "github.com/SKAARHOJ/ibeam-lib-config"
	elog "github.com/s00500/env_logger"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"
)

// Global definition as we need to be able to reache it outside of main from the generated scripts
var imageFS *embed.FS
var statusOverride string

// IBeamServer implements the IbeamCoreServer interface of the generated protofile library.
type IBeamServer struct {
	*pb.UnimplementedIbeamCoreServer
	parameterRegistry        *IBeamParameterRegistry
	clientsSetterStream      chan *pb.Parameter
	serverClientsStream      chan *pb.Parameter
	serverClientsDistributor map[chan *pb.Parameter]*SubscribeData
	muDistributor            sync.RWMutex
	configPtr                interface{} // TODO: can I not make this better ? as a fix ptr ?
	schemaBytes              []byte
	imageFS                  *embed.FS
	log                      *elog.Entry
}

type SubscribeData struct {
	ChannelClosed atomic.Bool
	IDs           *pb.DeviceParameterIDs
	Identifier    string
}

// GetCoreInfo returns the CoreInfo of the IBeamCore
func (s *IBeamServer) GetCoreInfo(_ context.Context, _ *pb.Empty) (*pb.CoreInfo, error) {
	coreInfo := proto.Clone(s.parameterRegistry.coreInfo).(*pb.CoreInfo)
	s.muDistributor.RLock()
	coreInfo.ConnectedClients = uint32(len(s.serverClientsDistributor))
	s.muDistributor.RUnlock()
	return coreInfo, nil
}

// GetCoreInfo returns the CoreInfo of the IBeamCore
func (s *IBeamServer) RestartCore(_ context.Context, _ *pb.RestartInfo) (*pb.Empty, error) {
	go func() {
		s.log.Warn("Restart requested via core protocol, executing...")
		time.Sleep(time.Millisecond * 300)
		err := execReload()
		s.log.Should(err)
	}()
	return &pb.Empty{}, nil
}

// RestartCore via core implementation... only works on linux and macOS
func RestartCore() {
	go func() {
		log.Warn("Restart requested, executing...")
		time.Sleep(time.Millisecond * 300)
		err := execReload()
		log.Should(err)
	}()
}

// GetCoreInfo returns the configuration schema of the core
func (s *IBeamServer) GetCoreConfigSchema(_ context.Context, _ *pb.Empty) (*pb.ByteData, error) {
	return &pb.ByteData{Data: s.schemaBytes}, nil
}

// GetCoreInfo returns the current active configuration of the core
func (s *IBeamServer) GetCoreConfig(_ context.Context, _ *pb.Empty) (*pb.ByteData, error) {
	jsonBytes, err := json.Marshal(s.configPtr)
	s.log.Should(err)

	// Special construction to allow multicore runner to be used
	var didfilter []int
	if os.Getenv("DID") != "" {
		didfilter = make([]int, 0)
		stringValues := strings.Split(os.Getenv("DID"), ",")
		for _, sV := range stringValues {
			v, err := strconv.Atoi(sV)
			if log.ShouldWrap(err, "on parsing didfilter: ") {
				continue
			}
			didfilter = append(didfilter, v)
		}
	}

	return &pb.ByteData{Data: jsonBytes}, nil
}

// SetCoreConfig validates and saves the new config to the config file, it does not load it into the core without a restart
func (s *IBeamServer) SetCoreConfig(_ context.Context, input *pb.ByteData) (*pb.Empty, error) {
	// first we need to validate config, if valid we can store it into the config file

	var configMap interface{}
	s.log.Trace("Incoming Config ", string(input.GetData()))
	err := json.Unmarshal(input.Data, &configMap)
	if s.log.Should(err) {
		return nil, fmt.Errorf("could not parse config: %w", err)
	}

	cleaned, err := skconfig.ValidateConfig(skconfig.GetSchema(s.configPtr), configMap, false, "devicecore")
	if err != nil {
		return nil, fmt.Errorf("could not validate config: %w", err)
	}

	// save json to files
	err = skconfig.Save(&cleaned)
	if s.log.Should(err) {
		return nil, fmt.Errorf("could not save config")
	}
	return &pb.Empty{}, nil
}

// GetDeviceInfo returns the DeviceInfos for given DeviceIDs.
// If no IDs are given, all DeviceInfos will be returned.
func (s *IBeamServer) GetDeviceInfo(_ context.Context, deviceIDs *pb.DeviceIDs) (*pb.DeviceInfos, error) {
	s.log.Debugf("Client asks for DeviceInfo with ids %v", deviceIDs.Ids)
	s.parameterRegistry.muInfo.RLock()
	defer s.parameterRegistry.muInfo.RUnlock()

	if len(deviceIDs.Ids) == 0 {
		infos := make([]*pb.DeviceInfo, len(s.parameterRegistry.deviceInfos))
		index := 0
		for _, info := range s.parameterRegistry.deviceInfos {
			infos[index] = info
			index++
		}
		return &pb.DeviceInfos{DeviceInfos: infos}, nil
	}

	var rDeviceInfos pb.DeviceInfos
	for _, deviceID := range deviceIDs.Ids {
		_, dExists := s.parameterRegistry.deviceInfos[deviceID]
		if dExists && deviceID != 0 {
			rDeviceInfos.DeviceInfos = append(rDeviceInfos.DeviceInfos, s.parameterRegistry.deviceInfos[deviceID])
		}
		// If we have no Device with such a ID, skip
	}
	return &rDeviceInfos, nil

}

// GetModelInfo returns the ModelInfos for given ModelIDs.
// If no IDs are given, all ModelInfos will be returned.
func (s *IBeamServer) GetModelInfo(_ context.Context, mIDs *pb.ModelIDs) (*pb.ModelInfos, error) {
	s.parameterRegistry.muInfo.RLock()
	defer s.parameterRegistry.muInfo.RUnlock()

	if len(mIDs.Ids) == 0 {
		infos := make([]*pb.ModelInfo, len(s.parameterRegistry.modelInfos))
		index := 0
		for _, info := range s.parameterRegistry.modelInfos {
			infos[index] = info
			index++
		}
		return &pb.ModelInfos{ModelInfos: infos}, nil
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

func getModelWithID(s *IBeamServer, mID uint32) *pb.ModelInfo {
	for _, model := range s.parameterRegistry.modelInfos {
		if model.Id == mID {
			return model
		}
	}
	return nil
}

// Get returns the Parameters with their current state for given DeviceParameterIDs.
// If no IDs are given, all Parameters will be returned.
func (s *IBeamServer) Get(_ context.Context, dpIDs *pb.DeviceParameterIDs) (rParameters *pb.Parameters, err error) {
	rParameters = &pb.Parameters{}
	s.parameterRegistry.muValue.RLock()
	defer s.parameterRegistry.muValue.RUnlock()

	// Unfiltered response
	if len(dpIDs.Ids) == 0 {
		for did, dState := range s.parameterRegistry.parameterValue {
			for pid := range dState {
				dpID := pb.DeviceParameterID{
					Parameter: pid,
					Device:    did,
				}

				iv := s.parameterRegistry.getInstanceValues(&dpID, true)
				if iv == nil {
					continue
				}
				rParameters.Parameters = append(rParameters.Parameters, &pb.Parameter{
					Id:    &dpID,
					Error: 0,
					Value: iv,
				})
			}
		}
		return rParameters, err
	}

	// All Parameters for filtered device
	if len(dpIDs.Ids) == 1 && dpIDs.Ids[0].Parameter == 0 && dpIDs.Ids[0].Device != 0 {
		did := dpIDs.Ids[0].Device
		if _, exists := s.parameterRegistry.parameterValue[did]; !exists {
			rParameters.Parameters = append(rParameters.Parameters, &pb.Parameter{
				Id:    dpIDs.Ids[0],
				Error: pb.ParameterError_UnknownID,
				Value: nil,
			})
			return
		}
		for pid := range s.parameterRegistry.parameterValue[did] {
			dpID := pb.DeviceParameterID{
				Parameter: pid,
				Device:    did,
			}
			iv := s.parameterRegistry.getInstanceValues(&dpID, true)
			if iv != nil {
				rParameters.Parameters = append(rParameters.Parameters, &pb.Parameter{
					Id:    &dpID,
					Error: 0,
					Value: iv,
				})
			}

		}
		return rParameters, err
	}

	// Filter specific parameters
	for _, dpID := range dpIDs.Ids {
		if dpID.Device == 0 || dpID.Parameter == 0 {
			err = errors.New("Failed to get instance values " + dpID.String())
			return
		}
		iv := s.parameterRegistry.getInstanceValues(dpID, true)
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
	return rParameters, err
}

// GetParameterDetails returns the Details for given ModelParameterIDs.
// If no IDs are given, the Details from all Parameters will be returned.
func (s *IBeamServer) GetParameterDetails(c context.Context, mpIDs *pb.ModelParameterIDs) (*pb.ParameterDetails, error) {
	p, _ := peer.FromContext(c)
	clientIP := p.Addr.String()

	s.log.Debugln("Got a GetParameterDetails from", clientIP)
	rParameterDetails := &pb.ParameterDetails{}
	s.parameterRegistry.muInfo.RLock()
	defer s.parameterRegistry.muInfo.RUnlock()

	if len(mpIDs.Ids) == 0 {
		for _, modelDetails := range s.parameterRegistry.parameterDetail {
			for _, modelDetail := range modelDetails {
				rParameterDetails.Details = append(rParameterDetails.Details, modelDetail)
			}
		}
	} else if len(mpIDs.Ids) == 1 && int(mpIDs.Ids[0].Parameter) == 0 {
		// Return all parameters for model
		if _, exists := s.parameterRegistry.parameterDetail[mpIDs.Ids[0].Model]; exists {
			for _, modelDetail := range s.parameterRegistry.parameterDetail[mpIDs.Ids[0].Model] {
				rParameterDetails.Details = append(rParameterDetails.Details, modelDetail)
			}
		} else {
			s.log.Error("Invalid model ID specified")
		}
	} else {
		for _, mpID := range mpIDs.Ids {
			d, err := s.getParameterDetail(mpID)
			if err != nil {
				s.log.Errorf(err.Error())
				return nil, err
			}
			rParameterDetails.Details = append(rParameterDetails.Details, d)

		}
	}
	s.log.Debugf("Send ParameterDetails for %v parameters", len(rParameterDetails.Details))
	return rParameterDetails, nil
}

func (s *IBeamServer) getParameterDetail(mpID *pb.ModelParameterID) (*pb.ParameterDetail, error) {
	return s.parameterRegistry.getParameterDetail(mpID.Parameter, mpID.Model)
}

// Set will change the Value for the given Parameter.
func (s *IBeamServer) Set(_ context.Context, ps *pb.Parameters) (*pb.Empty, error) {
	for _, parameter := range ps.Parameters {
		s.clientsSetterStream <- parameter
	}
	return &pb.Empty{}, nil
}

// Subscribe starts a ServerStream and send updates if a Parameter changes.
// On subscribe all current values should be sent back.
// If no IDs are given, subscribe to every Parameter.
func (s *IBeamServer) Subscribe(dpIDs *pb.DeviceParameterIDs, stream pb.IbeamCore_SubscribeServer) error {
	/*	var sendCounter atomic.Int32
		var getCounter atomic.Int32

		go func() {
			t := time.NewTicker(time.Second)
			for {
				select {
				case <-t.C:
					s.log.Infof("Server: sent %d (get: %d)", sendCounter.Load(), getCounter.Load())
				case <-stream.Context().Done():
					return

				}
			}
		}()
	*/

	p, _ := peer.FromContext(stream.Context())
	clientIP := p.Addr.String()
	subscribeId := ""
	md, ok := metadata.FromIncomingContext(stream.Context())
	if ok {
		if val, exists := md["x-ibeam-subscribeid"]; exists {
			if len(val) < 1 {
				return fmt.Errorf("Metadata not holding any data")
			}

			subscribeId = val[0]
			// got subscribe ID
			s.muDistributor.RLock()
			for dChan, distData := range s.serverClientsDistributor {
				if subscribeId != distData.Identifier {
					continue
				}
				if distData.ChannelClosed.Load() {
					continue
				}

				// redefine filter! but only do a get on new params!
				s.muDistributor.RUnlock()

				s.muDistributor.Lock()
				sendParams := make([]*pb.DeviceParameterID, 0)
				s.log.Debugf("Resubscribe %s %v", subscribeId, distData.IDs)
				for _, id := range dpIDs.Ids {
					if !containsDeviceParameter(id, distData.IDs) {
						sendParams = append(sendParams, &pb.DeviceParameterID{Parameter: id.Parameter, Device: id.Device})
					}
				}
				s.serverClientsDistributor[dChan] = &SubscribeData{Identifier: subscribeId, IDs: dpIDs}
				s.muDistributor.Unlock()

				//only get new dpids
				if len(sendParams) > 0 {
					parameters, err := s.Get(stream.Context(), &pb.DeviceParameterIDs{Ids: sendParams})
					if err != nil {
						return err
					}

					for _, param := range parameters.Parameters {
						dChan <- param
					}
				}

				return nil
			}
			s.muDistributor.RUnlock()
		}
	}

	// Get the current subscription ID, check if it has been used before, if so just perform a get on the new parameters and update the filtering

	s.log.Debug("New Client subscribed from ", clientIP)
	//s.log.Timer("subtimer")

	// Fist send all parameters
	parameters, err := s.Get(stream.Context(), dpIDs)
	if err != nil {
		return err
	}

	// Create dist first to catch incoming params
	distributor := make(chan *pb.Parameter, 200)
	s.muDistributor.Lock()
	subData := SubscribeData{Identifier: subscribeId, IDs: dpIDs}
	s.serverClientsDistributor[distributor] = &subData

	s.log.Debugf("Added distributor number %v", len(s.serverClientsDistributor))
	s.muDistributor.Unlock()

	defer func() {
		subData.ChannelClosed.Store(true)
		close(distributor)
	}()

	for _, parameter := range parameters.Parameters {
		s.log.Debugf("Send Parameter with ID '%v' to client", parameter.Id)
		s.log.Tracef("Param: %+v", parameter)
		err := stream.Send(parameter)
		if err != nil {
			if !strings.Contains(err.Error(), "Canceled desc = context canceled") && !strings.Contains(err.Error(), "Unavailable desc = transport is closing") {
				log.ShouldWrap(err, "on sending param")
			}
		}
		//getCounter.Add(1)
	}
	//log.Info("Send of existing took ", s.log.TimerEnd("subtimer")) // On kairos this took 1.3 seconds... keep that in mind

	// Send out all errors
	errorParams := getActiveStatuses()
	for _, param := range errorParams {
		err := stream.Send(param)
		log.ShouldWrap(err, "on sending initial error parameters")
	}

	ping := time.NewTicker(time.Millisecond * 200)
	for {
		select {
		case <-ping.C:
			err := stream.Context().Err()
			if err != nil {
				s.muDistributor.Lock()
				delete(s.serverClientsDistributor, distributor)
				s.muDistributor.Unlock()
				s.log.Debug("Connection to client for subscription lost")
				return nil
			}
		case parameter := <-distributor:
			// Filtering already happened in the distributor
			s.log.Debugf("Send Parameter with ID '%v' to client from ServerClientsStream", parameter.Id)
			err := stream.Send(parameter)
			if err != nil {
				if !strings.Contains(err.Error(), "Canceled desc = context canceled") && !strings.Contains(err.Error(), "Unavailable desc = transport is closing") {
					log.ShouldWrap(err, "on sending param")
				}
				return nil // get out of here...
			}
		}
	}
}

// GetModelImages allows the client to request core images
func (s *IBeamServer) GetModelImages(_ context.Context, req *pb.ModelImageRequest) (*pb.ModelImages, error) {
	if !s.parameterRegistry.coreInfo.HasModelImages {
		return nil, fmt.Errorf("This core does not provide model images yet")
	}

	if req.Models == nil || len(req.Models.Ids) == 0 { // Fetch all available
		dirEntries, err := imageFS.ReadDir("model_images")
		if err != nil {
			return nil, fmt.Errorf("Could not load image directory")
		}
		modelPattern := regexp.MustCompile(`model\-[0-9]+\.png`)
		images := make([]*pb.ModelImage, 0)

		for _, entry := range dirEntries {
			if entry.IsDir() {
				continue
			}
			if !modelPattern.Match([]byte(entry.Name())) {
				continue
			}
			data, err := imageFS.ReadFile(fmt.Sprintf("model_images/%s", entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("Could not load image for file %s", entry.Name())
			}
			mIDstring := strings.TrimPrefix(entry.Name(), "model-")
			mIDstring = strings.TrimSuffix(mIDstring, ".png")
			mid, _ := strconv.Atoi(mIDstring)
			hash, err := md5sum(data)
			log.Should(log.Wrap(err, "on hashing md5 for GetModelImages"))
			imageStruct := &pb.ModelImage{ModelID: uint32(mid), Hash: hash}
			if !req.HashOnly {
				imageStruct.ImageData = data
			}
			images = append(images, imageStruct)
		}
		return &pb.ModelImages{Imgs: images}, nil
	}

	images := make([]*pb.ModelImage, len(req.Models.Ids))
	for i, mid := range req.Models.Ids {
		data, err := imageFS.ReadFile(fmt.Sprintf("model_images/model-%d.png", mid))
		if err != nil {
			return nil, fmt.Errorf("Could not load image for modelID %d", mid)
		}
		hash, err := md5sum(data)
		log.Should(log.Wrap(err, "on hashing md5 for GetModelImages"))
		imageStruct := &pb.ModelImage{ModelID: uint32(mid), Hash: hash}
		if !req.HashOnly {
			imageStruct.ImageData = data
		}
		images[i] = imageStruct
	}

	return &pb.ModelImages{Imgs: images}, nil
}

func md5sum(data []byte) (string, error) {
	var md5string string

	hash := md5.New()
	if _, err := io.Copy(hash, bytes.NewReader(data)); err != nil {
		return md5string, err
	}
	hashInBytes := hash.Sum(nil)[:16]
	md5string = hex.EncodeToString(hashInBytes)
	return md5string, nil
}

func containsDeviceParameter(dpID *pb.DeviceParameterID, dpIDs *pb.DeviceParameterIDs) bool {
	for _, ids := range dpIDs.Ids {
		if ids.Device == dpID.Device && ids.Parameter == dpID.Parameter {
			return true
		}
	}
	return false
}

// SetImageFS sets the image folder embedded fs, this needs to be called before eveything else, usefull in generated code
func SetImageFS(fs *embed.FS) {
	if imageFS != nil {
		log.Fatal("Can not set ImageFS a second time")
	}
	imageFS = fs
}

// SetDevStatusOverride sets a override for the development status in coreinfo, this needs to be called before eveything else, usefull in generated code
func SetDevStatusOverride(status string) {
	statusOverride = status
}

// CreateServer sets up the ibeam server, parameter manager and parameter registry
func CreateServer(coreInfo *pb.CoreInfo) (manager *IBeamParameterManager, registry *IBeamParameterRegistry, settoManager chan<- *pb.Parameter, getfromManager <-chan *pb.Parameter) {
	defaultModelInfo := &pb.ModelInfo{
		Name:        coreInfo.Label + " Generic Model",
		Description: "Default model of the core, inherits all possible parameters from other models",
	}
	return CreateServerWithDefaultModelAndConfig(coreInfo, defaultModelInfo, nil)
}

// CreateServer sets up the ibeam server, parameter manager and parameter registry
func CreateServerWithConfig(coreInfo *pb.CoreInfo, config interface{}) (manager *IBeamParameterManager, registry *IBeamParameterRegistry, settoManager chan<- *pb.Parameter, getfromManager <-chan *pb.Parameter) {
	defaultModelInfo := &pb.ModelInfo{
		Name:        coreInfo.Label + " Generic Model",
		Description: "Default model of the core, inherits all possible parameters from other models",
	}
	return CreateServerWithDefaultModelAndConfig(coreInfo, defaultModelInfo, config)
}

var PreLoadedConfig interface{}

// CreateServerWithDefaultModel sets up the ibeam server, parameter manager and parameter registry and allows to specify a default model
func CreateServerWithDefaultModelAndConfig(coreInfo *pb.CoreInfo, defaultModel *pb.ModelInfo, config interface{}) (manager *IBeamParameterManager, registry *IBeamParameterRegistry, setToManager chan<- *pb.Parameter, getFromManager <-chan *pb.Parameter) {
	// Load configuration
	if config != nil && PreLoadedConfig == nil {
		skconfig.SetCoreName(coreInfo.Name)
		err := skconfig.Load(config)
		elog.MustFatal(err)
	} else if PreLoadedConfig != nil {
		skconfig.SetCoreName(coreInfo.Name)
		config = PreLoadedConfig
	}

	clientsSetter := make(chan *pb.Parameter, 100)
	getfromManagerChannel := make(chan *pb.Parameter, 100)
	settoManagerChannel := make(chan *pb.Parameter, 100)
	outInternal := make(chan *pb.Parameter, 100)
	getFromManager = getfromManagerChannel
	setToManager = settoManagerChannel

	watcher := make(chan *pb.Parameter)

	coreInfo.IbeamVersion = GetProtocolVersion()

	coreInfo.HasModelImages = false // Make sure we do not allow the core to control this field
	if imageFS != nil {
		dirEntries, err := imageFS.ReadDir("model_images")
		if err == nil {

		}
		modelPattern := regexp.MustCompile(`model\-[0-9]+\.png`)

		for _, entry := range dirEntries {
			if entry.IsDir() {
				continue
			}
			if !modelPattern.Match([]byte(entry.Name())) {
				continue
			}
			// We have found at lease 1 image, so we are good to go
			coreInfo.HasModelImages = true
			break
		}
	}

	coreInfo.DevelopmentStatus = "concept"

	if statusOverride != "" {
		coreInfo.DevelopmentStatus = statusOverride
	}

	registry = &IBeamParameterRegistry{
		coreInfo:         proto.Clone(coreInfo).(*pb.CoreInfo),
		deviceInfos:      map[uint32]*pb.DeviceInfo{},
		deviceLastEvent:  syncmap.New[uint32, time.Time](),
		modelRateLimiter: make(map[uint32]uint),
		modelInfos:       map[uint32]*pb.ModelInfo{},
		parameterDetail:  map[uint32]map[uint32]*pb.ParameterDetail{},
		parameterValue:   map[uint32]map[uint32]*iBeamParameterDimension{},
		log:              elog.GetLoggerForPrefix("ib/registry"),
	}

	sLog := elog.GetLoggerForPrefix("ib/server")
	server := IBeamServer{
		parameterRegistry:        registry,
		clientsSetterStream:      clientsSetter,
		serverClientsStream:      watcher,
		serverClientsDistributor: make(map[chan *pb.Parameter]*SubscribeData),
		log:                      sLog,
	}

	if config != nil {
		server.configPtr = config
		schemaValue := skconfig.GetSchema(server.configPtr)
		server.schemaBytes, _ = json.Marshal(schemaValue)
	}

	manager = &IBeamParameterManager{
		parameterRegistry:   registry,
		out:                 outInternal,
		outActual:           getfromManagerChannel,
		in:                  settoManagerChannel,
		clientsSetterStream: clientsSetter,
		serverClientsStream: watcher,
		parameterEvent:      make(chan paramDimensionAddress, 2000), // Has to be big as all the reevals travel through here
		server:              &server,
		log:                 elog.GetLoggerForPrefix("ib/manager"),
	}

	go func() {
		for {
			parameter := <-watcher
			//log.Timer("muLock")
			server.muDistributor.RLock()
			//dur, _ := log.TimerEndValue("muLock")
			//if dur > time.Millisecond*50 {
			//	log.Error("RLocking took more than 50")
			//}

			id := 0
			for channel, subscribeData := range server.serverClientsDistributor {
				id++

				if subscribeData.ChannelClosed.Load() {
					continue
				}
				if subscribeData != nil && subscribeData.IDs != nil {
					paramfilter := subscribeData.IDs

					// Global error
					if parameter.Error == pb.ParameterError_Custom && parameter.Id != nil && parameter.Id.Device == 0 && parameter.Id.Parameter == 0 {
						select {
						case channel <- parameter:
						case <-time.After(1 * time.Second):
							sLog.Errorln("corelib distributor timed out (global error case) Nr: ", id)
						}
						continue
					}

					// Filtering as specified by the original request
					if parameter.Error != pb.ParameterError_Custom && (parameter.Id == nil || parameter.Id.Device == 0 || parameter.Id.Parameter == 0) {
						continue
					}
					// Check if Device is Subscribed
					if len(paramfilter.Ids) == 1 && paramfilter.Ids[0].Parameter == 0 && paramfilter.Ids[0].Device != parameter.Id.Device {
						sLog.Tracef("Blocked sending out of change because of devicefilter Want: %d, Got: %d", paramfilter.Ids[0].Device, parameter.Id.Device)
						continue
					}

					// Check for parameter filtering
					if len(paramfilter.Ids) >= 1 && paramfilter.Ids[0].Parameter != 0 && !containsDeviceParameter(parameter.Id, paramfilter) {
						sLog.Tracef("Blocked sending out of change of parameter %d (D: %d) because of device parameter id filter, %v", parameter.Id.Parameter, parameter.Id.Device, paramfilter)
						continue
					}

					if subscribeData.ChannelClosed.Load() {
						continue
					}
					//log.Timer("secondLock")
					select {
					case channel <- parameter:
					default:
						//case <-time.After(1 * time.Second):
						sLog.Debugln("corelib distributor timed out Nr: ", id)
						subscribeData.ChannelClosed.Store(true) // signal back to recon

						server.muDistributor.RUnlock()
						server.muDistributor.Lock()
						delete(server.serverClientsDistributor, channel)
						server.muDistributor.Unlock()
						server.muDistributor.RLock()

					}
					//dur, _ := log.TimerEndValue("secondLock")
					//if dur > time.Millisecond*50 {
					//	log.Errorf("Sending took more than 50 %s", dur)
					//}
					continue
				}

				server.muDistributor.RUnlock()
				server.muDistributor.Lock()
				delete(server.serverClientsDistributor, channel)
				server.muDistributor.Unlock()
				server.muDistributor.RLock()
			}
			server.muDistributor.RUnlock()
		}
	}()
	registry.RegisterModel(defaultModel)
	return
}

func (m *IBeamParameterManager) pName(id *pb.DeviceParameterID) string {
	return fmt.Sprintf("parameter %s (P:%d, D: %d)", m.parameterRegistry.PName(id.Parameter), id.Parameter, id.Device)
}

func GetProtocolVersion() string {
	return pb.File_ibeam_core_proto.Options().ProtoReflect().Get(pb.E_IbeamVersion.TypeDescriptor()).String()
}
