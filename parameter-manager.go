package ibeamcorelib

import (
	"net"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
	"google.golang.org/grpc"
)

// IbeamParameterManager manages parameter changes.
type IbeamParameterManager struct {
	parameterRegistry   *IbeamParameterRegistry
	out                 chan *pb.Parameter
	in                  chan *pb.Parameter
	clientsSetterStream chan *pb.Parameter
	serverClientsStream chan *pb.Parameter
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
	err = grpcServer.Serve(lis)
	if err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
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
	parameterIndex := parameterID
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
				var parameter *pb.Parameter
				select {
				case parameter = <-m.clientsSetterStream:
					m.ingestTargetParameter(parameter)
				default:
					break clientToManagerLoop
				}
			}

			// ***************
			//    Param Ingest loop, inputs changes from a device (f.e. a camera) to manager
			// ***************

		deviceToManagerLoop:
			for {
				var parameter *pb.Parameter
				select {
				case parameter = <-m.in:
					m.ingestCurrentParameter(parameter)
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
