package ibeam_corelib

import (
	"errors"
	"fmt"
)

type IbeamParameterDimension struct {
	subDimensions []*IbeamParameterDimension
	value         *IBeamParameterValueBuffer
}

func (pd *IbeamParameterDimension) isValue() bool {
	if pd.value == nil {
		return false
	}
	return true
}

// All of these functions can return null if they hit an error, be sure to check, maybe should do different style of error handling at some point...

// Value of the Dimension
func (pd *IbeamParameterDimension) Value() (*IBeamParameterValueBuffer, error) {
	if pd.isValue() {
		return pd.value, nil
	}
	return nil, errors.New("Dimension is not a value")
}

// Subdimensions of the Dimension
func (pd *IbeamParameterDimension) Subdimensions() ([]*IbeamParameterDimension, error) {
	if !pd.isValue() {
		return pd.subDimensions, nil
	}
	return nil, errors.New("Dimension has no subdimension")
}

func (pd *IbeamParameterDimension) MultiIndexHasValue(dimensionID []uint32) bool {
	valuePointer := pd

	if len(dimensionID) == 0 {
		return valuePointer.isValue()
	}

	for i, id := range dimensionID {
		if i == len(dimensionID)-1 {
			dimension, err := valuePointer.index(id - 1)
			if err != nil || dimension == nil {
				return false
			}
			return true
		}
		valuePointer, err := valuePointer.index(id - 1)
		if valuePointer == nil || err != nil {
			return false
		}
	}
	return false
}

func (pd *IbeamParameterDimension) MultiIndex(dimensionID []uint32) (*IbeamParameterDimension, error) {
	valuePointer := pd
	if len(dimensionID) == 0 {
		return valuePointer, nil
	}

	for i, id := range dimensionID {
		if i == len(dimensionID)-1 {
			return valuePointer.index(id - 1)
		}
		valuePointer, err := valuePointer.index(id - 1)
		if err != nil {
			return nil, err
		}
		if valuePointer == nil {
			return nil, fmt.Errorf("DimensionID too long")
		}
	}
	return nil, fmt.Errorf("DimensionID too short")
}

func (pd *IbeamParameterDimension) index(index uint32) (*IbeamParameterDimension, error) {
	if len(pd.subDimensions) <= int(index) {
		return nil, fmt.Errorf("Parameter Dimension Index out of range: wanted: %v length: %v", index, len(pd.subDimensions))
	}
	return pd.subDimensions[int(index)], nil
}
