package ibeamcorelib

import elog "github.com/s00500/env_logger"

//the test registry is used for simple tests where only pid and pname is used. Will crash if other registry options are used. If models are added, it will add the same map for each modelid specified
func NewTestRegistry(ids map[uint32]string, models ...uint32) *IBeamParameterRegistry {
	registry := IBeamParameterRegistry{log: elog.GetLoggerForPrefix("ib/registry")}

	reverse := map[string]uint32{}
	for k, v := range ids {
		reverse[v] = k
	}
	cachedIDMap = map[uint32]map[uint32]string{
		0: ids,
	}
	cachedNameMap = map[uint32]map[string]uint32{
		0: reverse,
	}

	for _, modelId := range models {
		cachedIDMap[modelId] = ids
		cachedNameMap[modelId] = reverse
	}

	registry.parametersDone.Store(true)
	return &registry
}

//this function creates a test registry with automatic ids based on the hash of the names. Only use with Pid and PName. See the warning for NewTestRegistry.
func NewSimpleTestRegistry(parameterNames ...string) *IBeamParameterRegistry {
	ids := map[uint32]string{}

	for _, v := range parameterNames {
		ids[idFromName(v)] = v
	}

	return NewTestRegistry(ids)
}
