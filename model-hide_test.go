package ibeamcorelib

import (
	"testing"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
)

func TestGenericModelVisibleByDefault(t *testing.T) {
	_, registry, _, _ := CreateServer(&pb.CoreInfo{Name: "hidetest", Description: "test"})
	registry.RegisterModel(&pb.ModelInfo{Id: 1, Name: "Model 1", Description: "specific"})

	if got := registry.modelInfos[0].DevelopmentStatus; got == "hidden" {
		t.Fatalf("generic model must stay visible unless HideGenericModel is called, got %q", got)
	}
}

func TestHideGenericModel(t *testing.T) {
	_, registry, _, _ := CreateServer(&pb.CoreInfo{Name: "hidetest", Description: "test"})
	registry.RegisterModel(&pb.ModelInfo{Id: 1, Name: "Model 1", Description: "specific", DevelopmentStatus: "released"})
	registry.HideGenericModel()

	if got := registry.modelInfos[0].DevelopmentStatus; got != "hidden" {
		t.Fatalf("generic model should be hidden, got %q", got)
	}
	if got := registry.modelInfos[1].DevelopmentStatus; got != "released" {
		t.Fatalf("specific model status must not change, got %q", got)
	}
}
