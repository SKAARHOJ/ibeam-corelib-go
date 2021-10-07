package ibeamcorelib

import (
	"errors"
	"fmt"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	"google.golang.org/protobuf/proto"
)

// iBeamParameterDimension is a recursive structure to enable the state management of parameters with several dimensions
type iBeamParameterDimension struct {
	subDimensions map[uint32]*iBeamParameterDimension
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
func (pd *iBeamParameterDimension) getSubdimensions() (map[uint32]*iBeamParameterDimension, error) {
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
			dimension, err := valuePointer.index(id)
			if err != nil || dimension == nil {
				return false
			}
			return dimension.isValue()
		}
		var err error
		valuePointer, err = valuePointer.index(id)
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
			return valuePointer.index(id)
		}
		var err error
		valuePointer, err = valuePointer.index(id)
		if err != nil {
			return nil, fmt.Errorf("dimensionID too long %w", err)
		}
	}
	return nil, fmt.Errorf("dimensionID too short")
}

func (pd *iBeamParameterDimension) index(index uint32) (*iBeamParameterDimension, error) {
	if dim, exists := pd.subDimensions[index]; exists {
		return dim, nil
	}
	return nil, fmt.Errorf("parameter dimension index out of range: wanted: %v length: %v", index, len(pd.subDimensions))
}

func generateDimensions(dimensionConfig []*pb.DimensionDetail, initialValueDimension *iBeamParameterDimension) *iBeamParameterDimension {
	if len(dimensionConfig) == 0 || dimensionConfig[0] == nil {
		return initialValueDimension
	}

	dimensions := make(map[uint32]*iBeamParameterDimension, 0)

	dimElementIDs := make([]uint32, len(dimensionConfig[0].ElementLabels)+int(dimensionConfig[0].Count))

	for count := uint32(1); count <= dimensionConfig[0].Count; count++ {
		dimElementIDs = append(dimElementIDs, count)
	}

	for id := range dimensionConfig[0].ElementLabels {
		dimElementIDs = append(dimElementIDs, id)
	}

	for _, dimID := range dimElementIDs {
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
		valueWithID.value.dimensionID = append(valueWithID.value.dimensionID, dimID)

		if len(dimensionConfig) == 1 {
			dimensions[dimID] = &valueWithID
		} else {
			subDim := generateDimensions(dimensionConfig[1:], &valueWithID)
			dimensions[dimID] = subDim
		}
	}
	return &iBeamParameterDimension{
		subDimensions: dimensions,
	}
}
