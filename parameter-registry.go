package ibeamcorelib

import (
	"fmt"
	"hash/fnv"
	"time"

	"sync"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	b "github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers"
	"github.com/SKAARHOJ/ibeam-corelib-go/syncmap"
	log "github.com/s00500/env_logger"
	"go.uber.org/atomic"
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
	parametersDone   atomic.Bool      // Sanity flag set on first call to add devices to ensure order
	connectionExists bool
	log              *log.Entry

	deviceLastEvent *syncmap.Map[uint32, time.Time] //Needs to be used via getter/setter functions on registry internally

	// Options for individual Parameters or models
	modelRateLimiter         map[uint32]uint // minimum milliseconds between commands sent
	defaultValidParams       []*pb.ModelParameterID
	defaultUnavailableParams []*pb.ModelParameterID
	parameterFlags           map[uint32]map[uint32][]ParamBufferConfigFlag // Model -> Parameter

	// debug info
	parameterCount uint
	dimensionCount uint
}

func idFromName(name string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(name))
	return h.Sum32()
}

// device,parameter,instance
func (r *IBeamParameterRegistry) getInstanceValues(dpID *pb.DeviceParameterID, includeDynamicConfig bool, dimensionFilter ...[]uint32) (values []*pb.ParameterValue) {
	deviceID := dpID.Device
	parameterIndex := dpID.Parameter
	_, dExists := r.parameterValue[deviceID]
	if dpID.Device == 0 || dpID.Parameter == 0 || !dExists {
		r.log.Error("Could not get instance values for DeviceParameterID: Device:", dpID.Device, " and param: ", dpID.Parameter)
		return nil
	}

	if _, ok := r.parameterValue[deviceID][parameterIndex]; !ok {
		r.log.Error("Could not get instance values for DeviceParameterID: Device:", dpID.Device, " and param: ", dpID.Parameter, " param does not exist")
		return nil
	}

	return getValues(r.log, r.parameterValue[deviceID][parameterIndex], includeDynamicConfig, dimensionFilter...)
}

func getValues(rlog *log.Entry, dimension *iBeamParameterDimension, includeDynamicConfig bool, dimensionFilter ...[]uint32) (values []*pb.ParameterValue) {
	if dimension.isValue() {
		value, err := dimension.getValue()
		if err != nil {
			rlog.Error(log.Wrap(err, "Critical error in getting value"))
			return nil
		}

		paramValue := value.getParameterValue()

		if dimensionFilter == nil {
			values = append(values, paramValue)
		}

		for _, df := range dimensionFilter {
			if dimsEqual(paramValue.DimensionID, df) {
				values = append(values, paramValue)
			}
		}

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
		values = append(values, getValues(rlog, dimension, includeDynamicConfig, dimensionFilter...)...)
	}
	return values
}

func (r *IBeamParameterRegistry) getModelID(deviceID uint32) uint32 {
	// This function assumes that mutexes are already locked
	_, dExists := r.deviceInfos[deviceID]
	if !dExists || deviceID == 0 {
		r.log.Fatalf("Could not get model for device with id %v.", deviceID)
	}
	return r.deviceInfos[deviceID].ModelID
}
func (r *IBeamParameterRegistry) getModelIDErr(deviceID uint32) (uint32, error) {
	// This function assumes that mutexes are already locked
	_, dExists := r.deviceInfos[deviceID]
	if !dExists || deviceID == 0 {
		return 0, fmt.Errorf("Could not get model for device with id %v.", deviceID)
	}
	return r.deviceInfos[deviceID].ModelID, nil
}

// RegisterParameterForModels registers a parameter and its detail struct in the registry for multiple models.
func (r *IBeamParameterRegistry) RegisterParameterForModels(modelIDs []uint32, detail *pb.ParameterDetail, registerOptions ...RegisterOption) uint32 {
	var returnID uint32
	for _, id := range modelIDs {
		if id == 0 {
			r.log.Fatal("RegisterParameterForModels: do not use this function with the generic model")
		}
		dt := proto.Clone(detail).(*pb.ParameterDetail)
		returnID = r.RegisterParameterForModel(id, dt, registerOptions...)
	}
	return returnID
}

