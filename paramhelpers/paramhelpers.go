package paramhelpers

import pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"

// Param is the main function of the parameter helpers suite. It is used to create a pb.Parameter with a specified id and device.
// After that you can add as many parameter values as needed for the specific parameter (usually onbly one, more for parameters with dimensions that you want to update at the same time)
func Param(pid uint32, did uint32, vals ...*pb.ParameterValue) *pb.Parameter {
	return &pb.Parameter{
		Id:    &pb.DeviceParameterID{Device: uint32(did), Parameter: pid},
		Value: vals,
	}
}

// ################### Values ###################

// Int just returns a parameter value of type ParameterValue_Integer
func Int(val int, dimensionID ...uint32) *pb.ParameterValue {
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_Integer{Integer: int32(val)}}
}

// Float just returns a parameter value of type ParameterValue_Floating
func Float(val float64, dimensionID ...uint32) *pb.ParameterValue {
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_Floating{Floating: val}}
}

// Bool just returns a parameter value of type ParameterValue_Binary
func Bool(val bool, dimensionID ...uint32) *pb.ParameterValue {
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_Binary{Binary: val}}
}

// String just returns a parameter value of type ParameterValue_Str
func String(val string, dimensionID ...uint32) *pb.ParameterValue {
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_Str{Str: val}}
}

// OptIndex just returns a parameter value of type ParameterValue_CurrentOption
func OptIndex(val int, dimensionID ...uint32) *pb.ParameterValue {
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_CurrentOption{CurrentOption: uint32(val)}}
}

// Png returns a parameter value of type ParameterValue_Png
func Png(val []byte, dimensionID ...uint32) *pb.ParameterValue {
	// TODO: pass image.Image and do some validation ? or just decide to not care ?
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_Png{Png: val}}
}

// Jpeg returns a parameter value of type ParameterValue_Jpeg
func Jpeg(val []byte, dimensionID ...uint32) *pb.ParameterValue {
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_Jpeg{Jpeg: val}}
}

// Detail Updates

// OptList just returns a parameter value of type ParameterValue_OptionList, used to update a dynamic option list
func NewOptList(val *pb.OptionList, dimensionID ...uint32) *pb.ParameterValue {
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_OptionListUpdate{OptionListUpdate: val}}
}

// NewMax just returns a parameter value of type ParameterValue_MaximumUpdate, used to update a parameters maximum value
func NewMax(val float64, dimensionID ...uint32) *pb.ParameterValue {
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_MaximumUpdate{MaximumUpdate: val}}
}

// NewMin just returns a parameter value of type ParameterValue_MinimumUpdate, used to update a parameters minimum value
func NewMin(val float64, dimensionID ...uint32) *pb.ParameterValue {
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_MinimumUpdate{MinimumUpdate: val}}
}

// Available and Invalid

// Avail is used to set a specific dimension values available flag
func Avail(available bool, dimensionID ...uint32) *pb.ParameterValue {
	return &pb.ParameterValue{DimensionID: dimensionID, Available: available}
}

// Invalid is used to set a specific values invalid flag
func Invalid(dimensionID ...uint32) *pb.ParameterValue {
	// This is only evaluated when set to true, to clear this send a valid value
	return &pb.ParameterValue{DimensionID: dimensionID, Invalid: true}
}
