package ibeam_corelib

import (
	"errors"
	"log"
)

type IbeamParameterDimension struct {
	subDimensions map[int]*IbeamParameterDimension
	value         *IBeamParameterValueBuffer
}

func (pd *IbeamParameterDimension) isValue() bool {
	if pd.value == nil {
		return false
	}
	return true
}

// Value of the Dimension
func (pd *IbeamParameterDimension) Value() (*IBeamParameterValueBuffer, error) {
	if pd.isValue() {
		return pd.value, nil
	}
	return nil, errors.New("Dimension is not a value")
}

// Subdimensions of the Dimension
func (pd *IbeamParameterDimension) Subdimensions() (map[int]*IbeamParameterDimension, error) {
	if !pd.isValue() {
		return pd.subDimensions, nil
	}
	return nil, errors.New("Dimension has no subdimension")
}

func (pd *IbeamParameterDimension) multiIndex(dimensionID []uint32) *IbeamParameterDimension {
	valuePointer := pd
	for i, id := range dimensionID {
		if i == len(dimensionID)-1 {
			return valuePointer.index(id - 1)
		}
		valuePointer = valuePointer.index(id - 1)
	}
	log.Panic("DimensionID too short")
	return nil
}

func (pd *IbeamParameterDimension) index(index uint32) *IbeamParameterDimension {
	if pd.isValue() {
		log.Panic("Called Index on Value")
	}
	if len(pd.subDimensions) <= int(index) {
		log.Panicf("Parameter Dimension Index out of range: wanted: %v length: %v", index, len(pd.subDimensions))
	}
	return pd.subDimensions[int(index)]

}