// RegisterParameterForModel registers a parameter and its detail struct in the registry for a single specified model and the default model if the id does not exist there yet.
func (r *IBeamParameterRegistry) RegisterParameterForModel(modelID uint32, detail *pb.ParameterDetail, registerOptions ...RegisterOption) (parameterIndex uint32) {
	if detail.Id == nil {
		detail.Id = new(pb.ModelParameterID)
	}
	detail.Id.Model = modelID
	return r.RegisterParameter(detail, registerOptions...)
}

// RegisterParameter registers a parameter and its detail struct in the registry.
func (r *IBeamParameterRegistry) RegisterParameter(detail *pb.ParameterDetail, registerOptions ...RegisterOption) (paramID uint32) {
	r.modelsDone = true
	if r.parametersDone.Load() {
		r.log.Fatal("Can not unregister a parameter after registering the first device")
	}

	modelID := uint32(0)
	paramID = uint32(0)
	if detail.Id != nil {
		modelID = detail.Id.Model
		paramID = detail.Id.Parameter
	}

	r.parameterCount++

	// Could make this not do stuff for no value... But this is currently not really true though...
	//if detail.ValueType != pb.ValueType_NoValue {
	paramDims := 1
	for _, dim := range detail.Dimensions {
		paramDims *= int(dim.Count) + len(dim.ElementLabels)
	}
	r.dimensionCount += uint(paramDims)
	//}

	r.muDetail.RLock()
	if modelID == 0 {
		// append to all models, need to check for ids

		id := r.parameterIDByName(detail.Name, modelID)
		if id != 0 {
			r.log.Fatal("Duplicate parameter name for ", detail.Name)
		}

		if paramID == 0 {
			paramID = idFromName(detail.Name)
		}
		r.muDetail.RUnlock()
		detail.Id = &pb.ModelParameterID{
			Parameter: paramID,
			Model:     modelID,
		}

		validateParameter(r.log, detail)

		r.muDetail.Lock()
		for _, o := range registerOptions {
			o(r, detail.Id)
		}
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
			r.log.Fatalf("Could not register parameter '%s' for model with ID: %d", detail.Name, modelID)
		}

		if paramID == 0 {
			paramID = idFromName(detail.Name)
		}

		r.muDetail.RUnlock()
		detail.Id = &pb.ModelParameterID{
			Parameter: paramID,
			Model:     modelID,
		}

		validateParameter(r.log, detail)
		r.muDetail.Lock()
		for _, o := range registerOptions {
			o(r, detail.Id)
		}
		modelconfig[paramID] = detail

		// if the default model does not have the param it still needs to be added there too!
		if pid == 0 {
			dt := proto.Clone(detail).(*pb.ParameterDetail)
			dt.Id.Model = 0
			r.parameterDetail[0][paramID] = dt
		}

		r.muDetail.Unlock()
		if pid == 0 {
			r.log.Debugf("ParameterDetail '%v' with ID: %v was overridden for Model %v", detail.Name, detail.Id.Parameter, detail.Id.Model)
			return
		}
	}

	if detail.GenericType == pb.GenericType_ConnectionState {
		r.connectionExists = true
	}

	r.log.Debugf("ParameterDetail '%v' registered with ID: %v for Model %v", detail.Name, detail.Id.Parameter, detail.Id.Model)
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
	if r.parametersDone.Load() {
		r.log.Fatal("Can not unregister a parameter after registering the first device")
	}

	if modelID == 0 {
		r.log.Fatal("Do not unregister parameters on the default model")
	}

	r.muDetail.Lock()
	id := r.parameterIDByName(parameterName, modelID)

	if id == 0 {
		r.log.Fatalf("Unknown parameter %s to be unregistered for model %d", parameterName, modelID)
	}

	delete(r.parameterDetail[modelID], id)
	r.muDetail.Unlock()

	r.log.Debugf("ParameterDetail with ID: %d removed for Model %d", id, modelID)
}

