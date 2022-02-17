package ibeamcorelib

import (
	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
)

type RegisterOption func(r *IBeamParameterRegistry, id *pb.ModelParameterID)

func WithDefaultValid() func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
	return func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
		r.defaultValidParams = append(r.defaultValidParams, id)
	}
}

func WithIncrementPassthrough() func(r *IBeamParameterRegistry, id *pb.ModelParameterID) {
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

		r.parameterFlags[id.Model][id.Parameter] = append(r.parameterFlags[id.Model][id.Parameter], FlagIncrementalPassthrough)
	}
}
