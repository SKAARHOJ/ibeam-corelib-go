package ibeamcorelib

type ModelRegisterOption func(r *IBeamParameterRegistry, modelid uint32)

func WithGlobalRateLimit(limit uint) func(r *IBeamParameterRegistry, modelid uint32) {
	return func(r *IBeamParameterRegistry, modelid uint32) {
		r.modelRateLimiter[modelid] = limit
	}
}
