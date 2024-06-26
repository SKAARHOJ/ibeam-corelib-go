package ibeamcorelib

import (
	"strings"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	log "github.com/s00500/env_logger"
)

func validateParameter(rlog *log.Entry, detail *pb.ParameterDetail) {
	// Fatals
	if detail.Name == "" {
		rlog.Fatalf("Parameter: ID %v: No name set", detail.Id)
	}

	if strings.Contains(detail.Name, " ") {
		rlog.Fatalf("Parameter: ID %v: Name %q contains space", detail.Id, detail.Name)
	}

	if strings.Contains(detail.Name, "/") {
		rlog.Fatalf("Parameter: ID %v: Name %q contains /", detail.Id, detail.Name)
	}

	/*
		if len(detail.ShortLabel) > 11 {
			rlog.Fatalf("Parameter: ID %v: Shortlabel is too long, must be 11 or lower chars", detail.Id)
		}

		if len(detail.ShortLabel) == 0 && len(detail.Label) > 11 {
			rlog.Fatalf("Parameter: ID %v: No shortlabel, but normal label is too long, must be 11 or lower chars or shortlabel provided", detail.Id)
		}
	*/

	if detail.ControlStyle == pb.ControlStyle_NoControl && detail.FeedbackStyle == pb.FeedbackStyle_NoFeedback {
		rlog.Fatalf("Parameter: '%v': Can not have no control and no feedback", detail.Name)
	}

	if detail.ControlStyle == pb.ControlStyle_Incremental && detail.IncDecStepsLowerLimit == 0 && detail.IncDecStepsUpperLimit == 0 {
		rlog.Fatalf("Parameter: '%v': Incremental: please provide lower and upper range for incDecSteps", detail.Name)
	}
	if detail.ControlStyle != pb.ControlStyle_Incremental &&
		(detail.IncDecStepsLowerLimit != 0 || detail.IncDecStepsUpperLimit != 0) {
		rlog.Fatalf("Parameter: '%v': Lower and upper limit are only valid on Incremental Control Mode", detail.Name)
	}
	if detail.Label == "" {
		rlog.Fatalf("Parameter: '%v': No label set", detail.Name)
	}
	if detail.ControlStyle != pb.ControlStyle_NoControl && detail.ControlStyle != pb.ControlStyle_Oneshot && detail.FeedbackStyle != pb.FeedbackStyle_NoFeedback && detail.RetryCount == 0 {
		rlog.Fatalf("Parameter '%v': Any non assumed value (FeedbackStyle_NoFeedback) needs to have RetryCount set", detail.Name)
	}

	if detail.RetryCount != 0 && detail.ControlDelayMs == 0 {
		rlog.Fatalf("Parameter '%v': RetryCount will not work without ControlDelayMs being set", detail.Name)
	}

	if detail.InputCurveExpo != 0 && detail.ValueType != pb.ValueType_Integer && detail.ValueType != pb.ValueType_Floating {
		rlog.Fatalf("Parameter '%v': InputCurves can only be used on Integer or Float values", detail.Name)
	}
	if detail.InputCurveExpo >= 1 || detail.InputCurveExpo == 0.5 || detail.InputCurveExpo < 0 {
		rlog.Fatalf("Parameter '%v': InputCurveExpo can not be bigger or equals 1, smaller than 0 or exactly 0.5 (which would be liniar anyways)", detail.Name)
	}

	if detail.DisplayFloatPrecision != pb.FloatPrecision_UndefinedFloatPrecision && detail.ValueType != pb.ValueType_Floating {
		rlog.Fatalf("Parameter '%v': Float Percision is only usable on floats", detail.Name)
	}

	if detail.FineSteps != 0 && detail.FineSteps < detail.AcceptanceThreshold {
		rlog.Fatalf("Parameter '%v': Acceptance Threshold needs to be smaller than the fine steps, this will cause issues otherwise", detail.Name)
	}

	// Dimension check
	for dId, dimensionDetail := range detail.Dimensions {
		if dimensionDetail.Count == 0 && len(dimensionDetail.ElementLabels) == 0 {
			rlog.Fatalf("Parameter '%v' Dimension '%d': Count is 0 and element labels are empty, this will cause a staleless parameter", detail.Name, dId)
		}
	}

	// Metavalue checks
	for mName, mDetail := range detail.MetaDetails {
		if mDetail.MetaType != pb.ParameterMetaType_MetaInteger && mDetail.MetaType != pb.ParameterMetaType_MetaFloating {
			if mDetail.Minimum != 0 || mDetail.Maximum != 0 {
				rlog.Warnf("Parameter metavalue '%s' of type %v has useless min / max values", mName, mDetail.MetaType)
			}
		}

		if mDetail.MetaType == pb.ParameterMetaType_MetaOption && len(mDetail.Options) == 0 {
			rlog.Fatalf("Parameter metavalue '%s' of type MetaOption has no option list", mName)
		} else if mDetail.MetaType != pb.ParameterMetaType_MetaOption && len(mDetail.Options) > 0 {
			rlog.Warnf("Parameter metavalue '%s' of type %v has useless option list", mName, mDetail.MetaType)
		}

	}

	// ValueType Checks
	switch detail.ValueType {
	case pb.ValueType_Floating:
		fallthrough
	case pb.ValueType_Integer:
		if detail.Minimum == 0 && detail.Maximum == 0 {
			rlog.Fatalf("Parameter: '%v': Integer or Floating needs min/max set", detail.Name)
		}
	case pb.ValueType_Binary:
		if detail.ControlStyle == pb.ControlStyle_Incremental {
			rlog.Fatalf("Parameter: '%v': Binary can not have incremental control", detail.Name)
		}
	case pb.ValueType_NoValue:
		if detail.FeedbackStyle != pb.FeedbackStyle_NoFeedback {
			rlog.Fatalf("Parameter: '%v': NoValue can not have Feedback", detail.Name)
		}
		if detail.Minimum != 0 || detail.Maximum != 0 {
			rlog.Fatalf("Parameter: '%v': NoValue can not min/max", detail.Name)
		}
	case pb.ValueType_Opt:
		if !detail.OptionListIsDynamic && (detail.OptionList == nil || detail.OptionList.Options == nil || len(detail.OptionList.Options) == 0) {
			rlog.Fatalf("Parameter: '%v': Missing option list", detail.Name)
		}
	}

	if detail.DefaultValue != nil {
		// Default Value Checks
		switch detail.DefaultValue.Value.(type) {
		case *pb.ParameterValue_Integer:
			if detail.ValueType != pb.ValueType_Integer {
				rlog.Fatalf("Parameter: '%v': Invalid default value for %s", detail.Name, detail.ValueType)
			}
		case *pb.ParameterValue_IncDecSteps:
			rlog.Fatalf("Parameter: '%v': DefaultValue cant be IncDecSteps", detail.Name)
		case *pb.ParameterValue_Floating:
			if detail.ValueType != pb.ValueType_Floating {
				rlog.Fatalf("Parameter: '%v': Invalid default value for %s", detail.Name, detail.ValueType)
			}
		case *pb.ParameterValue_Str:
			if detail.ValueType != pb.ValueType_String && detail.ValueType != pb.ValueType_Opt {
				rlog.Fatalf("Parameter: '%v': Invalid default value for %s", detail.Name, detail.ValueType)
			}
		case *pb.ParameterValue_CurrentOption:
			if detail.ValueType != pb.ValueType_Opt {
				rlog.Fatalf("Parameter: '%v': Invalid default value for %s", detail.Name, detail.ValueType)
			}
		case *pb.ParameterValue_Cmd:
			rlog.Fatalf("Parameter: '%v': DefaultValue cant be Cmd", detail.Name)
		case *pb.ParameterValue_Binary:
			if detail.ValueType != pb.ValueType_Binary {
				rlog.Fatalf("Parameter: '%v': Invalid default value for %s", detail.Name, detail.ValueType)
			}
		case *pb.ParameterValue_OptionListUpdate:
			rlog.Fatalf("Parameter: '%v': DefaultValue cant be OptionListUpdate", detail.Name)
		case *pb.ParameterValue_MinimumUpdate:
			rlog.Fatalf("Parameter: '%v': DefaultValue cant be MinUpdate", detail.Name)
		case *pb.ParameterValue_MaximumUpdate:
			rlog.Fatalf("Parameter: '%v': DefaultValue cant be MaxUpdate", detail.Name)
		case *pb.ParameterValue_Png:
			if detail.ValueType != pb.ValueType_PNG {
				rlog.Fatalf("Parameter: '%v': Invalid default value for %s", detail.Name, detail.ValueType)
			}
		case *pb.ParameterValue_Jpeg:
			if detail.ValueType != pb.ValueType_JPEG {
				rlog.Fatalf("Parameter: '%v': Invalid default value for %s", detail.Name, detail.ValueType)
			}
		case *pb.ParameterValue_Error:
			rlog.Fatalf("Parameter: '%v': DefaultValue cant be CustomError", detail.Name)
		}
	}

	// Warnings
	if detail.Description == "" {
		rlog.Warnf("Parameter '%v': No description set", detail.Name)
	}
}

