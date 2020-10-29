package ibeam_core_lib

import (
	"fmt"

	ibeam_core "github.com/SKAARHOJ/ibeam-core-go/ibeam-core"
)

func GenerateOptionList(options []string) (optionList *ibeam_core.OptionList) {
	optionList = &ibeam_core.OptionList{}
	for index, option := range options {
		optionList.Options = append(optionList.Options, &ibeam_core.ParameterOption{
			Id:   uint32(index),
			Name: option,
		})
	}
	return optionList
}

func GetNameOfParameter(parameterID uint32, pds map[string]ibeam_core.ParameterDetail) (string, error) {
	for _, pd := range pds {
		if pd.Id.Parameter == parameterID {
			return pd.Name, nil
		}
	}
	return "", fmt.Errorf("Could not find Parameter with id %v", parameterID)
}
