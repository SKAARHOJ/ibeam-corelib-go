package ibeam_corelib

import "errors"

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