func validateModel(rlog *log.Entry, model *pb.ModelInfo) {
	if model.Name == "" {
		rlog.Fatal("please specify a name for all models")
	}

	if model.Description == "" {
		rlog.Fatal("please specify a description for all models")
	}

	switch model.DevelopmentStatus {
	case "":
		model.DevelopmentStatus = "concept"
	case "hidden":
	case "concept":
	case "beta":
	case "released":
	case "mature":
	default:
		rlog.Fatalf("Model %s: Invalid developmentstatus, valid options are concept,beta,released,mature", model.Name)
	}
}

func (r *IBeamParameterRegistry) validateAllParams() {
	r.muDetail.RLock()

	for mIndex, model := range r.modelInfos {
		// For every model validate
		for _, parameter := range r.parameterDetail[mIndex] {
			if parameter.RecommendedParamForTextDisplay != "" {
				if strings.ContainsAny(parameter.RecommendedParamForTextDisplay, "{}#") {
					validNesting, parts := SplitByBrackets(parameter.RecommendedParamForTextDisplay)
					if !validNesting {
						r.log.Fatalf("Parameter: '%v': RecommendedParamForTextDisplay %q has invalid nesting", parameter.Name, parameter.RecommendedParamForTextDisplay)
					}
					for _, part := range parts {
						if r.PID(part) == 0 {
							r.log.Fatalf("Parameter: '%v': RecommendedParamForTextDisplay %q does not exist on model %q", parameter.Name, part, model.Name)
						}
					}
				} else if r.PID(parameter.RecommendedParamForTextDisplay) == 0 {
					r.log.Fatalf("Parameter: '%v': RecommendedParamForTextDisplay %q does not Xexist on model %q", parameter.Name, parameter.RecommendedParamForTextDisplay, model.Name)
				}
			}

			if parameter.RecommendedParamForTitleDisplay != "" {
				if strings.ContainsAny(parameter.RecommendedParamForTitleDisplay, "{}#") {
					validNesting, parts := SplitByBrackets(parameter.RecommendedParamForTitleDisplay)
					if !validNesting {
						r.log.Fatalf("Parameter: '%v': RecommendedParamForTitleDisplay %q has invalid nesting", parameter.Name, parameter.RecommendedParamForTitleDisplay)
					}
					for _, part := range parts {
						if r.PID(part) == 0 {
							r.log.Fatalf("Parameter: '%v': RecommendedParamForTitleDisplay %q does not exist on model %q", parameter.Name, part, model.Name)
						}
					}
				} else if r.PID(parameter.RecommendedParamForTitleDisplay) == 0 {
					r.log.Fatalf("Parameter: '%v': RecommendedParamForTitleDisplay %q does not exist on model %q", parameter.Name, parameter.RecommendedParamForTitleDisplay, model.Name)
				}
			}
		}
	}
	r.muDetail.RUnlock()
}

func SplitByBrackets(input string) (valid bool, stringsInBraces []string) {
	open := 0
	start := -1
	for i, char := range input {
		switch char {
		case '{':
			if open == 0 {
				start = i
			}
			open++
		case '}':
			open--
			if open == 0 && start != -1 {
				stringsInBraces = append(stringsInBraces, input[start+1:i])
				start = -1
			}
		}
	}

	return open == 0, stringsInBraces
}
