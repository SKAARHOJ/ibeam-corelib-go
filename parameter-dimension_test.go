package ibeamcorelib

import (
	"reflect"
	"testing"
	"time"
)

func TestIbeamParameterDimension_isValue(t *testing.T) {
	type fields struct {
		subDimensions []*IbeamParameterDimension
		value         *IBeamParameterValueBuffer
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{name: "Test isValue with value", fields: fields{value: &IBeamParameterValueBuffer{}}, want: true},
		{name: "Test isValue with dimension", fields: fields{value: nil}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &IbeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			if got := pd.isValue(); got != tt.want {
				t.Errorf("IbeamParameterDimension.isValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIbeamParameterDimension_Value(t *testing.T) {
	type fields struct {
		subDimensions []*IbeamParameterDimension
		value         *IBeamParameterValueBuffer
	}
	buffer := &IBeamParameterValueBuffer{}

	tests := []struct {
		name    string
		fields  fields
		want    *IBeamParameterValueBuffer
		wantErr bool
	}{
		{name: "Test Value with value", fields: fields{value: buffer}, want: buffer},
		{name: "Test Value with no value", fields: fields{value: nil}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &IbeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			got, err := pd.Value()
			if (err != nil) != tt.wantErr {
				t.Errorf("IbeamParameterDimension.Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IbeamParameterDimension.Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIbeamParameterDimension_Subdimensions(t *testing.T) {
	type fields struct {
		subDimensions []*IbeamParameterDimension
		value         *IBeamParameterValueBuffer
	}
	buffer := &IBeamParameterValueBuffer{}
	dim := &IbeamParameterDimension{}
	dimRay := []*IbeamParameterDimension{dim}

	tests := []struct {
		name    string
		fields  fields
		want    []*IbeamParameterDimension
		wantErr bool
	}{
		{name: "Test Subdimensions with subdim", fields: fields{subDimensions: dimRay}, want: dimRay},
		{name: "Test Subdimensions with value", fields: fields{value: buffer}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &IbeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			got, err := pd.Subdimensions()
			if (err != nil) != tt.wantErr {
				t.Errorf("IbeamParameterDimension.Subdimensions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IbeamParameterDimension.Subdimensions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIbeamParameterDimension_MultiIndexHasValue(t *testing.T) {
	type fields struct {
		subDimensions []*IbeamParameterDimension
		value         *IBeamParameterValueBuffer
	}
	type args struct {
		dimensionID []uint32
	}

	initialValueDimension := &IbeamParameterDimension{
		value: &IBeamParameterValueBuffer{
			dimensionID:    make([]uint32, 0),
			available:      true,
			isAssumedState: true,
			lastUpdate:     time.Now(),
		},
	}

	dimensions1 := generateDimensions([]uint32{2, 3, 5}, initialValueDimension)

	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name:   "With valid max dimID",
			fields: fields{subDimensions: dimensions1.subDimensions, value: dimensions1.value},
			args:   args{dimensionID: []uint32{2, 3, 5}},
			want:   true,
		},
		{
			name:   "With valid min dimID",
			fields: fields{subDimensions: dimensions1.subDimensions, value: dimensions1.value},
			args:   args{dimensionID: []uint32{1, 1, 1}},
			want:   true,
		},
		{
			name:   "With too long dim id",
			fields: fields{subDimensions: dimensions1.subDimensions, value: dimensions1.value},
			args:   args{dimensionID: []uint32{1, 1, 1, 4}},
			want:   false,
		},
		{
			name:   "With too short dim id",
			fields: fields{subDimensions: dimensions1.subDimensions, value: dimensions1.value},
			args:   args{dimensionID: []uint32{1, 1}},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &IbeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			if got := pd.MultiIndexHasValue(tt.args.dimensionID); got != tt.want {
				t.Errorf("IbeamParameterDimension.MultiIndexHasValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIbeamParameterDimension_MultiIndex(t *testing.T) {
	type fields struct {
		subDimensions []*IbeamParameterDimension
		value         *IBeamParameterValueBuffer
	}
	type args struct {
		dimensionID []uint32
	}

	initialValueDimension := &IbeamParameterDimension{
		value: &IBeamParameterValueBuffer{
			dimensionID:    make([]uint32, 0),
			available:      true,
			isAssumedState: true,
			lastUpdate:     time.Now(),
		},
	}
	dimensions1 := generateDimensions([]uint32{2, 3, 5}, initialValueDimension)

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *IbeamParameterDimension
		wantErr bool
	}{
		{
			name:   "With valid max dimID",
			fields: fields{subDimensions: dimensions1.subDimensions, value: dimensions1.value},
			args:   args{dimensionID: []uint32{2, 3, 5}},
			want:   initialValueDimension,
		},
		{
			name:   "With valid min dimID",
			fields: fields{subDimensions: dimensions1.subDimensions, value: dimensions1.value},
			args:   args{dimensionID: []uint32{1, 1, 1}},
			want:   initialValueDimension,
		},

		{
			name:    "With too long dim id",
			fields:  fields{subDimensions: dimensions1.subDimensions, value: dimensions1.value},
			args:    args{dimensionID: []uint32{1, 1, 1, 4}},
			wantErr: true,
		},
		{
			name:    "With too short dim id returns valid dimension",
			fields:  fields{subDimensions: dimensions1.subDimensions, value: dimensions1.value},
			args:    args{dimensionID: []uint32{1, 1}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &IbeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			got, err := pd.MultiIndex(tt.args.dimensionID)
			if (err != nil) != tt.wantErr {
				t.Errorf("IbeamParameterDimension.MultiIndex() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Errorf("IbeamParameterDimension.MultiIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIbeamParameterDimension_index(t *testing.T) {
	type fields struct {
		subDimensions []*IbeamParameterDimension
		value         *IBeamParameterValueBuffer
	}
	type args struct {
		index uint32
	}

	initialValueDimension := &IbeamParameterDimension{
		value: &IBeamParameterValueBuffer{
			dimensionID:    make([]uint32, 0),
			available:      true,
			isAssumedState: true,
			lastUpdate:     time.Now(),
		},
	}
	dimensions1 := generateDimensions([]uint32{5}, initialValueDimension)

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *IbeamParameterDimension
		wantErr bool
	}{
		{
			name:   "With valid max index",
			fields: fields{subDimensions: dimensions1.subDimensions, value: dimensions1.value},
			args:   args{index: 4},
		},
		{
			name:   "With valid min dimID",
			fields: fields{subDimensions: dimensions1.subDimensions, value: dimensions1.value},
			args:   args{index: 0},
		},
		{
			name:    "With too big index",
			fields:  fields{subDimensions: dimensions1.subDimensions, value: dimensions1.value},
			args:    args{index: 5},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &IbeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			got, err := pd.index(tt.args.index)
			if (err != nil) != tt.wantErr {
				t.Errorf("IbeamParameterDimension.index() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Errorf("IbeamParameterDimension.index() = %v, want %v", got, tt.want)
			}
		})
	}
}
