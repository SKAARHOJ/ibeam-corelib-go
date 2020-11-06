package ibeam_core_lib

import (
	"fmt"

	ibeam_core "github.com/SKAARHOJ/ibeam-core-go/ibeam-core"
)

// GenerateOptionList returns a new OptionList with ascending IDs
func GenerateOptionList(options ...string) (optionList *ibeam_core.OptionList) {
	optionList = &ibeam_core.OptionList{}
	for index, option := range options {
		optionList.Options = append(optionList.Options, &ibeam_core.ParameterOption{
			Id:   uint32(index),
			Name: option,
		})
	}
	return optionList
}

// GenerateParameterMetaList helper returns a new list of ParameterMetaDescriptions for use in register parameter
func GenerateParameterMetaList(metaDescs ...*ibeam_core.ParameterMetaDescription) []*ibeam_core.ParameterMetaDescription {
	metaList := make([]*ibeam_core.ParameterMetaDescription, 0)
	for _, metaDesc := range metaDescs {
		metaList = append(metaList, metaDesc)
	}
	return metaList
}

// GetNameOfParameter returns the Name of a Parameter with a given ParameterID in a given ParameterDetail Map
func GetNameOfParameter(parameterID, modelID uint32, pds []map[string]ibeam_core.ModelParameterID) (string, error) {
	if len(pds) <= int(modelID) {
		return "", fmt.Errorf("Could not find Parameter for Model with id %d", modelID)
	}

	for name, pd := range pds[modelID] {
		if pd.Parameter == parameterID {
			return name, nil
		}
	}
	return "", fmt.Errorf("Could not find Parameter with id %v", parameterID)
}

func getElementNameFromOptionListByID(list *ibeam_core.OptionList, id ibeam_core.ParameterValue_CurrentOption) (string, error) {
	for _, option := range list.Options {
		if option.Id == id.CurrentOption {
			return option.Name, nil
		}
	}
	return "", fmt.Errorf("No Name found in OptionList '%v' with ID %v", list, id)
}

func getIDFromOptionListByElementName(list *ibeam_core.OptionList, name string) (uint32, error) {
	for _, option := range list.Options {
		if option.Name == name {
			return option.Id, nil
		}
	}
	return 0, fmt.Errorf("No ID found in OptionList '%v' for Name %v", list, name)
}