// RegisterModel registers a new Model in the Registry with given ModelInfo
func (r *IBeamParameterRegistry) RegisterModel(model *pb.ModelInfo, registerOptions ...ModelRegisterOption) uint32 {
	if r.modelsDone {
		r.log.Fatal("Can not register a new model after registering parameters")
	}

	validateModel(r.log, model)

	r.muInfo.Lock()
	if _, exists := r.modelInfos[model.Id]; exists {
		// if the id already exists count it up
		if !r.ModelAutoIDs {
			r.log.Fatalf("Refusing to autoassign id for model '%s', please specify an explicit ID", model.Name)
		}
		r.muDetail.RLock()
		model.Id = uint32(len(r.parameterDetail))
		r.log.Warnf("Autoassigning id %d for model '%s'", model.Id, model.Name)
		r.muDetail.RUnlock()
	}

	r.modelInfos[model.Id] = model
	for _, o := range registerOptions {
		o(r, model.Id)
	}
	r.muInfo.Unlock()

	r.muDetail.Lock()
	r.parameterDetail[model.Id] = map[uint32]*pb.ParameterDetail{}
	r.muDetail.Unlock()

	r.log.Debugf("Model '%v' registered with ID: %v ", model.Name, model.Id)
	return model.Id
}

// GetParameterDetail gets the details of a parameter by id and model id
func (r *IBeamParameterRegistry) GetParameterDetail(parameterID, modelID uint32) (*pb.ParameterDetail, error) {
	r.muDetail.RLock()
	defer r.muDetail.RUnlock()

	return r.getParameterDetail(parameterID, modelID)
}

