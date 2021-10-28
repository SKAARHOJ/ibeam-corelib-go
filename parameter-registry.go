package ibeamcorelib

import (
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	b "github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers"
	log "github.com/s00500/env_logger"
	"google.golang.org/protobuf/proto"
)

var cachedIDMap map[uint32]map[uint32]string
var cachedIDMapMu sync.RWMutex

var cachedNameMap map[uint32]map[string]uint32
var cachedNameMapMu sync.RWMutex

type parameterDetails map[uint32]map[uint32]*pb.ParameterDetail     //Parameter Details: model, parameter
type parameterStates map[uint32]map[uint32]*iBeamParameterDimension //Parameter States: device,parameter,dimension

// IBeamParameterRegistry is the storage of the core.
// It saves all Infos about the Core, Device and Models and stores the Details and current Values of the Parameter.
type IBeamParameterRegistry struct {
	muInfo           sync.RWMutex // Protects both Model and DeviceInfos
	muDetail         sync.RWMutex
	muValue          sync.RWMutex
	coreInfo         *pb.CoreInfo
	deviceInfos      map[uint32]*pb.DeviceInfo
	modelInfos       map[uint32]*pb.ModelInfo
	parameterDetail  parameterDetails //Parameter Details: model, parameter
	parameterValue   parameterStates  //Parameter States: device,parameter,dimension
	ModelAutoIDs     bool             // This is not recommended to use, please do use the proper IDs for models
	modelsDone       bool             // Sanity flag set on first call to add parameters to ensure order
	parametersDone   bool             // Sanity flag set on first call to add devices to ensure order
	connectionExists bool
}

func idFromName(name string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(name))
	return h.Sum32()
}

// device,parameter,instance
func (r *IBeamParameterRegistry) getInstanceValues(dpID *pb.DeviceParameterID, includeDynamicConfig bool) (values []*pb.ParameterValue) {
	deviceID := dpID.Device
	parameterIndex := dpID.Parameter
	_, dExists := r.parameterValue[deviceID]
	if dpID.Device == 0 || dpID.Parameter == 0 || !dExists {
		log.Error("Could not get instance values for DeviceParameterID: Device:", dpID.Device, " and param: ", dpID.Parameter)
		return nil
	}

	if _, ok := r.parameterValue[deviceID][parameterIndex]; !ok {
		log.Info(r.parameterValue[deviceID])
		log.Error("Could not get instance values for DeviceParameterID: Device:", dpID.Device, " and param: ", dpID.Parameter, " param does not exist")
		return nil
	}

	return getValues(r.parameterValue[deviceID][parameterIndex], includeDynamicConfig)
}

func getValues(dimension *iBeamParameterDimension, includeDynamicConfig bool) (values []*pb.ParameterValue) {
	if dimension.isValue() {
		value, err := dimension.getValue()
		if err != nil {
			log.Error(log.Wrap(err, "Critical error in getting value"))
			return nil
		}

		paramValue := value.getParameterValue()
		values = append(values, paramValue)

		if !includeDynamicConfig {
			return values
		}
		if value.dynamicOptions != nil {
			values = append(values, b.NewOptList(value.dynamicOptions, paramValue.DimensionID...))
		}

		if value.dynamicMax != nil {
			values = append(values, b.NewMax(*value.dynamicMax, paramValue.DimensionID...))
		}

		if value.dynamicMin != nil {
			values = append(values, b.NewMin(*value.dynamicMin, paramValue.DimensionID...))
		}
		return values
	}

	for _, dimension := range dimension.subDimensions {
		values = append(values, getValues(dimension, includeDynamicConfig)...)
	}
	return values
}

func (r *IBeamParameterRegistry) getModelID(deviceID uint32) uint32 {
	// This function assumes that mutexes are already locked
	_, dExists := r.deviceInfos[deviceID]
	if !dExists || deviceID == 0 {
		log.Fatalf("Could not get model for device with id %v.", deviceID)
	}
	return r.deviceInfos[deviceID].ModelID
}

// RegisterParameterForModels registers a parameter and its detail struct in the registry for multiple models.
func (r *IBeamParameterRegistry) RegisterParameterForModels(modelIDs []uint32, detail *pb.ParameterDetail) {
	for _, id := range modelIDs {
		if id == 0 {
			log.Fatal("RegisterParameterForModels: do not use this function with the generic model")
		}
		dt := proto.Clone(detail).(*pb.ParameterDetail)
		r.RegisterParameterForModel(id, dt)
	}
}

