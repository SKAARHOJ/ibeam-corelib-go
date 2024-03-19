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

// WithIncrementPassthrough disables the manager for an ControlStyle_Incremental parameter
func WithIncrementPassthrough() func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
	return setFlag(FlagIncrementalPassthrough)
}

// WithValuePassthrough disables the manager for Normal Int and Binary Parameters
func WithValuePassthrough() func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
	return setFlag(FlagValuePassthrough)
}

// WithModelRatelimitExlude makes this parameter excluded from the global per-model rate limit
func WithModelRatelimitExlude() func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
	return setFlag(FlagRateLimitExclude)
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
