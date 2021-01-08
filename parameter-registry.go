package ibeamcorelib

import (
	"sync"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	"github.com/jinzhu/copier"
	log "github.com/s00500/env_logger"
)

type parameterDetails []map[int]*pb.ParameterDetail     //Parameter Details: model, parameter
type parameterStates []map[int]*IbeamParameterDimension //Parameter States: device,parameter,dimension

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
	allowAutoIDs    bool
}

// AllowAutoIDs Allowing Automatic IDs for parameters, this is only meant for initial development
func (r *IbeamParameterRegistry) AllowAutoIDs() {
	log.Warn("Allowing Automatic IDs for parameters, this is only meant for initial development!!!")
	r.allowAutoIDs = true
}

// device,parameter,instance
func (r *IbeamParameterRegistry) getInstanceValues(dpID *pb.DeviceParameterID) (values []*pb.ParameterValue) {
	deviceIndex := int(dpID.Device) - 1
	parameterIndex := int(dpID.Parameter)

	if dpID.Device == 0 || dpID.Parameter == 0 || len(r.parameterValue) <= deviceIndex {
		log.Error("Could not get instance values for DeviceParameterID: Device:", dpID.Device, " and param: ", dpID.Parameter)
		return nil
	}

	if _, ok := r.parameterValue[deviceIndex][parameterIndex]; !ok {
		log.Error("Could not get instance values for DeviceParameterID: Device:", dpID.Device, " and param: ", dpID.Parameter)
		return nil
	}

	return getValues(r.parameterValue[deviceIndex][parameterIndex])
}

func getValues(dimension *IbeamParameterDimension) (values []*pb.ParameterValue) {
	if dimension.isValue() {
		value, err := dimension.Value()
		if err != nil {
			log.Fatal(err)
		}
		values = append(values, value.getParameterValue())
	} else {
		for _, dimension := range dimension.subDimensions {
			values = append(values, getValues(dimension)...)
		}
	}
	return values
}

func (r *IbeamParameterRegistry) getModelIndex(deviceID uint32) int {
	if len(r.DeviceInfos) < int(deviceID) || deviceID == 0 {
		log.Panicf("Could not get model for device with id %v. DeviceInfos has lenght of %v", deviceID, len(r.DeviceInfos))
	}
	return int(r.DeviceInfos[deviceID-1].ModelID)
}

