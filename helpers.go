package ibeamcorelib

import (
	"fmt"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
)

// MetaElements Type alias to make parameter definition nicer
type MetaElements map[string]*pb.ParameterMetaDetail

// DimensionDetails Type alias to make parameter definition nicer
type DimensionDetails []*pb.DimensionDetail

// DescreteValueDetails Type alias to make parameter definition nicer
type DescreteValueDetails []*pb.DescreteValueDetail

// GenerateOptionList returns a new OptionList with ascending IDs
func GenerateOptionList(options ...string) (optionList *pb.OptionList) {
	optionList = &pb.OptionList{}
	for index, option := range options {
		optionList.Options = append(optionList.Options, &pb.ParameterOption{
			Id:   uint32(index),
			Name: option,
		})
	}
	return optionList
}

// GenerateOptionListWithOffset returns a new OptionList with ascending IDs all offset by a set amount. If offset is 1 first option would have id 1
func GenerateOptionListWithOffset(offset uint32, options ...string) (optionList *pb.OptionList) {
	optionList = &pb.OptionList{}
	for index, option := range options {
		optionList.Options = append(optionList.Options, &pb.ParameterOption{
			Id:   uint32(index) + offset,
			Name: option,
		})
	}
	return optionList
}

func getIDFromOptionListByElementName(list *pb.OptionList, name string) (uint32, error) {
	for _, option := range list.Options {
		if option.Name == name {
			return option.Id, nil
		}
	}
	return 0, fmt.Errorf("no ID found in OptionList '%v' for Name %v", list, name)
}

func paramError(pid uint32, did uint32, e pb.ParameterError) *pb.Parameter {
	return &pb.Parameter{
		Id:    &pb.DeviceParameterID{Device: did, Parameter: pid},
		Error: e,
	}
}

func HashedID(name string) uint32 {
	return idFromName(name)
}

// Internal helpers

func dimsEqual(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
