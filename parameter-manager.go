package ibeamcorelib

import (
	"net"
	"os"
	"strings"
	"time"

	"sync"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	env "github.com/SKAARHOJ/ibeam-lib-env"
	log "github.com/s00500/env_logger"
	"google.golang.org/grpc"
)

// Internal version of parameterID, to not mess with protobuff mechanisma
type paramDimensionAddress struct {
	parameter   uint32
	device      uint32
	dimensionID []uint32
}

// IBeamParameterManager manages parameter changes.
type IBeamParameterManager struct {
	parameterRegistry   *IBeamParameterRegistry
	out                 chan *pb.Parameter
	outActual           chan<- *pb.Parameter
	in                  <-chan *pb.Parameter
	clientsSetterStream chan *pb.Parameter
	serverClientsStream chan *pb.Parameter
	parameterEvent      chan paramDimensionAddress
	server              *IBeamServer
	log                 *log.Entry

	// Debugging
	//processCounter       atomic.Int32
	//ingestCurrentCounter atomic.Int32
	//ingestTargetCounter  atomic.Int32
}

func (m *IBeamParameterManager) SetServerConfigPtr(configptr interface{}) {
	m.server.configPtr = configptr
}

// StartWithServer Starts the ibeam parameter routine and the GRPC server in one call. This is blocking and should be called at the end of main.
func (m *IBeamParameterManager) StartWithServer(address string) {
	ReloadHook() // just to be sure, this can later be called in the top of the main function to avoid duplicate logs

	// Start parameter management routine
	m.Start()
	/*
		go func() {
			t := time.NewTicker(time.Second)
			for range t.C {
				m.log.Infof("ManagerStatus: Processed %d, (Target: %d Current %d)", m.processCounter.Load(), m.ingestTargetCounter.Load(), m.ingestCurrentCounter.Load())
			}
		}()
	*/

	addressOverride := os.Getenv("IBEAM_CORE_ADDRESS")
	if addressOverride != "" {
		address = addressOverride
	} else if env.IsSkaarOSProd() {
		m.server.log.Trace("overriding listeningport with socket")
		address = "/var/ibeam/sockets/" + m.server.parameterRegistry.coreInfo.Name + ".socket"
	}

	if strings.HasPrefix(address, "/var/ibeam/sockets") {
		err := os.Remove(address)
		if err != nil {
			m.log.Trace(log.Wrap(err, "on removing old socket file"))
		}
	}

	network := "tcp"
	if strings.HasPrefix(address, "/") {
		network = "unix"
	}

	lis, err := net.Listen(network, address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	size := 1024 * 1024 * 20
	grpcServer := grpc.NewServer(
		grpc.MaxSendMsgSize(size),
		grpc.MaxRecvMsgSize(size),
	)
	pb.RegisterIbeamCoreServer(grpcServer, m.server)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = grpcServer.Serve(lis)
		if err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
		wg.Done()
	}()

	sig, err := wait() // Will this term on all sigs ?
	log.Should(err)

	grpcServer.Stop()
	wg.Wait()

	if sig == SIGUSR2 {
		log.Info("Restart requested via Signal")
		err := execReload()
		log.Should(err)
	}
}

func (m *IBeamParameterManager) checkValidParameter(parameter *pb.Parameter) *pb.Parameter {
	// Check if given Parameter has an DeviceParameterID
	if parameter.Id == nil {
		// Client sees what he has send
		parameter.Error = pb.ParameterError_UnknownID
		m.log.Errorf("Given Parameter %v has no ID", parameter)

		return &pb.Parameter{
			Id:    parameter.Id,
			Error: pb.ParameterError_UnknownID,
			Value: []*pb.ParameterValue{},
		}
	}

	// Get Index and ID for Device and Parameter and the actual state of all parameters
	parameterID := parameter.Id.Parameter
	parameterIndex := parameterID
	deviceID := parameter.Id.Device
	modelIndex := m.parameterRegistry.getModelID(deviceID)

	// Get State and the Configuration (Details) of the Parameter, assume mutex is locked in outer layers of parameterLoop
	state := m.parameterRegistry.parameterValue
	parameterConfig := m.parameterRegistry.parameterDetail[modelIndex][parameterIndex]
	// Check if device and param id are valid and in the State
	if _, exists := state[deviceID][parameterIndex]; !exists {
		m.log.Errorf("Invalid ID for: DeviceID %d, ParameterID %d", deviceID, parameterID)

		return &pb.Parameter{
			Id:    parameter.Id,
			Error: pb.ParameterError_UnknownID,
			Value: []*pb.ParameterValue{},
		}
	}

	foundNonNil := false
	for _, v := range parameter.Value {
		if v == nil {
			continue
		}
		if v.Value != nil {
			foundNonNil = true
			break
		}
	}

	if !foundNonNil {
		// a request to set available or invalid from ingest current
		return nil
	}

	// Check if the configured type of the Parameter has a value
	if parameterConfig.ValueType == pb.ValueType_NoValue && parameterConfig.ControlStyle == pb.ControlStyle_NoControl {
		m.log.Errorf("Want to set Parameter with ID %v (%v), but it is configured as Type NoValue with no Control", parameterID, parameterConfig.Name)
		return &pb.Parameter{
			Id:    parameter.Id,
			Error: pb.ParameterError_HasNoValue,
			Value: []*pb.ParameterValue{},
		}
	}

	if parameterConfig.ValueType == pb.ValueType_NoValue && parameterConfig.ControlStyle == pb.ControlStyle_Oneshot {
		if cmd, ok := parameter.Value[0].Value.(*pb.ParameterValue_Cmd); ok {
			if cmd.Cmd != pb.Command_Trigger {
				m.log.Errorf("Want to set Parameter with ID %v (%v), but it is configured as Type NoValue with ControlStyle OneShot. Accept only Command:Trigger", parameterID, parameterConfig.Name)
				return &pb.Parameter{
					Id:    parameter.Id,
					Error: pb.ParameterError_InvalidType,
					Value: []*pb.ParameterValue{},
				}
			}
		} else {
			m.log.Errorf("Want to set Parameter with ID %v (%v), but it is configured as Type NoValue with ControlStyle OneShot. Accept only Command:Trigger", parameterID, parameterConfig.Name)
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
func (m *IBeamParameterManager) Start() {
	// Helper Routine to ensure we do not block the manager on core blocking and can output proper errors
	go func() {
		for p := range m.out {
			select {
			case m.outActual <- p:
			case <-time.After(1 * time.Second):
				m.log.Error("Core implementation was not able to handle additional messages from parameter manager")
			}
		}
	}()

	// Main Manager Routines
	go func() {
		for parameter := range m.clientsSetterStream {
			// 				m.log.Infof("Got set from client %v", parameter)
			m.ingestTargetParameter(parameter)
		}
	}()
	go func() {
		for parameter := range m.in {
			//				m.log.Infof("Got result from device %v", parameter)
			m.ingestCurrentParameter(parameter)
		}
	}()
	go func() {
		for address := range m.parameterEvent {
			//				m.log.Infof("Gonna proccess param %v", parameter)
			m.processParameter(address)
		}
	}()
}

func isDescreteValue(parameterConfig *pb.ParameterDetail, value float64) bool {
	found := false
	if len(parameterConfig.DescreteValueDetails) > 0 {
		for _, dv := range parameterConfig.DescreteValueDetails {
			if dv.GetValue() == value {
				found = true
				break
			}
		}
	}
	return found
}
