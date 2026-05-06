package ibeamcorelib

import (
	"bytes"
	"slices"
	"sync"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	"go.uber.org/atomic"
)

type ParamBufferConfigFlag int

const (
	FlagIncrementalPassthrough ParamBufferConfigFlag = 1
	FlagRateLimitExclude       ParamBufferConfigFlag = 2
	FlagValuePassthrough       ParamBufferConfigFlag = 3
	FlagValueSmoothing         ParamBufferConfigFlag = 4
)

// ibeamParameterValueBuffer is used for updating a ParameterValue.
// It holds a current and a target Value.
type ibeamParameterValueBuffer struct {
	dimensionID         []uint32
	available           bool
	isAssumedState      atomic.Bool
	lastUpdate          time.Time
	tryCount            uint32
	reEvaluationTimer   *timeTimer
	reEvaluationTimerMu sync.Mutex
	currentValue        *pb.ParameterValue
	targetValue         *pb.ParameterValue

	dynamicOptions *pb.OptionList
	dynamicMin     *float64
	dynamicMax     *float64

	// Value smoothing
	smoothingMaxStep  float64            // max change per step; 0 = no smoothing
	smoothingLastSent *pb.ParameterValue // last intermediate value sent; nil = not actively smoothing

	// Additional flags
	flags []ParamBufferConfigFlag
}
type timeTimer struct {
	timer *time.Timer
	end   time.Time
}

func (b *ibeamParameterValueBuffer) hasFlag(flag ParamBufferConfigFlag) bool {
	return slices.Contains(b.flags, flag)
}

func (b *ibeamParameterValueBuffer) getParameterValue() *pb.ParameterValue {
	b.isAssumedState.Store(!b.currentEquals(b.targetValue))
	return &pb.ParameterValue{
		DimensionID:    b.dimensionID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState.Load(),
		Value:          b.targetValue.Value,
		Invalid:        b.targetValue.Invalid,
		MetaValues:     b.targetValue.MetaValues,
	}
}

func (b *ibeamParameterValueBuffer) getCurrentParameterValue() *pb.ParameterValue {
	b.isAssumedState.Store(!b.currentEquals(b.targetValue))
	return &pb.ParameterValue{
		DimensionID:    b.dimensionID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState.Load(),
		Value:          b.currentValue.Value,
		Invalid:        b.currentValue.Invalid,
		MetaValues:     b.currentValue.MetaValues,
	}
}

func (b *ibeamParameterValueBuffer) incrementParameterValue() *pb.ParameterValue {
	return &pb.ParameterValue{
		DimensionID:    b.dimensionID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState.Load(),
		Value: &pb.ParameterValue_IncDecSteps{
			IncDecSteps: 1,
		},
		MetaValues: b.currentValue.MetaValues,
	}
}

func (b *ibeamParameterValueBuffer) decrementParameterValue() *pb.ParameterValue {
	return &pb.ParameterValue{
		DimensionID:    b.dimensionID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState.Load(),
		Value: &pb.ParameterValue_IncDecSteps{
			IncDecSteps: -1,
		},
		MetaValues: b.currentValue.MetaValues,
	}
}

// This function provides an optimized way of checking against current value without proto.Equals (which is an expensive function)
func (b *ibeamParameterValueBuffer) currentEquals(new *pb.ParameterValue) bool {

	if b.currentValue.Invalid != new.Invalid {
		return false
	}

	// This check is not really needed here, we check it everywhere else manually... it only created the issue on the very first ingest when the param was sent back straight
	// if b.currentValue.Available != new.Available {
	// 	 return false
	// }

	switch cv := b.currentValue.Value.(type) {
	case *pb.ParameterValue_Integer:
		nv, ok := new.Value.(*pb.ParameterValue_Integer)
		if !ok || cv.Integer != nv.Integer {
			return false
		}
	case *pb.ParameterValue_IncDecSteps:
		return false
	case *pb.ParameterValue_Floating:
		nv, ok := new.Value.(*pb.ParameterValue_Floating)
		if !ok || cv.Floating != nv.Floating {
			return false
		}
	case *pb.ParameterValue_Str:
		nv, ok := new.Value.(*pb.ParameterValue_Str)
		if !ok || cv.Str != nv.Str {
			return false
		}
	case *pb.ParameterValue_CurrentOption:
		nv, ok := new.Value.(*pb.ParameterValue_CurrentOption)
		if !ok || cv.CurrentOption != nv.CurrentOption {
			return false
		}
	case *pb.ParameterValue_Cmd:
		return false
	case *pb.ParameterValue_Binary:
		nv, ok := new.Value.(*pb.ParameterValue_Binary)
		if !ok || cv.Binary != nv.Binary {
			return false
		}
	case *pb.ParameterValue_OptionListUpdate:
		return false
	case *pb.ParameterValue_MinimumUpdate:
		return false
	case *pb.ParameterValue_MaximumUpdate:
		return false
	case *pb.ParameterValue_Png:
		nv, ok := new.Value.(*pb.ParameterValue_Png)
		if !ok || !bytes.Equal(cv.Png, nv.Png) {
			return false
		}
	case *pb.ParameterValue_Jpeg:
		nv, ok := new.Value.(*pb.ParameterValue_Jpeg)
		if !ok || !bytes.Equal(cv.Jpeg, nv.Jpeg) {
			return false
		}
	default:
		return false
	}

	return true
}
