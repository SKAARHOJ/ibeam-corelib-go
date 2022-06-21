package paramhelpers

import (
	"fmt"
	"strconv"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
)

// Param is the main function of the parameter helpers suite. It is used to create a pb.Parameter with a specified id and device.
// After that you can add as many parameter values as needed for the specific parameter (usually onbly one, more for parameters with dimensions that you want to update at the same time)
func Param(pid uint32, did uint32, vals ...*pb.ParameterValue) *pb.Parameter {
	return &pb.Parameter{
		Id:    &pb.DeviceParameterID{Device: uint32(did), Parameter: pid},
		Value: vals,
	}
}

// Helpers for custom error messages

// Return a error for a param
func ParamError(pid uint32, did uint32, id, message string, dimensionID ...uint32) *pb.Parameter {
	return &pb.Parameter{
		Id:    &pb.DeviceParameterID{Device: did, Parameter: pid},
		Error: pb.ParameterError_Custom,
		Value: []*pb.ParameterValue{
			&pb.ParameterValue{
				Value: &pb.ParameterValue_Error{
					Error: &pb.CustomError{
						Message:   message,
						ID:        id,
						Errortype: pb.CustomErrorType_Error,
					},
				},
			},
		},
	}
}

// Return a error for a param
func ParamWarn(pid uint32, did uint32, id, message string, dimensionID ...uint32) *pb.Parameter {
	return &pb.Parameter{
		Id:    &pb.DeviceParameterID{Device: did, Parameter: pid},
		Error: pb.ParameterError_Custom,
		Value: []*pb.ParameterValue{
			&pb.ParameterValue{
				Value: &pb.ParameterValue_Error{
					Error: &pb.CustomError{
						Message:   message,
						Errortype: pb.CustomErrorType_Warning,
						ID:        id,
					},
				},
			},
		},
	}
}

func DeviceError(did uint32, id, message string, args ...interface{}) *pb.Parameter {
	return &pb.Parameter{
		Id:    &pb.DeviceParameterID{Device: did, Parameter: 0},
		Error: pb.ParameterError_Custom,
		Value: []*pb.ParameterValue{
			&pb.ParameterValue{
				Value: &pb.ParameterValue_Error{
					Error: &pb.CustomError{
						Message:   fmt.Sprintf(message, args...),
						ID:        id,
						Errortype: pb.CustomErrorType_Error,
					},
				},
			},
		},
	}
}

func DeviceWarn(did uint32, id, message string, args ...interface{}) *pb.Parameter {
	return &pb.Parameter{
		Id:    &pb.DeviceParameterID{Device: did, Parameter: 0},
		Error: pb.ParameterError_Custom,
		Value: []*pb.ParameterValue{
			&pb.ParameterValue{
				Value: &pb.ParameterValue_Error{
					Error: &pb.CustomError{
						Message:   fmt.Sprintf(message, args...),
						Errortype: pb.CustomErrorType_Warning,
						ID:        id,
					},
				},
			},
		},
	}
}

// Return a global error message
func Error(id, message string, args ...interface{}) *pb.Parameter {
	return &pb.Parameter{
		Error: pb.ParameterError_Custom,
		Id:    &pb.DeviceParameterID{Device: 0, Parameter: 0},
		Value: []*pb.ParameterValue{
			&pb.ParameterValue{
				Value: &pb.ParameterValue_Error{
					Error: &pb.CustomError{
						Message:   fmt.Sprintf(message, args...),
						ID:        id,
						Errortype: pb.CustomErrorType_Error,
					},
				},
			},
		},
	}
}

// Return a global error message
func Warn(id, message string, args ...interface{}) *pb.Parameter {
	return &pb.Parameter{
		Error: pb.ParameterError_Custom,
		Id:    &pb.DeviceParameterID{Device: 0, Parameter: 0},
		Value: []*pb.ParameterValue{
			&pb.ParameterValue{
				Value: &pb.ParameterValue_Error{
					Error: &pb.CustomError{
						Message:   fmt.Sprintf(message, args...),
						Errortype: pb.CustomErrorType_Warning,
						ID:        id,
					},
				},
			},
		},
	}
}

// Return a global error message
func ResolveError(id string) *pb.Parameter {
	return &pb.Parameter{
		Error: pb.ParameterError_Custom,
		Id:    &pb.DeviceParameterID{Device: 0, Parameter: 0},
		Value: []*pb.ParameterValue{
			&pb.ParameterValue{
				Value: &pb.ParameterValue_Error{
					Error: &pb.CustomError{
						Errortype: pb.CustomErrorType_Resolve,
						ID:        id,
					},
				},
			},
		},
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

// Float just returns a parameter value of type ParameterValue_Floating that is correctly converted, this is an experimental helper, its implementation might change in the future
func Float32(val float32, dimensionID ...uint32) *pb.ParameterValue {
	val64, _ := strconv.ParseFloat(fmt.Sprint(val), 64)
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_Floating{Floating: val64}}
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

// IncDecSteps just returns a parameter value of type ParameterValue_IncDecSteps
func IncDecSteps(val int, dimensionID ...uint32) *pb.ParameterValue {
	return &pb.ParameterValue{DimensionID: dimensionID, Value: &pb.ParameterValue_IncDecSteps{IncDecSteps: int32(val)}}
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
