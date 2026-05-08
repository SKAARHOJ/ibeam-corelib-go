package ibeamcorelib

import (
	"fmt"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	b "github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers"
	"google.golang.org/protobuf/proto"
)

func computeSmoothingStep(from, to *pb.ParameterValue, maxStep float64, vt pb.ValueType) *pb.ParameterValue {
	result := proto.Clone(to).(*pb.ParameterValue)
	switch vt {
	case pb.ValueType_Floating:
		cur, tgt := from.GetFloating(), to.GetFloating()
		diff := tgt - cur
		if diff > maxStep {
			diff = maxStep
		} else if diff < -maxStep {
			diff = -maxStep
		}
		result.Value = &pb.ParameterValue_Floating{Floating: cur + diff}
	case pb.ValueType_Integer:
		cur, tgt := int64(from.GetInteger()), int64(to.GetInteger())
		diff := tgt - cur
		ms := max(int64(maxStep), 1)
		if diff > ms {
			diff = ms
		} else if diff < -ms {
			diff = -ms
		}
		result.Value = &pb.ParameterValue_Integer{Integer: int32(cur + diff)}
	}
	return result
}

func smoothingReachedTarget(step, target *pb.ParameterValue, vt pb.ValueType) bool {
	switch vt {
	case pb.ValueType_Floating:
		return step.GetFloating() == target.GetFloating()
	case pb.ValueType_Integer:
		return step.GetInteger() == target.GetInteger()
	}
	return true
}