// getParameterDetail gets the details of a parameter by id and model id
func (r *IBeamParameterRegistry) getParameterDetail(parameterID, modelID uint32) (*pb.ParameterDetail, error) {
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

// GetParameterValue gets a copy of the new parameter value from the state by pid, did and dimensionIDs
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

// GetParameterCurrentValue gets a copy of the current parameter value from the state by pid, did and dimensionIDs
func (r *IBeamParameterRegistry) GetParameterCurrentValue(parameterID, deviceID uint32, dimensionID ...uint32) (*pb.ParameterValue, error) {
	r.muValue.RLock()
	defer r.muValue.RUnlock()
	state := r.parameterValue

	// first check param and deviceID
	if _, exists := state[deviceID][parameterID]; !exists {
		return nil, fmt.Errorf("getcurrentparametervalue: invalid ID for: DeviceID %d, ParameterID %d", deviceID, parameterID)
	}

	// Check if Dimension is Valid
	if !state[deviceID][parameterID].multiIndexHasValue(dimensionID) {
		return nil, fmt.Errorf("getcurrentparametervalue: invalid dimension id  %v for parameter %d and device %d", dimensionID, parameterID, deviceID)
	}

	parameterDimension, err := state[deviceID][parameterID].multiIndex(dimensionID)
	if err != nil {
		return nil, err
	}

	parameterBuffer, err := parameterDimension.getValue()
	if err != nil {
		return nil, err
	}

	valueCopy := proto.Clone(parameterBuffer.getCurrentParameterValue()).(*pb.ParameterValue)
	return valueCopy, nil
}

// GetParameterCurrentValue gets a copy of the current parameter value from the state by pid, did and dimensionIDs
func (r *IBeamParameterRegistry) GetParameterOptions(parameterID, deviceID uint32, dimensionID ...uint32) (*pb.OptionList, error) {
	model := r.GetModelIDByDeviceID(deviceID)

	r.muDetail.RLock()
	// first check param and model
	if _, exists := r.parameterDetail[model][parameterID]; !exists {
		r.muDetail.RUnlock()
		return nil, fmt.Errorf("getoptions: invalid ID for: DeviceID %d, ParameterID %d", deviceID, parameterID)
	}

	options := proto.Clone(r.parameterDetail[model][parameterID].OptionList).(*pb.OptionList)

	if !r.parameterDetail[model][parameterID].OptionListIsDynamic {
		r.muDetail.RUnlock()
		return options, nil
	}
	r.muDetail.RUnlock()

	r.muValue.RLock()
	defer r.muValue.RUnlock()
	state := r.parameterValue

	// first check param and deviceID
	if _, exists := state[deviceID][parameterID]; !exists {
		return nil, fmt.Errorf("getoptions: invalid ID for: DeviceID %d, ParameterID %d", deviceID, parameterID)
	}

	// Check if Dimension is Valid
	if !state[deviceID][parameterID].multiIndexHasValue(dimensionID) {
		return nil, fmt.Errorf("getoptions: invalid dimension id  %v for parameter %d and device %d", dimensionID, parameterID, deviceID)
	}

	parameterDimension, err := state[deviceID][parameterID].multiIndex(dimensionID)
	if err != nil {
		return nil, err
	}

	parameterBuffer, err := parameterDimension.getValue()
	if err != nil {
		return nil, err
	}

	if parameterBuffer.dynamicOptions != nil {
		options := proto.Clone(parameterBuffer.dynamicOptions).(*pb.OptionList)
		return options, nil
	}

	return options, nil
}

// GetParameterCurrentValue gets a copy of the current parameter value from the state by pid, did and dimensionIDs
func (r *IBeamParameterRegistry) GetParameterMinMax(parameterID, deviceID uint32, dimensionID ...uint32) (minimum float64, maximum float64, err error) {
	model := r.GetModelIDByDeviceID(deviceID)

	r.muDetail.RLock()
	// first check param and model
	if _, exists := r.parameterDetail[model][parameterID]; !exists {
		r.muDetail.RUnlock()
		return 0, 0, fmt.Errorf("getminmax: invalid ID for: DeviceID %d, ParameterID %d", deviceID, parameterID)
	}

	if !r.parameterDetail[model][parameterID].MinMaxIsDynamic {
		r.muDetail.RUnlock()
		return r.parameterDetail[model][parameterID].Minimum, r.parameterDetail[model][parameterID].Maximum, nil
	}
	r.muDetail.RUnlock()

	r.muValue.RLock()
	defer r.muValue.RUnlock()
	state := r.parameterValue

	// first check param and deviceID
	if _, exists := state[deviceID][parameterID]; !exists {
		return 0, 0, fmt.Errorf("getminmax: invalid ID for: DeviceID %d, ParameterID %d", deviceID, parameterID)
	}

	// Check if Dimension is Valid
	if !state[deviceID][parameterID].multiIndexHasValue(dimensionID) {
		return 0, 0, fmt.Errorf("getminmax: invalid dimension id  %v for parameter %d and device %d", dimensionID, parameterID, deviceID)
	}

	parameterDimension, err := state[deviceID][parameterID].multiIndex(dimensionID)
	if err != nil {
		return 0, 0, err
	}

	parameterBuffer, err := parameterDimension.getValue()
	if err != nil {
		return 0, 0, err
	}
	min := r.parameterDetail[model][parameterID].Minimum
	max := r.parameterDetail[model][parameterID].Maximum

	if parameterBuffer.dynamicMin != nil {
		min = *parameterBuffer.dynamicMin
	}

	if parameterBuffer.dynamicMax != nil {
		max = *parameterBuffer.dynamicMax
	}

	return min, max, nil
}

// GetModelIDByDeviceID is a helper to get the modelid for a specific device
func (r *IBeamParameterRegistry) GetModelIDByDeviceID(deviceID uint32) uint32 {
	r.muInfo.RLock()
	defer r.muInfo.RUnlock()

	device, exists := r.deviceInfos[deviceID]
	if !exists {
		r.log.Warnf("can not get model: no device with ID %d found", deviceID)
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
	r.log.Warnf("Could not find model for '%s', using generic model", modelName)
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
		r.log.Fatalf("Could not register device for nonexistent model with id: %v", modelID)
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
	if !r.parametersDone.Load() {
		if !r.connectionExists {
			// Autoregister connection parameter
			r.RegisterParameter(&pb.ParameterDetail{
				Id:            &pb.ModelParameterID{Parameter: 1},
				Path:          "config",
				Name:          "connection",
				Label:         "Connected",
				ShortLabel:    "Connected",
				Description:   "Connection status of device",
				GenericType:   pb.GenericType_ConnectionState,
				ControlStyle:  pb.ControlStyle_NoControl,
				FeedbackStyle: pb.FeedbackStyle_NormalFeedback,
				ValueType:     pb.ValueType_Binary,
				DefaultValue:  b.Bool(false),
			}, WithDefaultValid())
		}
		if cachedIDMap == nil {
			r.cacheIDMaps() // make sure cachedIDMap is initialized for all further usage
		}
		r.log.Debugf("Registered %d Parameters with %d Dimensional Values", r.parameterCount, r.dimensionCount)
	}
	r.parametersDone.Store(true)
	r.validateAllParams()

	r.muValue.Lock()
	defer r.muValue.Unlock() // Locking here to ensure the rest of corelib does not send out weird incomplete info meanwhile...

	r.muDetail.RLock()
	if _, exists := r.parameterDetail[modelID]; !exists {
		r.muDetail.RUnlock()
		return 0, fmt.Errorf("could not register device for nonexistent model with id: %v", modelID)
	}
	r.muDetail.RUnlock()

	r.muInfo.RLock()
	if _, exists := r.deviceInfos[deviceID]; exists {
		r.muInfo.RUnlock()
		return 0, fmt.Errorf("could not register device with existing deviceid: %v", deviceID)
	}
	r.muInfo.RUnlock()

	r.muDetail.RLock()
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

		for _, dvp := range r.defaultValidParams {
			if dvp.Parameter == parameterID && (dvp.Model == 0 || dvp.Model == modelID) {
				initialValue.Invalid = false
				break
			}
		}

		for _, dup := range r.defaultUnavailableParams {
			if dup.Parameter == parameterID && (dup.Model == 0 || dup.Model == modelID) {
				initialValue.Available = false
				break
			}
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

		if parameterDetail.DefaultValue != nil {
			initialValue.Value = parameterDetail.DefaultValue.Value
		}

		if len(parameterDetail.Dimensions) > 3 {
			r.log.Fatalf("It is not recommended to use more than 3 dimensions, if needed please contact the maintainer")
		}

		initialValueDimension := iBeamParameterDimension{
			value: &ibeamParameterValueBuffer{
				dimensionID:  make([]uint32, 0),
				available:    initialValue.Available,
				lastUpdate:   time.Now().Add(-time.Hour), // This is to ensure all delays do nothing weird on init
				currentValue: proto.Clone(initialValue).(*pb.ParameterValue),
				targetValue:  proto.Clone(initialValue).(*pb.ParameterValue),
			},
		}
		initialValueDimension.value.isAssumedState.Store(false)

		// Retrieve Flags

		var flags []ParamBufferConfigFlag

		if modelFlags, exists := r.parameterFlags[modelID]; exists {
			if modelParamFlags, exists := modelFlags[parameterID]; exists {
				flags = modelParamFlags
			}
		}

		if modelID != 0 {
			// Append flags from generic model
			if modelFlags, exists := r.parameterFlags[0]; exists {
				if modelParamFlags, exists := modelFlags[parameterID]; exists {
					if len(flags) == 0 {
						flags = modelParamFlags
					} else {
						// merge
					outerLoop:
						for _, f := range modelParamFlags {
							for _, existingF := range flags {
								if f == existingF {
									break outerLoop
								}
							}
							flags = append(flags, f)
						}
					}
				}
			}
		}

		parameterDimensions[parameterID] = generateDimensions(parameterDetail.Dimensions, &initialValueDimension, flags)
	}
	r.muDetail.RUnlock()

	r.muInfo.Lock()
	if deviceID == 0 {
		deviceID = uint32(len(r.deviceInfos) + 1)
		r.log.Warnf("Automatically assigning DeviceID %d to device with model %d", deviceID, modelID)
	}
	r.deviceInfos[deviceID] = &pb.DeviceInfo{
		DeviceID: deviceID,
		ModelID:  modelID,
	}
	r.muInfo.Unlock()

	//r.muValue.Lock() // Earlier we only locked the value around here... but now we need to use this lock to make sure the manager does not send out nonsense meanwhile
	r.parameterValue[deviceID] = parameterDimensions
	//r.muValue.Unlock()

	r.log.Debugf("Device '%v' registered with model: %v (%v)", deviceID, modelID, r.modelInfos[modelID].Name)
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
	return r.PNameByModel(parameterID, 0)
}

// PName Get a parameter Name by ID, returns "" if not found, always uses model 0
func (r *IBeamParameterRegistry) PNameByModel(parameterID, modelID uint32) string {
	// check for device registered
	if !r.parametersDone.Load() {
		r.log.Error("PNameByModel: only call after registering the first device")
		return ""
	}
	if cachedIDMap == nil {
		r.log.Error("ID Map not initialized yet, register a device first!")
	}
	cachedIDMapMu.RLock()
	defer cachedIDMapMu.RUnlock()

	model, exists := cachedIDMap[modelID]
	if !exists {
		r.log.Error("PNameByModel: model not found")
		return ""
	}

	name, exists := model[parameterID]
	if exists {
		return name
	}

	r.log.Debug("PNameByModel: could not find ", parameterID)
	return ""
}

// PID Get a parameterID by name, returns 0 if not found, always uses model 0
func (r *IBeamParameterRegistry) PID(parameterName string) uint32 {
	return r.PIDByModel(parameterName, 0)
}

func (r *IBeamParameterRegistry) PIDByModel(parameterName string, modelID uint32) uint32 {
	// check for device registered
	if !r.parametersDone.Load() {
		r.log.Error("PIDByModel: only call after registering the first device")
		return 0
	}

	if cachedIDMap == nil {
		r.log.Error("ID Map not initialized yet, register a device first!")
	}
	cachedNameMapMu.RLock()
	defer cachedNameMapMu.RUnlock()

	model, exists := cachedNameMap[modelID]
	if !exists {
		r.log.Error("PIDByModel: model not found")
		return 0
	}

	id, exists := model[parameterName]
	if exists {
		return id
	}
	r.log.Debug("PID: could not find ", parameterName)
	return 0
}

// GetNameMap returns a map of all parameter names, usefull for initial state requests
func (r *IBeamParameterRegistry) GetNameMap() map[uint32]string {
	return r.GetNameMapByModel(0)
}

func (r *IBeamParameterRegistry) GetNameMapByModel(modelID uint32) map[uint32]string {
	// check for device registered
	if !r.parametersDone.Load() {
		r.log.Error("GetNameMap: only call after registering the first device")
		return nil
	}

	if cachedIDMap == nil {
		r.log.Error("ID Map not initialized yet, register a device first!")
	}
	cachedIDMapMu.RLock()
	defer cachedIDMapMu.RUnlock()

	model, exists := cachedIDMap[modelID]
	if !exists {
		r.log.Error("GetNameMap: model not found")
		return nil
	}

	nameMap := make(map[uint32]string)
	for key, value := range model {
		nameMap[key] = value
	}
	return nameMap
}

// parameterIDByName get a parameterID by name, returns 0 if not found, not allowed to be public because it needs the mutexlock
func (r *IBeamParameterRegistry) parameterIDByName(parameterName string, modelID uint32) uint32 {
	// Function requires mutex to be fully locked before invocation
	if _, ok := r.modelInfos[modelID]; !ok {
		r.log.Fatalln("Model", modelID, "doesn't exist")
	}

	for id, param := range r.parameterDetail[modelID] {
		if param.Name == parameterName {
			return id
		}
	}
	return 0
}

// Functions for global rate limiter

func (r *IBeamParameterRegistry) getLastEvent(deviceID uint32) time.Time {
	if !r.deviceLastEvent.Has(deviceID) {
		r.deviceLastEvent.Set(deviceID, time.Now().Add(-time.Hour))
	}
	return r.deviceLastEvent.Get(deviceID)
}

func (r *IBeamParameterRegistry) setLastEvent(deviceID uint32) {
	r.deviceLastEvent.Set(deviceID, time.Now())
}
