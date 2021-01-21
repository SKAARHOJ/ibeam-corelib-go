package ibeamcorelib

import (
	"fmt"
	"sync"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
	"google.golang.org/protobuf/proto"
)

type parameterDetails map[uint32]map[uint32]*pb.ParameterDetail //Parameter Details: model, parameter
type parameterStates []map[uint32]*IbeamParameterDimension      //Parameter States: device,parameter,dimension

// IbeamParameterRegistry is the storage of the core.
// It saves all Infos about the Core, Device and Models and stores the Details and current Values of the Parameter.
type IbeamParameterRegistry struct {
	muInfo          sync.RWMutex // Protects both Model and DeviceInfos
	muDetail        sync.RWMutex
	muValue         sync.RWMutex
	coreInfo        *pb.CoreInfo
	DeviceInfos     []*pb.DeviceInfo
	ModelInfos      map[uint32]*pb.ModelInfo
	ParameterDetail parameterDetails //Parameter Details: model, parameter
	parameterValue  parameterStates  //Parameter States: device,parameter,dimension
	allowAutoIDs    bool
	modelsDone      bool // Sanity flag set on first call to add parameters to ensure order
	parametersDone  bool // Sanity flag set on first call to add devices to ensure order
}

// AllowAutoIDs Allowing Automatic IDs for parameters, this is only meant for initial development
func (r *IbeamParameterRegistry) AllowAutoIDs() {
	log.Warn("Allowing Automatic IDs for parameters, this is only meant for initial development!!!")
	r.allowAutoIDs = true
}

