package ibeamcorelib

import (
	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	spellcheck "github.com/pjnr1/go-aspell-check"
	log "github.com/s00500/env_logger"
)

func validateParameter(rlog *log.Entry, detail *pb.ParameterDetail) {
	// Fatals
	if detail.Name == "" {
		rlog.Fatalf("Parameter: ID %v: No name set", detail.Id)
	}
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
	if detail.ShortLabel != "" && len(detail.ShortLabel) > 11 {
		rlog.Fatalf("Parameter: '%v': ShortLabel is too long: Must not be longer than 11 characters", detail.Name, detail.ShortLabel)
	}
	if detail.Description != "" {
		s, _ := spellcheck.NewSpeller(map[string]string{"lang": "en_US"})
		check := s.CheckWithFeedback(detail.Description)
		if check != "" {
			rlog.Warnln(detail.Description)
			rlog.Warnln(check)
			rlog.Fatalf("Parameter: '%v': Description has misspellings (see above warnings)", detail.Name)
		}
		s.S.Delete() // Take down the aspell-speller
	}
	if detail.ControlStyle != pb.ControlStyle_NoControl && detail.FeedbackStyle != pb.FeedbackStyle_NoFeedback && detail.RetryCount == 0 {
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
		model.DevelopmentStatus = "sandbox"
	case "sandbox":
	case "alpha":
	case "beta":
	case "mature":
	default:
		rlog.Fatal("Model %v: Invalid developmentstatus, valid options are sandbox,alpha,beta,mature")
	}
}
