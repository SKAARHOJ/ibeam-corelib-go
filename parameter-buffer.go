package ibeamcorelib

import (
	"reflect"
	"sync"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
)

// ibeamParameterValueBuffer is used for updating a ParameterValue.
// It holds a current and a target Value.
type ibeamParameterValueBuffer struct {
	dimensionID         []uint32
	available           bool
	isAssumedState      bool
	lastUpdate          time.Time
	tryCount            uint32
	reEvaluationTimer   *timeTimer
	reEvaluationTimerMu sync.Mutex
	currentValue        *pb.ParameterValue
	targetValue         *pb.ParameterValue

	dynamicOptions *pb.OptionList
	dynamicMin     *float64
	dynamicMax     *float64
}
type timeTimer struct {
	timer *time.Timer
	end   time.Time
}

func (b *ibeamParameterValueBuffer) getParameterValue() *pb.ParameterValue {
	b.isAssumedState = !b.currentEquals(b.targetValue)
	return &pb.ParameterValue{
		DimensionID:    b.dimensionID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState,
		Value:          b.targetValue.Value,
		Invalid:        b.targetValue.Invalid,
		MetaValues:     b.targetValue.MetaValues,
	}
}

func (b *ibeamParameterValueBuffer) incrementParameterValue() *pb.ParameterValue {
	return &pb.ParameterValue{
		DimensionID:    b.dimensionID,
		Available:      b.available,
		IsAssumedState: b.isAssumedState,
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
		IsAssumedState: b.isAssumedState,
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

	if b.currentValue.Available != new.Available {
		return false
	}

	if reflect.TypeOf(b.currentValue.Value) != reflect.TypeOf(new.Value) {
		return false
	}

	// Next we need to compare the actual values
	switch cv := b.currentValue.Value.(type) {
	case *pb.ParameterValue_Integer:
		if cv.Integer != new.Value.(*pb.ParameterValue_Integer).Integer {
			return false
		}
	case *pb.ParameterValue_IncDecSteps:
		// very unlikely on ingest current...
		return false
	case *pb.ParameterValue_Floating:
		if cv.Floating != new.Value.(*pb.ParameterValue_Floating).Floating {
			return false
		}
	case *pb.ParameterValue_Str:
		if cv.Str != new.Value.(*pb.ParameterValue_Str).Str {
			return false
		}
	case *pb.ParameterValue_CurrentOption:
		if cv.CurrentOption != new.Value.(*pb.ParameterValue_CurrentOption).CurrentOption {
			return false
		}
	case *pb.ParameterValue_Cmd:
		// very unlikely on ingest current...
		return false
	case *pb.ParameterValue_Binary:
		if cv.Binary != new.Value.(*pb.ParameterValue_Binary).Binary {
			return false
		}
	case *pb.ParameterValue_OptionListUpdate:
		// this would have returned earlier already
		return false
	case *pb.ParameterValue_MinimumUpdate:
		// this would have returned earlier already
		return false
	case *pb.ParameterValue_MaximumUpdate:
		// this would have returned earlier already
		return false
	case *pb.ParameterValue_Png:
		if !bytesEqual(cv.Png, new.Value.(*pb.ParameterValue_Png).Png) {
			return false
		}
	case *pb.ParameterValue_Jpeg:
		if !bytesEqual(cv.Jpeg, new.Value.(*pb.ParameterValue_Jpeg).Jpeg) {
			return false
		}
	}

	return true
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
