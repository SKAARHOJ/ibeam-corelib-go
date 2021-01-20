package ibeamcorelib

import (
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
	"google.golang.org/protobuf/proto"
)

func (m *IbeamParameterManager) ingestCurrentParameter(parameter *pb.Parameter) {
	if err := m.checkValidParameter(parameter); err != nil {
		log.Error(err)
		return
	}

	// Get Index and ID for Device and Parameter and the actual state of all parameters
	parameterID := parameter.Id.Parameter
	parameterIndex := parameterID
	deviceID := parameter.Id.Device
	deviceIndex := int(deviceID - 1)
	modelIndex := m.parameterRegistry.getModelIndex(deviceID)

	// Get State and the Configuration (Details) of the Parameter
	m.parameterRegistry.muValue.Lock()
	defer m.parameterRegistry.muValue.Unlock()
	state := m.parameterRegistry.parameterValue
	m.parameterRegistry.muDetail.RLock()
	defer m.parameterRegistry.muDetail.RUnlock()
	parameterConfig := m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex]

	shouldSend := false
	for _, newParameterValue := range parameter.Value {
		if newParameterValue == nil {
			log.Warnf("Received nil value for parameter %d from device %d", parameterID, deviceID)
			continue
		}
		// Check if Dimension is Valid
		if !state[deviceIndex][parameterIndex].MultiIndexHasValue(newParameterValue.DimensionID) {
			log.Errorf("Received invalid dimension id  %v for parameter %d from device %d", newParameterValue.DimensionID, parameterID, deviceID)
			continue
		}

		parameterDimension, err := state[deviceIndex][parameterIndex].MultiIndex(newParameterValue.DimensionID)
		if err != nil {
			log.Error(err)
			continue
		}
		parameterBuffer, err := parameterDimension.Value()
		if err != nil {
			log.Error(err)
			continue
		}
		if newParameterValue.Value == nil {
			if values := m.parameterRegistry.getInstanceValues(parameter.GetId()); values != nil {
				m.serverClientsStream <- &pb.Parameter{Value: values, Id: parameter.Id, Error: 0}
			}
			continue
		}

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
					continue
				}
				// FIXME: We have to clear this inconsistentcy here
				newValue := pb.ParameterValue{
					Value: &pb.ParameterValue_CurrentOption{
						CurrentOption: id,
					},
				}

				if time.Since(parameterBuffer.lastUpdate).Milliseconds() > int64(parameterConfig.QuarantineDelayMs) {
					if !proto.Equal(parameterBuffer.targetValue, &newValue) {
						parameterBuffer.targetValue = proto.Clone(&newValue).(*pb.ParameterValue)
						shouldSend = true
					}
				}

				if !proto.Equal(parameterBuffer.currentValue, &newValue) {
					parameterBuffer.currentValue = proto.Clone(&newValue).(*pb.ParameterValue)
					shouldSend = true
				}

				didSet = true

			case *pb.ParameterValue_OptionList:
				if !parameterConfig.OptionListIsDynamic {
					log.Errorf("Parameter with ID %v has no Dynamic OptionList", parameter.Id.Parameter)
					continue
				}
				m.parameterRegistry.ParameterDetail[modelIndex][parameterIndex].OptionList = v.OptionList

				m.serverClientsStream <- &pb.Parameter{
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
			if newParameterValue.Value.(*pb.ParameterValue_Floating).Floating > parameterConfig.Maximum {
				log.Errorf("Ingest Current Loop: Max violation for parameter %v", parameterID)
				continue
			}
			if newParameterValue.Value.(*pb.ParameterValue_Floating).Floating < parameterConfig.Minimum {
				log.Errorf("Ingest Current Loop: Min violation for parameter %v", parameterID)
				continue
			}
		case pb.ValueType_Integer:
			if _, ok := newParameterValue.Value.(*pb.ParameterValue_Integer); !ok {
				log.Errorf("Parameter with ID %v is Type Integer but got %T", parameter.Id.Parameter, parameterConfig.ValueType)
				continue
			}
			if newParameterValue.Value.(*pb.ParameterValue_Integer).Integer > int32(parameterConfig.Maximum) {
				log.Errorf("Ingest Current Loop: Max violation for parameter %v, got %d", parameterID, newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)
				continue
			}
			if newParameterValue.Value.(*pb.ParameterValue_Integer).Integer < int32(parameterConfig.Minimum) {
				log.Errorf("Ingest Current Loop: Min violation for parameter %v, got %d", parameterID, newParameterValue.Value.(*pb.ParameterValue_Integer).Integer)
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
				if !proto.Equal(parameterBuffer.targetValue, newParameterValue) {
					parameterBuffer.targetValue = proto.Clone(newParameterValue).(*pb.ParameterValue)
					shouldSend = true
				}
			}

			if !proto.Equal(parameterBuffer.currentValue, newParameterValue) {
				parameterBuffer.currentValue = proto.Clone(newParameterValue).(*pb.ParameterValue)
				shouldSend = true
			}
		}
		assumed := !proto.Equal(parameterBuffer.currentValue, parameterBuffer.targetValue)
		if parameterBuffer.isAssumedState != assumed {
			shouldSend = true
		}

	}

	if !shouldSend {
		return
	}

	if values := m.parameterRegistry.getInstanceValues(parameter.GetId()); values != nil {
		m.serverClientsStream <- &pb.Parameter{Value: values, Id: parameter.Id, Error: 0}
	}
}