// RegisterParameterForModel registers a parameter and its detail struct in the registry for a single specified model and the default model if the id does not exist there yet.
func (r *IBeamParameterRegistry) RegisterParameterForModel(modelID uint32, detail *pb.ParameterDetail) (parameterIndex uint32) {
	if detail.Id == nil {
		detail.Id = new(pb.ModelParameterID)
	}
	detail.Id.Model = modelID
	return r.RegisterParameter(detail)
}

// RegisterParameter registers a parameter and its detail struct in the registry.
func (r *IBeamParameterRegistry) RegisterParameter(detail *pb.ParameterDetail) (paramID uint32) {
	r.modelsDone = true
	if r.parametersDone {
		log.Fatal("Can not unregister a parameter after registering the first device")
	}

	modelID := uint32(0)
	paramID = uint32(0)
	if detail.Id != nil {
		modelID = detail.Id.Model
		paramID = detail.Id.Parameter
	}
	r.muDetail.RLock()
	if modelID == 0 {
		// append to all models, need to check for ids

		id := r.parameterIDByName(detail.Name, modelID)
		if id != 0 {
			log.Fatal("Duplicate parameter name for ", detail.Name)
		}

		if paramID == 0 {
			paramID = idFromName(detail.Name)
		}
		r.muDetail.RUnlock()
		detail.Id = &pb.ModelParameterID{
			Parameter: paramID,
			Model:     modelID,
		}

		validateParameter(detail)

		r.muDetail.Lock()
		for aMid, modelconfig := range r.parameterDetail {
			dt := proto.Clone(detail).(*pb.ParameterDetail)
			dt.Id.Model = aMid
			modelconfig[paramID] = dt
		}
		r.muDetail.Unlock()

	} else {
		pid := r.parameterIDByName(detail.Name, 0)
		if pid != 0 {
			paramID = pid
		}

		modelconfig, exists := r.parameterDetail[modelID]
		if !exists {
			log.Fatalf("Could not register parameter '%s' for model with ID: %d", detail.Name, modelID)
		}

		if paramID == 0 {
			paramID = idFromName(detail.Name)
		}

		r.muDetail.RUnlock()
		detail.Id = &pb.ModelParameterID{
			Parameter: paramID,
			Model:     modelID,
		}

		validateParameter(detail)
		r.muDetail.Lock()

		modelconfig[paramID] = detail

		// if the default model does not have the param it still needs to be added there too!
		if pid == 0 {
			dt := proto.Clone(detail).(*pb.ParameterDetail)
			dt.Id.Model = 0
			r.parameterDetail[0][paramID] = dt
		}
		r.muDetail.Unlock()
		if pid == 0 {
			log.Debugf("ParameterDetail '%v' with ID: %v was overridden for Model %v", detail.Name, detail.Id.Parameter, detail.Id.Model)
			return
		}
	}

	if detail.GenericType == pb.GenericType_ConnectionState {
		r.connectionExists = true
	}
	log.Debugf("ParameterDetail '%v' registered with ID: %v for Model %v", detail.Name, detail.Id.Parameter, detail.Id.Model)
	return
}

// UnregisterParameterForModels removes a specific parameter for a list of models.
func (r *IBeamParameterRegistry) UnregisterParameterForModels(modelIDs []uint32, parameterName string) {
	for _, id := range modelIDs {
		r.UnregisterParameterForModel(id, parameterName)
	}
}

// UnregisterParameterForModel removes a specific parameter for a specific model.
func (r *IBeamParameterRegistry) UnregisterParameterForModel(modelID uint32, parameterName string) {
	if r.parametersDone {
		log.Fatal("Can not unregister a parameter after registering the first device")
	}

	if modelID == 0 {
		log.Fatal("Do not unregister parameters on the default model")
	}

	r.muDetail.Lock()
	id := r.parameterIDByName(parameterName, modelID)

	if id == 0 {
		log.Fatalf("Unknown parameter %s to be unregistered for model %d", parameterName, modelID)
	}

	delete(r.parameterDetail[modelID], id)
	r.muDetail.Unlock()

	log.Debugf("ParameterDetail with ID: %d removed for Model %d", id, modelID)
}

