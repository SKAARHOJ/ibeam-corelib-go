package ibeamcorelib

import (
	"reflect"
	"testing"
	"time"
)

func TestIBeamParameterDimension_isValue(t *testing.T) {
	type fields struct {
		subDimensions []*IBeamParameterDimension
		value         *ibeamParameterValueBuffer
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{name: "Test isValue with value", fields: fields{value: &ibeamParameterValueBuffer{}}, want: true},
		{name: "Test isValue with dimension", fields: fields{value: nil}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &IBeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			if got := pd.isValue(); got != tt.want {
				t.Errorf("IBeamParameterDimension.isValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIBeamParameterDimension_Value(t *testing.T) {
	type fields struct {
		subDimensions []*IBeamParameterDimension
		value         *ibeamParameterValueBuffer
	}
	buffer := &ibeamParameterValueBuffer{}

	tests := []struct {
		name    string
		fields  fields
		want    *ibeamParameterValueBuffer
		wantErr bool
	}{
		{name: "Test Value with value", fields: fields{value: buffer}, want: buffer},
		{name: "Test Value with no value", fields: fields{value: nil}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &IBeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			got, err := pd.Value()
			if (err != nil) != tt.wantErr {
				t.Errorf("IBeamParameterDimension.Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IBeamParameterDimension.Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIBeamParameterDimension_Subdimensions(t *testing.T) {
	type fields struct {
		subDimensions []*IBeamParameterDimension
		value         *ibeamParameterValueBuffer
	}
	buffer := &ibeamParameterValueBuffer{}
	dim := &IBeamParameterDimension{}
	dimRay := []*IBeamParameterDimension{dim}

	tests := []struct {
		name    string
		fields  fields
		want    []*IBeamParameterDimension
		wantErr bool
	}{
		{name: "Test Subdimensions with subdim", fields: fields{subDimensions: dimRay}, want: dimRay},
		{name: "Test Subdimensions with value", fields: fields{value: buffer}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &IBeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			got, err := pd.Subdimensions()
			if (err != nil) != tt.wantErr {
				t.Errorf("IBeamParameterDimension.Subdimensions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IBeamParameterDimension.Subdimensions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIBeamParameterDimension_MultiIndexHasValue(t *testing.T) {
	type fields struct {
		subDimensions []*IBeamParameterDimension
		value         *ibeamParameterValueBuffer
	}
	type args struct {
		dimensionID []uint32
	}

	initialValueDimension := &IBeamParameterDimension{
		value: &ibeamParameterValueBuffer{
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
			pd := &IBeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			if got := pd.MultiIndexHasValue(tt.args.dimensionID); got != tt.want {
				t.Errorf("IBeamParameterDimension.MultiIndexHasValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIBeamParameterDimension_MultiIndex(t *testing.T) {
	type fields struct {
		subDimensions []*IBeamParameterDimension
		value         *ibeamParameterValueBuffer
	}
	type args struct {
		dimensionID []uint32
	}

	initialValueDimension := &IBeamParameterDimension{
		value: &ibeamParameterValueBuffer{
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
		want    *IBeamParameterDimension
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
			pd := &IBeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			got, err := pd.MultiIndex(tt.args.dimensionID)
			if (err != nil) != tt.wantErr {
				t.Errorf("IBeamParameterDimension.MultiIndex() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Errorf("IBeamParameterDimension.MultiIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIBeamParameterDimension_index(t *testing.T) {
	type fields struct {
		subDimensions []*IBeamParameterDimension
		value         *ibeamParameterValueBuffer
	}
	type args struct {
		index uint32
	}

	initialValueDimension := &IBeamParameterDimension{
		value: &ibeamParameterValueBuffer{
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
		want    *IBeamParameterDimension
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
			pd := &IBeamParameterDimension{
				subDimensions: tt.fields.subDimensions,
				value:         tt.fields.value,
			}
			got, err := pd.index(tt.args.index)
			if (err != nil) != tt.wantErr {
				t.Errorf("IBeamParameterDimension.index() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Errorf("IBeamParameterDimension.index() = %v, want %v", got, tt.want)
			}
		})
	}
}
