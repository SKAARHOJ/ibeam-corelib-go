package ibeamcorelib

import (
	"fmt"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
)

// MetaElements Type alias to make parameter definition nicer
type MetaElements map[string]pb.ParameterMetaType

// DimensionDetails Type alias to make parameter definition nicer
type DimensionDetails []*pb.DimensionDetail

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

// GetNameOfParameterOfModel returns the Name of a Parameter with a given id in a given ParameterDetail Map
//
// Deprecated: please use the registry function GetParameterNameOfModel
func GetNameOfParameterOfModel(parameterID, modelID uint32, paramIDs []map[string]uint32) (string, error) {
	if len(paramIDs) <= int(modelID) {
		return "", fmt.Errorf("Could not find Parameter for Model with id %d", modelID)
	}

	for name, pd := range paramIDs[modelID] {
		if pd == parameterID {
			return name, nil
		}
	}
	return "", fmt.Errorf("Could not find Parameter with id %v", parameterID)
}

// GetNameOfParameter returns the Name of a Parameter with a given id in a given ParameterDetail Map
//
// Deprecated: please use the registry function GetParameterNameOfModel
func GetNameOfParameter(parameterID uint32, paramIDs map[string]uint32) (string, error) {
	for name, pd := range paramIDs {
		if pd == parameterID {
			return name, nil
		}
	}
	return "", fmt.Errorf("Could not find Parameter with id %v", parameterID)
}

func getElementNameFromOptionListByID(list *pb.OptionList, id pb.ParameterValue_CurrentOption) (string, error) {
	for _, option := range list.Options {
		if option.Id == id.CurrentOption {
			return option.Name, nil
		}
	}
	return "", fmt.Errorf("No Name found in OptionList '%v' with ID %v", list, id)
}

func getIDFromOptionListByElementName(list *pb.OptionList, name string) (uint32, error) {
	for _, option := range list.Options {
		if option.Name == name {
			return option.Id, nil
		}
	}
	return 0, fmt.Errorf("No ID found in OptionList '%v' for Name %v", list, name)
}