// RegisterModel registers a new Model in the Registry with given ModelInfo
func (r *IBeamParameterRegistry) RegisterModel(model *pb.ModelInfo) uint32 {
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
	if _, exists := r.modelInfos[model.Id]; exists {
		// if the id already exists count it up
		if !r.ModelAutoIDs {
			log.Fatalf("Refusing to autoassign id for model '%s', please specify an explicit ID", model.Name)
		}
		r.muDetail.RLock()
		model.Id = uint32(len(r.parameterDetail))
		log.Warnf("Autoassigning id %d for model '%s'", model.Id, model.Name)
		r.muDetail.RUnlock()
	}

	r.modelInfos[model.Id] = model
	r.muInfo.Unlock()

	r.muDetail.Lock()
	r.parameterDetail[model.Id] = map[uint32]*pb.ParameterDetail{}
	r.muDetail.Unlock()

	log.Debugf("Model '%v' registered with ID: %v ", model.Name, model.Id)
	return model.Id
}

// GetParameterNameOfModel gets the name of a parameter by id and model id
func (r *IBeamParameterRegistry) GetParameterNameOfModel(parameterID, modelID uint32) (string, error) {
	r.muDetail.RLock()
	defer r.muDetail.RUnlock()

	modelInfo, exists := r.parameterDetail[modelID]
	if !exists {
		return "", fmt.Errorf("could not find Parameter for Model with id %d", modelID)
	}

	for _, pd := range modelInfo {
		if pd.Id.Parameter == parameterID {
			return pd.Name, nil
		}
	}
	return "", fmt.Errorf("could not find Parameter with id %v", parameterID)
}

// GetParameterDetail gets the details of a parameter by id and model id
func (r *IBeamParameterRegistry) GetParameterDetail(parameterID, modelID uint32) (*pb.ParameterDetail, error) {
	r.muDetail.RLock()
	defer r.muDetail.RUnlock()

	modelInfo, exists := r.parameterDetail[modelID]
	if !exists {
		return nil, fmt.Errorf("could not find Parameter for Model with id %d", modelID)
	}

	for _, pd := range modelInfo {
		if pd.Id.Parameter == parameterID {
			return proto.Clone(pd).(*pb.ParameterDetail), nil
		}
	}
	return nil, fmt.Errorf("could not find Parameter with id %v", parameterID)
}

// GetParameterValue gets a copy of the parameter value from the state by pid, did and dimensionIDs
func (r *IBeamParameterRegistry) GetParameterValue(parameterID, deviceID uint32, dimensionID ...uint32) (*pb.ParameterValue, error) {
	r.muValue.RLock()
	defer r.muValue.RUnlock()
	state := r.parameterValue

	// first check param and deviceID
	if _, exists := state[deviceID][parameterID]; !exists {
		return nil, fmt.Errorf("getparametervalue: invalid ID for: DeviceID %d, ParameterID %d", deviceID, parameterID)
	}

	// Check if Dimension is Valid
	if !state[deviceID][parameterID].multiIndexHasValue(dimensionID) {

		return nil, fmt.Errorf("getparametervalue: invalid dimension id  %v for parameter %d and device %d", dimensionID, parameterID, deviceID)
	}

	parameterDimension, err := state[deviceID][parameterID].multiIndex(dimensionID)
	if err != nil {
		return nil, err
	}

	parameterBuffer, err := parameterDimension.getValue()
	if err != nil {
		return nil, err
	}

	valueCopy := proto.Clone(parameterBuffer.getParameterValue()).(*pb.ParameterValue)
	return valueCopy, nil
}

// GetModelIDByDeviceID is a helper to get the modelid for a specific device
func (r *IBeamParameterRegistry) GetModelIDByDeviceID(deviceID uint32) uint32 {
	r.muInfo.RLock()
	defer r.muInfo.RUnlock()

	device, exists := r.deviceInfos[deviceID]
	if !exists {
		log.Warnf("can not get model: no device with ID %d found", deviceID)
		return 0
	}
	return device.ModelID
}