// RegisterParameter registers a Parameter and his Details in the Registry.
func (r *IbeamParameterRegistry) RegisterParameter(detail *pb.ParameterDetail) (parameterIndex uint32) {
	mid := uint32(0)
	parameterIndex = uint32(0)
	if detail.Id != nil {
		mid = detail.Id.Model
		parameterIndex = detail.Id.Parameter
	}
	r.muDetail.RLock()
	if uint32(len(r.ParameterDetail)) <= (mid) {
		log.Panic("Could not register parameter for nonexistent model ", mid)
		return 0
	}

	if mid == 0 {
		// append to all models, need to check for ids
		defaultModelConfig := &r.ParameterDetail[0]
		if parameterIndex == 0 {
			if !r.allowAutoIDs {
				log.Panicf("Missing ID on parameter '%s'", detail.Name)
			}
			parameterIndex = uint32(len(*defaultModelConfig) + 1)
		}
		r.muDetail.RUnlock()
		detail.Id = &pb.ModelParameterID{
			Parameter: parameterIndex,
			Model:     mid,
		}

		validateParameter(detail)

		r.muDetail.Lock()
		for _, modelconfig := range r.ParameterDetail {
			modelconfig[int(parameterIndex)] = detail
		}
		r.muDetail.Unlock()

	} else {
		modelconfig := &r.ParameterDetail[mid]
		if parameterIndex == 0 {
			parameterIndex = uint32(len(*modelconfig) + 1)
		}
		r.muDetail.RUnlock()
		detail.Id = &pb.ModelParameterID{
			Parameter: parameterIndex,
			Model:     mid,
		}

		validateParameter(detail)
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
	model.Id = uint32(len(r.ParameterDetail))
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
	r.muDetail.RLock()
	defer r.muDetail.RUnlock()

	if uint32(len(r.ParameterDetail)) <= (modelID) {
		log.Panicf("Could not register device for nonexistent model with id: %v", modelID)
	}

	modelConfig := r.ParameterDetail[modelID]

	// create device info
	// take all params from model and generate a value buffer array for all instances
	// add value buffers to the state array

	parameterDimensions := map[int]*IbeamParameterDimension{}
	for _, parameterDetail := range modelConfig {
		parameterID := parameterDetail.Id.Parameter

		// Integer is default
		initialValue := &pb.ParameterValue{
			DimensionID:    []uint32{},
			Available:      true,
			IsAssumedState: false,
			Invalid:        true,
			Value:          &pb.ParameterValue_Integer{Integer: 0},
			MetaValues:     map[string]*pb.ParameterMetaValue{},
		}

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
			log.Panicf("It is not recommended to use more than 3 dimensions, if needed please contact the maintainer")
		}

		dimensionConfig := []uint32{}
		initialValueDimension := IbeamParameterDimension{
			value: &IBeamParameterValueBuffer{
				dimensionID:    make([]uint32, 0),
				available:      true,
				isAssumedState: true,
				lastUpdate:     time.Now(),
			},
		}

		copier.Copy(&initialValueDimension.value.currentValue, initialValue)
		copier.Copy(&initialValueDimension.value.targetValue, initialValue)

		for _, dimension := range parameterDetail.Dimensions {
			dimensionConfig = append(dimensionConfig, dimension.Count)
		}

		parameterDimensions[int(parameterID)] = generateDimensions(dimensionConfig, &initialValueDimension)
	}

	r.muInfo.Lock()
	deviceIndex = uint32(len(r.DeviceInfos) + 1)
	r.DeviceInfos = append(r.DeviceInfos, &pb.DeviceInfo{
		DeviceID: deviceIndex,
		ModelID:  modelID,
	})
	r.muInfo.Unlock()

	r.muValue.Lock()
	r.parameterValue = append(r.parameterValue, parameterDimensions)
	r.muValue.Unlock()

	log.Debugf("Device '%v' registered with model: %v (%v)", deviceIndex, modelID, r.ModelInfos[modelID].Name)
	return deviceIndex
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

func validateParameter(detail *pb.ParameterDetail) {
	// Fatals
	if detail.Name == "" {
		log.Fatalf("Could not validate parameter ID %v: No name set", detail.Id)
	}
	if detail.ControlStyle == pb.ControlStyle_NoControl && detail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
		log.Fatalf("Could not validate parameter '%v': Can not have no control and no feedback", detail.Name)
	}
	if detail.ControlStyle == pb.ControlStyle_ControlledIncremental && detail.ValueType != pb.ValueType_Integer {
		log.Fatalf("Could not validate parameter '%v': Controlled Incremental only supported on integers right now", detail.Name)
	}
	if detail.ControlStyle == pb.ControlStyle_Incremental && detail.IncDecStepsLowerRange == 0 && detail.IncDecStepsUpperRange == 0 {
		log.Fatalf("Could not validate parameter '%v': Incremental: please provide lower and upper range for incDecSteps", detail.Name)
	}
	if detail.Label == "" {
		log.Fatalf("Could not validate parameter '%v': No label set", detail.Name)
	}
	if detail.ControlStyle != pb.ControlStyle_NoControl && detail.FeedbackStyle != pb.FeedbackStyle_NoFeedback && detail.RetryCount == 0 {
		log.Fatalf("Parameter '%v': Any non assumed value (FeedbackStyle_NoFeedback) needs to have RetryCount set", detail.Name)
	}

	// ValueType Checks
	switch detail.ValueType {
	case pb.ValueType_Integer:
		if detail.Minimum == 0 && detail.Maximum == 0 {
			log.Fatalf("Could not validate parameter '%v': Integer needs min/max set", detail.Name)
		}
	case pb.ValueType_Binary:
		if detail.ControlStyle == pb.ControlStyle_Incremental {
			log.Fatalf("Could not validate parameter '%v': Binary con not have incremental control", detail.Name)
		}
	}

	// Warnings
	if detail.ShortLabel == "" {
		log.Warnf("Parameter '%v': No short label set", detail.Name)
	}
	if detail.Description == "" {
		log.Warnf("Parameter '%v': No description set", detail.Name)
	}
}
