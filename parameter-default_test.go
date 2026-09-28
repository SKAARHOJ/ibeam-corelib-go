package ibeamcorelib

import (
	"testing"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	b "github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers"
)

func setupDefaultTestServer(t *testing.T) (*IBeamParameterManager, *IBeamParameterRegistry, map[string]uint32) {
	t.Helper()
	manager, registry, _, _ := CreateServer(&pb.CoreInfo{Name: "defaulttest", Description: "test"})

	ids := map[string]uint32{}
	ids["dynamic"] = registry.RegisterParameter(&pb.ParameterDetail{
		Name: "dynamic", Label: "dynamic", ShortLabel: "dynamic", Description: "dynamic", ValueType: pb.ValueType_Integer,
		ControlStyle: pb.ControlStyle_Normal, FeedbackStyle: pb.FeedbackStyle_NormalFeedback, RetryCount: 1, ControlDelayMs: 50,
		Minimum: 0, Maximum: 100, DefaultValue: b.Int(50), DefaultIsDynamic: true,
		Dimensions: []*pb.DimensionDetail{{Name: "ch", Count: 2}},
	})
	ids["static"] = registry.RegisterParameter(&pb.ParameterDetail{
		Name: "static", Label: "static", ShortLabel: "static", Description: "static", ValueType: pb.ValueType_Integer,
		ControlStyle: pb.ControlStyle_Normal, FeedbackStyle: pb.FeedbackStyle_NormalFeedback, RetryCount: 1, ControlDelayMs: 50,
		Minimum: 0, Maximum: 100, DefaultValue: b.Int(50),
	})

	if _, err := registry.RegisterDevice(1, 0); err != nil {
		t.Fatal(err)
	}
	// capture what would be sent to clients, the original stream is consumed by the distributor
	manager.serverClientsStream = make(chan *pb.Parameter, 100)
	return manager, registry, ids
}

func drainClients(m *IBeamParameterManager) []*pb.Parameter {
	out := make([]*pb.Parameter, 0)
	for {
		select {
		case p := <-m.serverClientsStream:
			out = append(out, p)
		default:
			return out
		}
	}
}

func defaultUpdatesFor(params []*pb.Parameter, pid uint32) []*pb.ParameterValue {
	out := make([]*pb.ParameterValue, 0)
	for _, p := range params {
		if p.Id.Parameter != pid {
			continue
		}
		for _, v := range p.Value {
			if v.GetDefaultUpdate() != nil {
				out = append(out, v)
			}
		}
	}
	return out
}

func TestNewDefault(t *testing.T) {
	v := b.NewDefault(b.Int(5), 1, 2)
	if !dimsEqual(v.DimensionID, []uint32{1, 2}) || v.GetDefaultUpdate().GetInteger() != 5 {
		t.Errorf("unexpected default update %v", v)
	}
}

func TestDynamicDefaultIngest(t *testing.T) {
	manager, registry, ids := setupDefaultTestServer(t)

	manager.ingestCurrentParameter(b.Param(ids["dynamic"], 1, b.NewDefault(b.Int(42), 2)))
	updates := defaultUpdatesFor(drainClients(manager), ids["dynamic"])
	if len(updates) != 1 || updates[0].GetDefaultUpdate().GetInteger() != 42 || !dimsEqual(updates[0].DimensionID, []uint32{2}) {
		t.Fatalf("expected one forwarded default update, got %v", updates)
	}

	// stored per dimension
	d, err := registry.GetParameterDefault(ids["dynamic"], 1, 2)
	if err != nil || d.GetInteger() != 42 {
		t.Errorf("expected dynamic default 42 for dim 2, got %v %v", d, err)
	}
	d, err = registry.GetParameterDefault(ids["dynamic"], 1, 1)
	if err != nil || d.GetInteger() != 50 {
		t.Errorf("expected static default 50 for dim 1, got %v %v", d, err)
	}

	// replayed to new subscribers
	ds := registry.loadDeviceState(1)
	replayed := getValues(registry.log, ds.params[ids["dynamic"]], true)
	found := false
	for _, v := range replayed {
		if v.GetDefaultUpdate() != nil {
			found = dimsEqual(v.DimensionID, []uint32{2}) && v.GetDefaultUpdate().GetInteger() == 42
		}
	}
	if !found {
		t.Errorf("dynamic default not replayed: %v", replayed)
	}

	// wrong type is rejected
	manager.ingestCurrentParameter(b.Param(ids["dynamic"], 1, b.NewDefault(b.Float(1.5), 1)))
	if updates := defaultUpdatesFor(drainClients(manager), ids["dynamic"]); len(updates) != 0 {
		t.Errorf("expected wrong typed default to be rejected, got %v", updates)
	}

	// parameters without DefaultIsDynamic reject updates
	manager.ingestCurrentParameter(b.Param(ids["static"], 1, b.NewDefault(b.Int(1))))
	if updates := defaultUpdatesFor(drainClients(manager), ids["static"]); len(updates) != 0 {
		t.Errorf("expected default update without flag to be rejected, got %v", updates)
	}
	d, _ = registry.GetParameterDefault(ids["static"], 1)
	if d.GetInteger() != 50 {
		t.Errorf("expected static default to be unchanged, got %v", d)
	}
}