// RegisterDeviceWithModelName registers a new Device in the Registry with given Modelname, if there is no it uses the generic one. If the DeviceID is 0 it will be picked automatically
func (r *IBeamParameterRegistry) RegisterDeviceWithModelName(deviceID uint32, modelName string) (deviceIndex uint32, err error) {
	modelID := uint32(0)
	r.muInfo.RLock()
	for _, m := range r.modelInfos {
		if m.Name == modelName {
			r.muInfo.RUnlock()
			return r.RegisterDevice(deviceID, modelID)
		}
	}
	r.muInfo.RUnlock()
	log.Warnf("Could not find model for '%s', using generic model", modelName)
	return r.RegisterDevice(deviceID, modelID)
}

// ReRegisterDevice registers an existing device again. This allows to reset it's state and change the type of model if necessary
// Make sure to handle the error properly
func (r *IBeamParameterRegistry) ReRegisterDevice(deviceID, modelID uint32) error {
	// Check for device exists
	r.muInfo.RLock()
	if _, exists := r.deviceInfos[deviceID]; exists {
		r.muInfo.RUnlock()
		return fmt.Errorf("could not re-register device with existing deviceid: %v", deviceID)
	}
	r.muInfo.RUnlock()

	r.muDetail.RLock()
	if _, exists := r.parameterDetail[modelID]; !exists {
		r.muDetail.RUnlock()
		log.Fatalf("Could not register device for nonexistent model with id: %v", modelID)
	}
	r.muDetail.RUnlock()

	// clear device
	r.muInfo.Lock()
	delete(r.deviceInfos, deviceID)
	r.muInfo.Unlock()

	r.muValue.Lock()
	delete(r.parameterValue, deviceID)
	r.muValue.Unlock()

	// Call register device with old ID
	_, err := r.RegisterDevice(deviceID, modelID)
	return err
}

// RegisterDevice registers a new Device in the Registry with given DeviceID and ModelID. If the DeviceID is 0 it will be picked automatically
// Make sure to handle the error properly
func (r *IBeamParameterRegistry) RegisterDevice(deviceID, modelID uint32) (uint32, error) {
	if !r.parametersDone {
		if !r.connectionExists {
			// Autoregister connection parameter
			r.RegisterParameter(&pb.ParameterDetail{
				Id:            &pb.ModelParameterID{Parameter: 1},
				Path:          "config",
				Name:          "connection",
				Label:         "Connected",
				Description:   "Connection status of device",
				GenericType:   pb.GenericType_ConnectionState,
				ControlStyle:  pb.ControlStyle_NoControl,
				FeedbackStyle: pb.FeedbackStyle_NormalFeedback,
				ValueType:     pb.ValueType_Binary,
			})
		}
	}
	r.parametersDone = true

	r.muDetail.RLock()
	defer r.muDetail.RUnlock()

	if _, exists := r.parameterDetail[modelID]; !exists {
		return 0, fmt.Errorf("could not register device for nonexistent model with id: %v", modelID)
	}

	r.muInfo.RLock()
	if _, exists := r.deviceInfos[deviceID]; exists {
		r.muInfo.RUnlock()
		return 0, fmt.Errorf("could not register device with existing deviceid: %v", deviceID)
	}
	r.muInfo.RUnlock()

	modelConfig := r.parameterDetail[modelID]

	// create device info
	// take all params from model and generate a value buffer array for all instances
	// add value buffers to the state array

	parameterDimensions := map[uint32]*iBeamParameterDimension{}
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

		initialValueDimension := iBeamParameterDimension{
			value: &ibeamParameterValueBuffer{
				dimensionID:  make([]uint32, 0),
				available:    true,
				lastUpdate:   time.Now(),
				currentValue: proto.Clone(initialValue).(*pb.ParameterValue),
				targetValue:  proto.Clone(initialValue).(*pb.ParameterValue),
			},
		}
		initialValueDimension.value.isAssumedState.Store(false)

		parameterDimensions[parameterID] = generateDimensions(parameterDetail.Dimensions, &initialValueDimension)
	}

	r.muInfo.Lock()
	if deviceID == 0 {
		deviceID = uint32(len(r.deviceInfos) + 1)
		log.Warnf("Automatically assigning DeviceID %d to device with model %d", deviceID, modelID)
	}
	r.deviceInfos[deviceID] = &pb.DeviceInfo{
		DeviceID: deviceID,
		ModelID:  modelID,
	}
	r.muInfo.Unlock()

	r.muValue.Lock()
	r.parameterValue[deviceID] = parameterDimensions
	r.muValue.Unlock()

	log.Debugf("Device '%v' registered with model: %v (%v)", deviceID, modelID, r.modelInfos[modelID].Name)
	return deviceID, nil
}

