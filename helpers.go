package ibeam_core // _lib

import "fmt"

func GenerateOptionList(options []string) (optionList *OptionList) {
	optionList = &OptionList{}
	for index, option := range options {
		optionList.Options = append(optionList.Options, &ParameterOption{
			Id:   uint32(index),
			Name: option,
		})
	}
	return optionList
}

func GetNameOfParameter(parameterID uint32, pds map[string]ParameterDetail) (string, error) {
	for _, pd := range pds {
		if pd.Id.Parameter == parameterID {
			return pd.Name, nil
		}
	}
	return "", fmt.Errorf("Could not find Parameter with id %v", parameterID)
}
