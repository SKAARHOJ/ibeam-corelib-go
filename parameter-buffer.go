package ibeamcorelib

import (
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	"google.golang.org/protobuf/proto"
)

// ibeamParameterValueBuffer is used for updating a ParameterValue.
// It holds a current and a target Value.
type ibeamParameterValueBuffer struct {
	dimensionID    []uint32
	available      bool
	isAssumedState bool
	lastUpdate     time.Time
	tryCount       uint32
	currentValue   *pb.ParameterValue
	targetValue    *pb.ParameterValue
	metaValues     []pb.ParameterMetaValue
}

func (b *ibeamParameterValueBuffer) getParameterValue() *pb.ParameterValue {
	b.isAssumedState = !proto.Equal(b.targetValue, b.currentValue)
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