func (r *IBeamParameterRegistry) cacheIDMaps() {
	if cachedIDMap != nil {
		return
	}

	idMaps := make(map[uint32]map[uint32]string)
	nameMaps := make(map[uint32]map[string]uint32)
	r.muDetail.RLock()

	for mIndex := range r.modelInfos {
		idMap := make(map[uint32]string)
		nameMap := make(map[string]uint32)
		for _, parameter := range r.parameterDetail[mIndex] {
			idMap[parameter.Id.Parameter] = parameter.Name
			nameMap[parameter.Name] = parameter.Id.Parameter
		}
		idMaps[mIndex] = idMap
		nameMaps[mIndex] = nameMap
	}
	r.muDetail.RUnlock()

	cachedIDMapMu.Lock()
	cachedIDMap = idMaps
	cachedIDMapMu.Unlock()

	cachedNameMapMu.Lock()
	cachedNameMap = nameMaps
	cachedNameMapMu.Unlock()
}

// ParameterNameByID Get a parameter Name by ID, returns "" if not found, always uses model 0 DEPRECATED: Use PName instead
func (r *IBeamParameterRegistry) ParameterNameByID(parameterID uint32) string {
	return r.PName(parameterID)
}

// PName Get a parameter Name by ID, returns "" if not found, always uses model 0
func (r *IBeamParameterRegistry) PName(parameterID uint32) string {
	// check for device registered
	if !r.parametersDone {
		log.Error("ParameterNameByID: only call after registering the first device")
		return ""
	}

	if cachedIDMap == nil {
		r.cacheIDMaps() // make sure cachedIDMap is initialized
	}
	cachedIDMapMu.RLock()
	defer cachedIDMapMu.RUnlock()

	// use cached id map of model 0
	name, exists := cachedIDMap[0][parameterID]
	if exists {
		return name
	}

	return ""
}

// PID Get a parameterID by name, returns 0 if not found, always uses model 0
func (r *IBeamParameterRegistry) PID(parameterName string) uint32 {
	// check for device registered
	if !r.parametersDone {
		log.Error("ParameterNameByID: only call after registering the first device")
		return 0
	}

	if cachedNameMap == nil {
		r.cacheIDMaps() // make sure cachedIDMap is initialized
	}
	cachedNameMapMu.RLock()
	defer cachedNameMapMu.RUnlock()

	// use cached id map of model 0
	id, exists := cachedNameMap[0][parameterName]
	if exists {
		return id
	}

	return 0
}

// GetNameMap returns a map of all parameter names, usefull for initial state requests
func (r *IBeamParameterRegistry) GetNameMap() map[uint32]string {
	// check for device registered
	if !r.parametersDone {
		log.Error("GetNameMap: only call after registering the first device")
		return nil
	}

	if cachedNameMap == nil {
		r.cacheIDMaps() // make sure cachedIDMap is initialized
	}
	cachedIDMapMu.RLock()
	defer cachedIDMapMu.RUnlock()

	// use cached map of model 0
	nameMap := make(map[uint32]string)
	for key, value := range cachedIDMap[0] {
		nameMap[key] = value
	}
	return nameMap
}

// parameterIDByName get a parameterID by name, returns 0 if not found, not allowed to be public because it needs the mutexlock
func (r *IBeamParameterRegistry) parameterIDByName(parameterName string, modelID uint32) uint32 {
	// Function requires mutex to be fully locked before invocation
	if uint32(len(r.parameterDetail)) <= (modelID) {
		log.Fatalln("Could not register parameter for nonexistent model", modelID)
	}

	for id, param := range r.parameterDetail[modelID] {
		if param.Name == parameterName {
			return id
		}
	}
	return 0
}

