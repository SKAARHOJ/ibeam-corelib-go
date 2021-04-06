package ibeamcorelib

import (
	"errors"
	"fmt"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	"google.golang.org/protobuf/proto"
)

// IBeamParameterDimension is a recursive structure to enable the state management of parameters with several dimensions
type IBeamParameterDimension struct {
	subDimensions []*IBeamParameterDimension
	value         *ibeamParameterValueBuffer
}

func (pd *IBeamParameterDimension) isValue() bool {
	return pd.value != nil
}

// Value of the Dimension
func (pd *IBeamParameterDimension) Value() (*ibeamParameterValueBuffer, error) {
	if pd.isValue() {
		return pd.value, nil
	}
	return nil, errors.New("dimension is not a value")
}

// Subdimensions of the Dimension
func (pd *IBeamParameterDimension) Subdimensions() ([]*IBeamParameterDimension, error) {
	if !pd.isValue() {
		return pd.subDimensions, nil
	}
	return nil, errors.New("dimension has no subdimension")
}

// MultiIndexHasValue checks for a specific dimension ids existence
func (pd *IBeamParameterDimension) MultiIndexHasValue(dimensionID []uint32) bool {
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

// MultiIndex gets a specific dimension by dimensionID
func (pd *IBeamParameterDimension) MultiIndex(dimensionID []uint32) (*IBeamParameterDimension, error) {
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

func (pd *IBeamParameterDimension) index(index uint32) (*IBeamParameterDimension, error) {
	if len(pd.subDimensions) <= int(index) {
		return nil, fmt.Errorf("parameter dimension index out of range: wanted: %v length: %v", index, len(pd.subDimensions))
	}
	return pd.subDimensions[int(index)], nil
}

func generateDimensions(dimensionConfig []uint32, initialValueDimension *IBeamParameterDimension) *IBeamParameterDimension {
	if len(dimensionConfig) == 0 {
		return initialValueDimension
	}

	dimensions := make([]*IBeamParameterDimension, 0)

	for count := 0; count < int(dimensionConfig[0]); count++ {
		valueWithID := IBeamParameterDimension{}
		dimValue := initialValueDimension.value

		valueWithID.value = &ibeamParameterValueBuffer{
			available:      dimValue.available,
			isAssumedState: dimValue.isAssumedState,
			currentValue:   proto.Clone(initialValueDimension.value.currentValue).(*pb.ParameterValue),
			targetValue:    proto.Clone(initialValueDimension.value.targetValue).(*pb.ParameterValue),
		}

		valueWithID.value.dimensionID = make([]uint32, len(dimValue.dimensionID))
		copy(valueWithID.value.dimensionID, dimValue.dimensionID)
		valueWithID.value.dimensionID = append(valueWithID.value.dimensionID, uint32(count+1))

		if len(dimensionConfig) == 1 {
			dimensions = append(dimensions, &valueWithID)
		} else {
			subDim := generateDimensions(dimensionConfig[1:], &valueWithID)
			dimensions = append(dimensions, subDim)
		}
	}
	return &IBeamParameterDimension{
		subDimensions: dimensions,
	}
}
