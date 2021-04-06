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
}
type timeTimer struct {
	timer *time.Timer
	end   time.Time
}

func (b *ibeamParameterValueBuffer) getParameterValue() *pb.ParameterValue {
	b.isAssumedState = !reflect.DeepEqual(b.targetValue.Value, b.currentValue.Value)
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