func validateParameter(detail *pb.ParameterDetail) {
	// Fatals
	if detail.Name == "" {
		log.Fatalf("Parameter: ID %v: No name set", detail.Id)
	}
	if detail.ControlStyle == pb.ControlStyle_NoControl && detail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
		log.Fatalf("Parameter: '%v': Can not have no control and no feedback", detail.Name)
	}
	if detail.ControlStyle == pb.ControlStyle_ControlledIncremental && detail.ValueType != pb.ValueType_Integer {
		log.Fatalf("Parameter: '%v': Controlled Incremental only supported on integers right now", detail.Name)
	}
	if detail.ControlStyle == pb.ControlStyle_Incremental && detail.IncDecStepsLowerLimit == 0 && detail.IncDecStepsUpperLimit == 0 {
		log.Fatalf("Parameter: '%v': Incremental: please provide lower and upper range for incDecSteps", detail.Name)
	}
	if detail.ControlStyle != pb.ControlStyle_Incremental &&
		detail.ControlStyle != pb.ControlStyle_ControlledIncremental &&
		(detail.IncDecStepsLowerLimit != 0 || detail.IncDecStepsUpperLimit != 0) {
		log.Fatalf("Parameter: '%v': Lower and upper limit are only valid on Incremental Control Mode", detail.Name)
	}
	if detail.Label == "" {
		log.Fatalf("Parameter: '%v': No label set", detail.Name)
	}
	if detail.ControlStyle != pb.ControlStyle_NoControl && detail.FeedbackStyle != pb.FeedbackStyle_NoFeedback && detail.RetryCount == 0 {
		log.Fatalf("Parameter '%v': Any non assumed value (FeedbackStyle_NoFeedback) needs to have RetryCount set", detail.Name)
	}

	if detail.RetryCount != 0 && detail.ControlDelayMs == 0 {
		log.Fatalf("Parameter '%v': RetryCount will not work without ControlDelayMs being set", detail.Name)
	}

	if detail.InputCurve != pb.InputCurve_NormalInputCurve && detail.ValueType != pb.ValueType_Integer && detail.ValueType != pb.ValueType_Floating {
		log.Fatalf("Parameter '%v': InputCurves can only be used on Integer or Float values", detail.Name)
	}

	if detail.DisplayFloatPrecision != pb.FloatPrecision_UndefinedFloatPrecision && detail.ValueType != pb.ValueType_Floating {
		log.Fatalf("Parameter '%v': Float Percision is only usable on floats", detail.Name)
	}

	// Dimension check
	for dId, dimensionDetail := range detail.Dimensions {
		if dimensionDetail.Count == 0 && len(dimensionDetail.ElementLabels) == 0 {
			log.Fatalf("Parameter '%v' Dimension '%d': Count is 0 and element labels are empty, this will cause a staleless parameter", detail.Name, dId)
		}
	}

	// Metavalue checks
	for mName, mDetail := range detail.MetaDetails {
		if mDetail.MetaType != pb.ParameterMetaType_MetaInteger && mDetail.MetaType != pb.ParameterMetaType_MetaFloating {
			if mDetail.Minimum != 0 || mDetail.Maximum != 0 {
				log.Warnf("Parameter metavalue '%s' of type %v has useless min / max values", mName, mDetail.MetaType)
			}
		}

		if mDetail.MetaType == pb.ParameterMetaType_MetaOption && len(mDetail.Options) == 0 {
			log.Fatalf("Parameter metavalue '%s' of type MetaOption has no option list", mName)
		} else if mDetail.MetaType != pb.ParameterMetaType_MetaOption && len(mDetail.Options) > 0 {
			log.Warnf("Parameter metavalue '%s' of type %v has useless option list", mName, mDetail.MetaType)
		}

	}

	// ValueType Checks
	switch detail.ValueType {
	case pb.ValueType_Floating:
		fallthrough
	case pb.ValueType_Integer:
		if detail.Minimum == 0 && detail.Maximum == 0 {
			log.Fatalf("Parameter: '%v': Integer needs min/max set", detail.Name)
		}
	case pb.ValueType_Binary:
		if detail.ControlStyle == pb.ControlStyle_Incremental {
			log.Fatalf("Parameter: '%v': Binary can not have incremental control", detail.Name)
		}
	case pb.ValueType_NoValue:
		if detail.FeedbackStyle != pb.FeedbackStyle_NoFeedback {
			log.Fatalf("Parameter: '%v': NoValue can not have Feedback", detail.Name)
		}
		if detail.Minimum != 0 || detail.Maximum != 0 {
			log.Fatalf("Parameter: '%v': NoValue can not min/max", detail.Name)
		}
	}

	// Warnings
	if detail.Description == "" {
		log.Warnf("Parameter '%v': No description set", detail.Name)
	}
}