func (m *IBeamParameterManager) processParameter(address paramDimensionAddress) {
	mlog := m.log

	deviceID := address.device
	paramID := address.parameter

	ds := m.parameterRegistry.loadDeviceState(deviceID)
	if ds == nil {
		mlog.Errorf("processParameter: unknown device %d", deviceID)
		return
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()

	m.parameterRegistry.muInfo.RLock()
	defer m.parameterRegistry.muInfo.RUnlock()

	m.parameterRegistry.muDetail.RLock()
	defer m.parameterRegistry.muDetail.RUnlock()

	modelID := m.parameterRegistry.getModelID(deviceID)
	rootDimension := ds.params[address.parameter]
	parameterDetail := m.parameterRegistry.parameterDetail[modelID][paramID]

	if !rootDimension.multiIndexHasValue(address.dimensionID) {
		mlog.Errorf("Invalid dimension ID %v for %d", address.dimensionID, address)
		return
	}
	parameterDimension, err := rootDimension.multiIndex(address.dimensionID)
	if err != nil {
		mlog.Errorf("could not get parameter buffer for dimension %v of param %v: %v", address.dimensionID, address, err)
		return
	}
	parameterBuffer, err := parameterDimension.getValue()
	if err != nil {
		mlog.Errorf("could not get parameter buffer value for dimension %v of param %v: %v", address.dimensionID, address, err)
		return
	}
	//m.processCounter.Add(1)
	m.handleSingleParameterBuffer(parameterBuffer, parameterDetail, deviceID)
}

func (m *IBeamParameterManager) handleSingleParameterBuffer(parameterBuffer *ibeamParameterValueBuffer, parameterDetail *pb.ParameterDetail, deviceID uint32) {
	//mlog := m.log.WithField("parameter", fmt.Sprintf("%s (P:%d, D: %d)", m.parameterRegistry.PName(parameterDetail.Id.Parameter), parameterDetail.Id.Parameter, deviceID))
	mlog := m.log.WithField("pname", m.parameterRegistry.PName(parameterDetail.Id.Parameter))
	mlog = mlog.WithField("pid", parameterDetail.Id.Parameter)
	mlog = mlog.WithField("did", deviceID)

	// Function assumes mutexes are already locked

	parameterID := parameterDetail.Id.Parameter
	// ********************************************************************
	// First Basic Check Pipeline if the Parameter Value can be send to out
	// ********************************************************************

	if !parameterBuffer.isAssumedState.Load() {
		return
	}

	// For edgecases we should ensure that assumed state is true in the context of AcceptanceThjreshold
	if parameterDetail.ValueType == pb.ValueType_Floating || parameterDetail.ValueType == pb.ValueType_Integer && parameterDetail.AcceptanceThreshold != 0 {
		isAcceptanceModeOverride := false
		// ceck for override
		if parameterDetail.ValueType == pb.ValueType_Floating {
			tv := parameterBuffer.targetValue.GetFloating()
			cv := parameterBuffer.currentValue.GetFloating()
			isAcceptanceModeOverride = (tv-parameterDetail.AcceptanceThreshold < cv && cv < tv+parameterDetail.AcceptanceThreshold)
		}

		if parameterDetail.ValueType == pb.ValueType_Integer {
			tv := parameterBuffer.targetValue.GetInteger()
			cv := parameterBuffer.currentValue.GetInteger()
			isAcceptanceModeOverride = (tv-int32(parameterDetail.AcceptanceThreshold) < cv && cv < tv+int32(parameterDetail.AcceptanceThreshold))
		}

		if isAcceptanceModeOverride {
			parameterBuffer.isAssumedState.Store(false)
			parameterBuffer.targetValue = proto.Clone(parameterBuffer.currentValue).(*pb.ParameterValue)
			parameterBuffer.smoothingLastSent = nil
			m.serverClientsStream <- b.Param(parameterID, deviceID, parameterBuffer.getParameterValue())
			return
		}
	}

	if ratelimit, exists := m.parameterRegistry.modelRateLimiter[parameterDetail.Id.Model]; exists && !parameterBuffer.hasFlag(FlagRateLimitExclude) {
		// Is is send after Control Delay time
		lastTime := m.parameterRegistry.getLastEvent(deviceID)
		if time.Since(lastTime).Milliseconds() < int64(ratelimit) {
			m.reevaluateIn(time.Millisecond*time.Duration(ratelimit)-time.Since(lastTime), parameterBuffer, parameterID, deviceID)
			return
		}
	}

	// Is is send after Control Delay time
	if parameterDetail.ControlDelayMs != 0 && time.Since(parameterBuffer.lastUpdate).Milliseconds() < int64(parameterDetail.ControlDelayMs) {
		m.reevaluateIn(time.Millisecond*time.Duration(parameterDetail.ControlDelayMs)-time.Since(parameterBuffer.lastUpdate), parameterBuffer, parameterID, deviceID)
		return
	}

	// Is the Retry Limit reached
	if parameterDetail.RetryCount != 0 && parameterDetail.FeedbackStyle != pb.FeedbackStyle_NoFeedback {
		if parameterBuffer.tryCount >= parameterDetail.RetryCount {
			// Is is send after Quarantine Delay time ? Do not fire an error max retry before quarantine delay is over
			if parameterDetail.QuarantineDelayMs != 0 && time.Since(parameterBuffer.lastUpdate).Milliseconds() < int64(parameterDetail.QuarantineDelayMs) {
				m.reevaluateIn(time.Millisecond*time.Duration(parameterDetail.QuarantineDelayMs)-time.Since(parameterBuffer.lastUpdate), parameterBuffer, parameterID, deviceID)
				return
			}

			mlog.Errorf("Failed to set parameter %v '%v' in %v tries on device %v", parameterID, parameterDetail.Name, parameterDetail.RetryCount, deviceID)
			parameterBuffer.targetValue = proto.Clone(parameterBuffer.currentValue).(*pb.ParameterValue)
			parameterBuffer.smoothingLastSent = nil

			perr := paramError(parameterID, deviceID, pb.ParameterError_MaxRetrys)
			perr.Value = []*pb.ParameterValue{parameterBuffer.getParameterValue()}
			m.serverClientsStream <- perr
			parameterBuffer.tryCount = 0
			return
		}
	}

	// Set the lastUpdate Time
	parameterBuffer.lastUpdate = time.Now()
	if !parameterBuffer.hasFlag(FlagRateLimitExclude) {
		m.parameterRegistry.setLastEvent(deviceID)
	}

	switch parameterDetail.ControlStyle {
	case pb.ControlStyle_Normal:
		// Value smoothing: send intermediate steps instead of jumping to target
		isSmoothingStep := false
		var valueToSend *pb.ParameterValue

		if parameterBuffer.smoothingMaxStep > 0 && parameterBuffer.smoothingLastSent != nil {
			stepValue := computeSmoothingStep(
				parameterBuffer.smoothingLastSent,
				parameterBuffer.targetValue,
				parameterBuffer.smoothingMaxStep,
				parameterDetail.ValueType,
			)

			if smoothingReachedTarget(stepValue, parameterBuffer.targetValue, parameterDetail.ValueType) {
				// Final step: send actual target and end smoothing
				parameterBuffer.smoothingLastSent = nil
				valueToSend = parameterBuffer.getParameterValue()
			} else {
				// Intermediate step
				isSmoothingStep = true
				parameterBuffer.smoothingLastSent = proto.Clone(stepValue).(*pb.ParameterValue)
				valueToSend = &pb.ParameterValue{
					DimensionID:    parameterBuffer.dimensionID,
					Available:      parameterBuffer.available,
					IsAssumedState: true,
					Value:          stepValue.Value,
					Invalid:        parameterBuffer.targetValue.Invalid,
					MetaValues:     parameterBuffer.targetValue.MetaValues,
				}
			}
		} else {
			valueToSend = parameterBuffer.getParameterValue()
		}

		if parameterDetail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
			parameterBuffer.currentValue = proto.Clone(valueToSend).(*pb.ParameterValue)
			if !isSmoothingStep {
				parameterBuffer.isAssumedState.Store(false)
			}
		}

		// If we Have a current Option, get the Value for the option from the Option List
		if parameterDetail.ValueType == pb.ValueType_Opt {
			if value, ok := parameterBuffer.targetValue.Value.(*pb.ParameterValue_CurrentOption); ok {
				parameterBuffer.targetValue.Value = &pb.ParameterValue_CurrentOption{CurrentOption: value.CurrentOption}
			}
		}

		m.out <- b.Param(parameterID, deviceID, valueToSend)
		parameterBuffer.tryCount++

		if parameterDetail.FeedbackStyle == pb.FeedbackStyle_DelayedFeedback ||
			parameterDetail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
			m.serverClientsStream <- b.Param(parameterID, deviceID, valueToSend)
		}

		if isSmoothingStep {
			parameterBuffer.tryCount = 0
			m.reevaluateIn(time.Millisecond*time.Duration(parameterDetail.ControlDelayMs), parameterBuffer, parameterID, deviceID)
		} else if parameterDetail.FeedbackStyle != pb.FeedbackStyle_NoFeedback && parameterDetail.ControlDelayMs != 0 {
			m.reevaluateIn(time.Millisecond*time.Duration(parameterDetail.ControlDelayMs+1), parameterBuffer, parameterID, deviceID)
		}
	case pb.ControlStyle_Incremental:
		m.out <- b.Param(parameterID, deviceID, parameterBuffer.getParameterValue())
		parameterBuffer.tryCount++
		m.reevaluateIn(time.Millisecond*time.Duration(parameterDetail.ControlDelayMs), parameterBuffer, parameterID, deviceID)

	case pb.ControlStyle_NoControl, pb.ControlStyle_Oneshot:
		if parameterDetail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
			parameterBuffer.targetValue = proto.Clone(parameterBuffer.currentValue).(*pb.ParameterValue)
		}

	default:
		mlog.Errorf("Could not match controlltype")
		return
	}
}

