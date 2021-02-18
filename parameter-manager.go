package ibeamcorelib

import (
	"net"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
	"google.golang.org/grpc"
)

// IBeamParameterManager manages parameter changes.
type IBeamParameterManager struct {
	parameterRegistry   *IBeamParameterRegistry
	out                 chan *pb.Parameter
	in                  chan *pb.Parameter
	clientsSetterStream chan *pb.Parameter
	serverClientsStream chan *pb.Parameter
	parameterEvent      chan *pb.Parameter
	server              *IBeamServer
}

// StartWithServer Starts the ibeam parameter routine and the GRPC server in one call. This is blocking and should be called at the end of main.
// The network must be "tcp", "tcp4", "tcp6", "unix" or "unixpacket".
func (m *IBeamParameterManager) StartWithServer(network, address string) {
	// Start parameter management routine
	m.Start()

	lis, err := net.Listen(network, address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterIbeamCoreServer(grpcServer, m.server)
	err = grpcServer.Serve(lis)
	if err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func (m *IBeamParameterManager) checkValidParameter(parameter *pb.Parameter) *pb.Parameter {
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
	parameterIndex := parameterID
	deviceID := parameter.Id.Device
	modelIndex := m.parameterRegistry.getModelID(deviceID)

	// Get State and the Configuration (Details) of the Parameter, assume mutex is locked in outer layers of parameterLoop
	state := m.parameterRegistry.ParameterValue
	parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]
	// Check if device and param id are valid and in the State
	if _, exists := state[deviceID][parameterIndex]; !exists {
		log.Errorf("Invalid ID for: DeviceID %d, ParameterID %d", deviceID, parameterID)

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
func (m *IBeamParameterManager) Start() {
	go func() {
		var parameter *pb.Parameter
		for {
			select {
			case parameter = <-m.clientsSetterStream:
				//				log.Info("Got set from client")
				m.ingestTargetParameter(parameter)
			case parameter = <-m.in:
				//				log.Info("Got result from device")
				m.ingestCurrentParameter(parameter)
			case parameter = <-m.parameterEvent:
				//				log.Info("Gonna proccess param")
				m.processParameter(parameter)
			}
		}
	}()
}
