package ibeamcorelib

import (
	"errors"
	"fmt"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	"google.golang.org/protobuf/proto"
)

// iBeamParameterDimension is a recursive structure to enable the state management of parameters with several dimensions
type iBeamParameterDimension struct {
	subDimensions []*iBeamParameterDimension
	value         *ibeamParameterValueBuffer
}

func (pd *iBeamParameterDimension) isValue() bool {
	return pd.value != nil
}

// getValue of the Dimension
func (pd *iBeamParameterDimension) getValue() (*ibeamParameterValueBuffer, error) {
	if pd.isValue() {
		return pd.value, nil
	}
	return nil, errors.New("dimension is not a value")
}

// getSubdimensions of the Dimension
func (pd *iBeamParameterDimension) getSubdimensions() ([]*iBeamParameterDimension, error) {
	if !pd.isValue() {
		return pd.subDimensions, nil
	}
	return nil, errors.New("dimension has no subdimension")
}

// multiIndexHasValue checks for a specific dimension ids existence
func (pd *iBeamParameterDimension) multiIndexHasValue(dimensionID []uint32) bool {
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
			return dimension.isValue()
		}
		var err error
		valuePointer, err = valuePointer.index(id - 1)
		if valuePointer == nil || err != nil {
			return false
		}
	}
	return false
}

// multiIndex gets a specific dimension by dimensionID
func (pd *iBeamParameterDimension) multiIndex(dimensionID []uint32) (*iBeamParameterDimension, error) {
	valuePointer := pd
	if len(dimensionID) == 0 {
		return valuePointer, nil
	}

	for i, id := range dimensionID {
		if i == len(dimensionID)-1 {
			return valuePointer.index(id - 1)
		}
		var err error
		valuePointer, err = valuePointer.index(id - 1)
		if err != nil {
			return nil, fmt.Errorf("dimensionID too long %w", err)
		}
	}
	return nil, fmt.Errorf("dimensionID too short")
}

func (pd *iBeamParameterDimension) index(index uint32) (*iBeamParameterDimension, error) {
	if len(pd.subDimensions) <= int(index) {
		return nil, fmt.Errorf("parameter dimension index out of range: wanted: %v length: %v", index, len(pd.subDimensions))
	}
	return pd.subDimensions[int(index)], nil
}

func generateDimensions(dimensionConfig []uint32, initialValueDimension *iBeamParameterDimension) *iBeamParameterDimension {
	if len(dimensionConfig) == 0 {
		return initialValueDimension
	}

	dimensions := make([]*iBeamParameterDimension, 0)

	for count := 0; count < int(dimensionConfig[0]); count++ {
		valueWithID := iBeamParameterDimension{}
		dimValue := initialValueDimension.value

		valueWithID.value = &ibeamParameterValueBuffer{
			available:      dimValue.available,
			isAssumedState: dimValue.isAssumedState,
			currentValue:   proto.Clone(initialValueDimension.value.currentValue).(*pb.ParameterValue),
			targetValue:    proto.Clone(initialValueDimension.value.targetValue).(*pb.ParameterValue),
		}

		valueWithID.value.dimensionID = make([]uint32, len(dimValue.dimensionID), len(dimValue.dimensionID)+1)
		copy(valueWithID.value.dimensionID, dimValue.dimensionID)
		valueWithID.value.dimensionID = append(valueWithID.value.dimensionID, uint32(count+1))

		if len(dimensionConfig) == 1 {
			dimensions = append(dimensions, &valueWithID)
		} else {
			subDim := generateDimensions(dimensionConfig[1:], &valueWithID)
			dimensions = append(dimensions, subDim)
		}
	}
	return &iBeamParameterDimension{
		subDimensions: dimensions,
	}
}