func (m *IBeamParameterManager) reevaluateIn(t time.Duration, buffer *ibeamParameterValueBuffer, parameterID, deviceID uint32) {
	if t == 0 {
		t = time.Microsecond * 100 // This just ensures there is at least a little bit of time for the set of the system to catch up before triggering a reevaluation
	}

	buffer.reEvaluationTimerMu.Lock()
	defer buffer.reEvaluationTimerMu.Unlock()

	if buffer.reEvaluationTimer != nil {
		remainingDuration := time.Until(buffer.reEvaluationTimer.end)
		if t > remainingDuration {
			return
		}

		m.log.Traceln("Resceduling in", t.Milliseconds(), "milliseconds")
		buffer.reEvaluationTimer.timer.Reset(t)
		return
	}

	m.log.Traceln("Scheduling reevaluation of "+fmt.Sprint(parameterID)+" in", t.Milliseconds(), "milliseconds")

	addr := paramDimensionAddress{
		parameter:   parameterID,
		device:      deviceID,
		dimensionID: buffer.getParameterValue().DimensionID,
	}
	buffer.reEvaluationTimer = &timeTimer{end: time.Now().Add(t), timer: time.AfterFunc(t, func() {
		m.reEvaluate(addr)
		buffer.reEvaluationTimerMu.Lock()
		buffer.reEvaluationTimer = nil
		buffer.reEvaluationTimerMu.Unlock()
	})}
}

func (m *IBeamParameterManager) reEvaluate(addr paramDimensionAddress) {
	select {
	case m.parameterEvent <- addr:
	default:
		m.log.Error("Parameter Event Channel would block, dropped reevaluation trigger to avoid deadlock")
	}
}
