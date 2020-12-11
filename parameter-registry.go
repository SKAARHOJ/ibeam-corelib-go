package ibeam_corelib

import (
	"sync"

	ibeam_core "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
)

type parameterDetails []map[int]*ibeam_core.ParameterDetail //Parameter Details: model, parameter
type parameterStates []map[int][]*IbeamParameterDimension   //Parameter States: device,parameter,dimension

// IbeamParameterRegistry is the storage of the core.
// It saves all Infos about the Core, Device and Models and stores the Details and current Values of the Parameter.
type IbeamParameterRegistry struct {
	muInfo          sync.RWMutex
	muDetail        sync.RWMutex
	muValue         sync.RWMutex
	coreInfo        ibeam_core.CoreInfo
	DeviceInfos     []*ibeam_core.DeviceInfo
	ModelInfos      []*ibeam_core.ModelInfo
	ParameterDetail parameterDetails //Parameter Details: model, parameter
	parameterValue  parameterStates  //Parameter States: device,parameter,dimension
}

// device,parameter,instance
func (r *IbeamParameterRegistry) getInstanceValues(dpID ibeam_core.DeviceParameterID) (values []*ibeam_core.ParameterValue) {
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
	for _, value := range r.parameterValue[deviceIndex][parameterIndex] {
		values = append(values, value.value.getParameterValue())
	}

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
