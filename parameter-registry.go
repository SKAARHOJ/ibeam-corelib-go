package ibeam_corelib

import (
	"sync"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
)

type parameterDetails []map[int]*pb.ParameterDetail       //Parameter Details: model, parameter
type parameterStates []map[int][]*IbeamParameterDimension //Parameter States: device,parameter,dimension

// IbeamParameterRegistry is the storage of the core.
// It saves all Infos about the Core, Device and Models and stores the Details and current Values of the Parameter.
type IbeamParameterRegistry struct {
	muInfo          sync.RWMutex
	muDetail        sync.RWMutex
	muValue         sync.RWMutex
	coreInfo        pb.CoreInfo
	DeviceInfos     []*pb.DeviceInfo
	ModelInfos      []*pb.ModelInfo
	ParameterDetail parameterDetails //Parameter Details: model, parameter
	parameterValue  parameterStates  //Parameter States: device,parameter,dimension
}

// device,parameter,instance
func (r *IbeamParameterRegistry) getInstanceValues(dpID pb.DeviceParameterID) (values []*pb.ParameterValue) {
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

	var getValues func([]*IbeamParameterDimension)
	getValues = func(dimensions []*IbeamParameterDimension) {
		for _, value := range dimensions {
			if value.isValue() {
				values = append(values, value.value.getParameterValue())
			} else {
				for _, dimension := range dimensions {
					for _, subDimension := range dimension.subDimensions {
						getValues([]*IbeamParameterDimension{subDimension})
					}
				}
			}
		}
	}

	getValues(r.parameterValue[deviceIndex][parameterIndex])

	/*
		for _, value := range r.parameterValue[deviceIndex][parameterIndex] {
			for _, dimension := range value {

			}
			values = append(values, value.value.getParameterValue())
		}
	*/
	return
}

func (r *IbeamParameterRegistry) getModelIndex(deviceID uint32) int {
	r.muInfo.RLock()
	defer r.muInfo.RUnlock()
	if len(r.DeviceInfos) < int(deviceID) || deviceID == 0 {
		log.Panicf("Could not get model for device with id %v. DeviceInfos has lenght of %v", deviceID, len(r.DeviceInfos))
	}
	return int(r.DeviceInfos[deviceID-1].ModelID - 1)
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