// device,parameter,instance
func (r *IbeamParameterRegistry) getInstanceValues(dpID *pb.DeviceParameterID) (values []*pb.ParameterValue) {
	deviceIndex := int(dpID.Device) - 1
	parameterIndex := dpID.Parameter

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

func (r *IbeamParameterRegistry) getModelIndex(deviceID uint32) uint32 {
	if len(r.DeviceInfos) < int(deviceID) || deviceID == 0 {
		log.Fatalf("Could not get model for device with id %v. DeviceInfos has lenght of %v", deviceID, len(r.DeviceInfos))
	}
	return r.DeviceInfos[deviceID-1].ModelID
}

// RegisterParameterForModels registers a parameter and its detail struct in the registry for multiple models.
func (r *IbeamParameterRegistry) RegisterParameterForModels(modelIDs []uint32, detail *pb.ParameterDetail) {
	for _, id := range modelIDs {
		if id == 0 {
			log.Fatal("RegisterParameterForModels: do not use this function with the generic model")
		}
		dt := proto.Clone(detail).(*pb.ParameterDetail)
		r.RegisterParameterForModel(id, dt)
	}
}

// RegisterParameterForModel registers a parameter and its detail struct in the registry for a single specified model and the default model if the id does not exist there yet.
func (r *IbeamParameterRegistry) RegisterParameterForModel(modelID uint32, detail *pb.ParameterDetail) (parameterIndex uint32) {
	if detail.Id == nil {
		detail.Id = new(pb.ModelParameterID)
	}
	detail.Id.Model = modelID
	return r.RegisterParameter(detail)
}

// RegisterParameter registers a parameter and its detail struct in the registry.
func (r *IbeamParameterRegistry) RegisterParameter(detail *pb.ParameterDetail) (parameterIndex uint32) {
	r.modelsDone = true
	if r.parametersDone {
		log.Fatal("Can not unregister a parameter after registering the first device")
	}

	mid := uint32(0)
	parameterIndex = uint32(0)
	if detail.Id != nil {
		mid = detail.Id.Model
		parameterIndex = detail.Id.Parameter
	}
	r.muDetail.RLock()
	if mid == 0 {
		// append to all models, need to check for ids

		_, found := r.findParameterByName(mid, detail.Name)
		if found {
			log.Fatal("Duplicate parameter name for ", detail.Name)
		}

		defaultModelConfig := r.ParameterDetail[0]
		if parameterIndex == 0 {
			if !r.allowAutoIDs {
				log.Fatalf("Missing ID on parameter '%s'", detail.Name)
			}
			parameterIndex = uint32(len(defaultModelConfig) + 1)
		}
		r.muDetail.RUnlock()
		detail.Id = &pb.ModelParameterID{
			Parameter: parameterIndex,
			Model:     mid,
		}

		validateParameter(detail)

		r.muDetail.Lock()
		for aMid, modelconfig := range r.ParameterDetail {
			dt := proto.Clone(detail).(*pb.ParameterDetail)
			dt.Id.Model = uint32(aMid)
			modelconfig[parameterIndex] = dt
		}
		r.muDetail.Unlock()

	} else {
		paramid, found := r.findParameterByName(0, detail.Name)
		if found {
			parameterIndex = uint32(paramid)
		}

		modelconfig, exists := r.ParameterDetail[mid]
		if !exists {
			log.Fatalf("Could not register parameter '%s' for model with ID: %d", detail.Name, mid)
		}
		if parameterIndex == 0 {
			if !r.allowAutoIDs {
				log.Fatalf("Missing ID on parameter '%s'", detail.Name)
			}
			parameterIndex = uint32(len(r.ParameterDetail[0]) + 1)
		}
		r.muDetail.RUnlock()
		detail.Id = &pb.ModelParameterID{
			Parameter: parameterIndex,
			Model:     mid,
		}

		validateParameter(detail)
		r.muDetail.Lock()

		modelconfig[parameterIndex] = detail

		// if the default model does not have the param it still needs to be added there too!
		if !found {
			dt := proto.Clone(detail).(*pb.ParameterDetail)
			dt.Id.Model = 0
			r.ParameterDetail[0][parameterIndex] = dt
		}
		r.muDetail.Unlock()
		if found {
			log.Debugf("ParameterDetail '%v' with ID: %v was overridden for Model %v", detail.Name, detail.Id.Parameter, detail.Id.Model)
			return
		}
	}

	log.Debugf("ParameterDetail '%v' registered with ID: %v for Model %v", detail.Name, detail.Id.Parameter, detail.Id.Model)
	return
}

// UnregisterParameterForModels removes a specific parameter for a list of models.
func (r *IbeamParameterRegistry) UnregisterParameterForModels(modelIDs []uint32, parameterName string) {
	for _, id := range modelIDs {
		r.UnregisterParameterForModel(id, parameterName)
	}
}

// UnregisterParameterForModel removes a specific parameter for a specific model.
func (r *IbeamParameterRegistry) UnregisterParameterForModel(modelID uint32, parameterName string) {
	if r.parametersDone {
		log.Fatal("Can not unregister a parameter after registering the first device")
	}

	if modelID == 0 {
		log.Fatal("Do not unregister parameters on the default model")
	}

	r.muDetail.Lock()
	id, found := r.findParameterByName(modelID, parameterName)

	if !found {
		log.Fatalf("Unknown parameter %s to be unregistered for model %d", parameterName, modelID)
	}

	delete(r.ParameterDetail[modelID], id)
	r.muDetail.Unlock()

	log.Debugf("ParameterDetail with ID: %d removed for Model %d", id, modelID)
}

// RegisterModel registers a new Model in the Registry with given ModelInfo
func (r *IbeamParameterRegistry) RegisterModel(model *pb.ModelInfo) uint32 {
	if r.modelsDone {
		log.Fatal("Can not register a new model after registering parameters")
	}

	if model.Name == "" {
		log.Fatal("please specify a name for all models")
	}

	if model.Description == "" {
		log.Fatal("please specify a description for all models")
	}

	r.muInfo.Lock()
	if _, exists := r.ModelInfos[model.Id]; exists {
		// if the id already exists count it up
		if !r.allowAutoIDs {
			log.Fatalf("Refusing to autoassign id for model '%s', please specify an explicit ID", model.Name)
		}
		r.muDetail.RLock()
		model.Id = uint32(len(r.ParameterDetail))
		log.Warnf("Autoassigning id %d for model '%s'", model.Id, model.Name)
		r.muDetail.RUnlock()
	}

	r.ModelInfos[model.Id] = model
	r.muInfo.Unlock()

	r.muDetail.Lock()
	r.ParameterDetail[model.Id] = map[uint32]*pb.ParameterDetail{}
	r.muDetail.Unlock()

	log.Debugf("Model '%v' registered with ID: %v ", model.Name, model.Id)
	return model.Id
}

// GetParameterNameOfModel gets the name of a parameter by id and model id
func (r *IbeamParameterRegistry) GetParameterNameOfModel(parameterID, modelID uint32) (string, error) {
	r.muDetail.RLock()
	defer r.muDetail.RUnlock()

	modelInfo, exists := r.ParameterDetail[modelID]
	if !exists {
		return "", fmt.Errorf("Could not find Parameter for Model with id %d", modelID)
	}

	for _, pd := range modelInfo {
		if pd.Id.Parameter == parameterID {
			return pd.Name, nil
		}
	}
	return "", fmt.Errorf("Could not find Parameter with id %v", parameterID)
}

// GetModelIDByDeviceID is a helper to get the modelid for a specific device
func (r *IbeamParameterRegistry) GetModelIDByDeviceID(deviceID uint32) uint32 {
	r.muInfo.RLock()
	defer r.muInfo.RUnlock()
	if int(deviceID) > len(r.DeviceInfos) {
		log.Warnf("can not get model: no device with ID %d found", deviceID)
		return 0
	}
	device := r.DeviceInfos[deviceID-1]
	return device.ModelID
}

// RegisterDeviceWithModelName registers a new Device in the Registry with given Modelname, if there is no it uses the generic one
func (r *IbeamParameterRegistry) RegisterDeviceWithModelName(modelName string) (deviceIndex uint32) {
	modelID := uint32(0)
	r.muInfo.RLock()
	for _, m := range r.ModelInfos {
		if m.Name == modelName {
			r.muInfo.RUnlock()
			return r.RegisterDevice(modelID)
		}
	}
	r.muInfo.RUnlock()
	log.Warnf("Could not find model for '%s', using generic model", modelName)
	return r.RegisterDevice(modelID)
}

// RegisterDevice registers a new Device in the Registry with given ModelID
func (r *IbeamParameterRegistry) RegisterDevice(modelID uint32) (deviceIndex uint32) {
	r.parametersDone = true

	r.muDetail.RLock()
	defer r.muDetail.RUnlock()

	if _, exists := r.ParameterDetail[modelID]; !exists {
		log.Fatalf("Could not register device for nonexistent model with id: %v", modelID)
	}

	modelConfig := r.ParameterDetail[modelID]

	// create device info
	// take all params from model and generate a value buffer array for all instances
	// add value buffers to the state array

	parameterDimensions := map[uint32]*IbeamParameterDimension{}
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
			log.Fatalf("It is not recommended to use more than 3 dimensions, if needed please contact the maintainer")
		}

		dimensionConfig := []uint32{}
		initialValueDimension := IbeamParameterDimension{
			value: &IBeamParameterValueBuffer{
				dimensionID:    make([]uint32, 0),
				available:      true,
				isAssumedState: true,
				lastUpdate:     time.Now(),
				currentValue:   proto.Clone(initialValue).(*pb.ParameterValue),
				targetValue:    proto.Clone(initialValue).(*pb.ParameterValue),
			},
		}

		for _, dimension := range parameterDetail.Dimensions {
			dimensionConfig = append(dimensionConfig, dimension.Count)
		}

		parameterDimensions[parameterID] = generateDimensions(dimensionConfig, &initialValueDimension)
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

func (r *IbeamParameterRegistry) findParameterByName(modelID uint32, parameterName string) (id uint32, found bool) {
	// Function requires mutex to be fully locked before invocation
	if uint32(len(r.ParameterDetail)) <= (modelID) {
		log.Fatalln("Could not register parameter for nonexistent model", modelID)
	}

	for id, param := range r.ParameterDetail[modelID] {
		if param.Name == parameterName {
			return id, true
		}
	}
	return id, false
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
	if detail.ControlStyle == pb.ControlStyle_Incremental && detail.IncDecStepsLowerLimit == 0 && detail.IncDecStepsUpperLimit == 0 {
		log.Fatalf("Could not validate parameter '%v': Incremental: please provide lower and upper range for incDecSteps", detail.Name)
	}
	if detail.ControlStyle != pb.ControlStyle_Incremental &&
		detail.ControlStyle != pb.ControlStyle_ControlledIncremental &&
		(detail.IncDecStepsLowerLimit != 0 || detail.IncDecStepsUpperLimit != 0) {
		log.Fatalf("Could not validate parameter '%v': Lower and upper limit are only valid on Incremental Control Mode", detail.Name)
	}
	if detail.Label == "" {
		log.Fatalf("Could not validate parameter '%v': No label set", detail.Name)
	}
	if detail.ControlStyle != pb.ControlStyle_NoControl && detail.FeedbackStyle != pb.FeedbackStyle_NoFeedback && detail.RetryCount == 0 {
		log.Fatalf("Parameter '%v': Any non assumed value (FeedbackStyle_NoFeedback) needs to have RetryCount set", detail.Name)
	}

	// ValueType Checks
	switch detail.ValueType {
	case pb.ValueType_Floating:
		fallthrough
	case pb.ValueType_Integer:
		if detail.Minimum == 0 && detail.Maximum == 0 {
			log.Fatalf("Could not validate parameter '%v': Integer needs min/max set", detail.Name)
		}
	case pb.ValueType_Binary:
		if detail.ControlStyle == pb.ControlStyle_Incremental {
			log.Fatalf("Could not validate parameter '%v': Binary can not have incremental control", detail.Name)
		}
	case pb.ValueType_NoValue:
		if detail.FeedbackStyle != pb.FeedbackStyle_NoFeedback {
			log.Fatalf("Could not validate parameter '%v': NoValue can not have Feedback", detail.Name)
		}
		if detail.Minimum != 0 || detail.Maximum != 0 {
			log.Fatalf("Could not validate parameter '%v': NoValue can not min/max", detail.Name)
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
