package ibeam_core_lib

import (
	"math/rand"
	"testing"

	ibeam_core "github.com/SKAARHOJ/ibeam-core-go/ibeam-core"
)

func Test_containsDeviceParameter(t *testing.T) {
	type args struct {
		dpID  *ibeam_core.DeviceParameterID
		dpIDs *ibeam_core.DeviceParameterIDs
	}

	randomID := &ibeam_core.DeviceParameterID{
		Device:    rand.Uint32(),
		Parameter: rand.Uint32(),
	}

	randomID2 := &ibeam_core.DeviceParameterID{
		Device:    rand.Uint32(),
		Parameter: rand.Uint32(),
	}

	randomID3 := &ibeam_core.DeviceParameterID{
		Device:    rand.Uint32(),
		Parameter: rand.Uint32(),
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "DPIDs that contains the DeviceParameter",
			args: args{
				dpID: randomID,
				dpIDs: &ibeam_core.DeviceParameterIDs{
					Ids: []*ibeam_core.DeviceParameterID{
						randomID,
						randomID2,
					},
				},
			},
			want: true,
		},
		{
			name: "DPIDs that not contains the DeviceParameter",
			args: args{
				dpID: randomID,
				dpIDs: &ibeam_core.DeviceParameterIDs{
					Ids: []*ibeam_core.DeviceParameterID{
						randomID2,
						randomID3,
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsDeviceParameter(tt.args.dpID, tt.args.dpIDs); got != tt.want {
				t.Errorf("containsDeviceParameter() = %v, want %v", got, tt.want)
			}
		})
	}
}
