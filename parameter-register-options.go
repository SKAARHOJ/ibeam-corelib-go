package ibeamcorelib

import (
	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
)

type RegisterOption func(r *IBeamParameterRegistry, id *pb.ModelParameterID)

// WithDefaultValid registers the parameter withouth setting the invalid flag
func WithDefaultValid() func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
	return func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
		r.defaultValidParams = append(r.defaultValidParams, id)
	}
}

// WithDefaultUnavailable registers the parameter as unavailable at startup
func WithDefaultUnavailable() func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
	return func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
		r.defaultUnavailableParams = append(r.defaultUnavailableParams, id)
	}
}

// WithIncrementPassthrough disables the manager for an ControlStyle_Incremental parameter
func WithIncrementPassthrough() func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
	return setFlag(FlagIncrementalPassthrough)
}

// WithValuePassthrough disables the manager for Normal Int, Float and Binary Parameters
func WithValuePassthrough() func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
	return setFlag(FlagValuePassthrough)
}

// WithModelRatelimitExlude makes this parameter excluded from the global per-model rate limit
func WithModelRatelimitExlude() func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
	return setFlag(FlagRateLimitExclude)
}

// WithValueSmoothing enables value smoothing for Float and Integer parameters.
// When a new target is set, intermediate values are sent at ControlDelayMs intervals,
// each changing by at most maxStepSize toward the target.
func WithValueSmoothing(maxStepSize float64) func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
	return func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
		setFlag(FlagValueSmoothing)(r, id)

		if r.parameterSmoothingMaxStep == nil {
			r.parameterSmoothingMaxStep = make(map[uint32]map[uint32]float64)
		}
		if _, exists := r.parameterSmoothingMaxStep[id.Model]; !exists {
			r.parameterSmoothingMaxStep[id.Model] = make(map[uint32]float64)
		}
		r.parameterSmoothingMaxStep[id.Model][id.Parameter] = maxStepSize
	}
}

func setFlag(flag ParamBufferConfigFlag) func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
	return func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {

		if r.parameterFlags == nil {
			r.parameterFlags = make(map[uint32]map[uint32][]ParamBufferConfigFlag)
		}

		if _, exists := r.parameterFlags[id.Model]; !exists {
			r.parameterFlags[id.Model] = make(map[uint32][]ParamBufferConfigFlag)
		}

		if _, exists := r.parameterFlags[id.Model][id.Parameter]; !exists {
			r.parameterFlags[id.Model][id.Parameter] = make([]ParamBufferConfigFlag, 0)
		}

		r.parameterFlags[id.Model][id.Parameter] = append(r.parameterFlags[id.Model][id.Parameter], flag)
	}
}
