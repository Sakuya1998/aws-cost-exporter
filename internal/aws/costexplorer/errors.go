package costexplorer

import awscommon "github.com/sakuya1998/aws-cost-exporter/internal/aws/common"

type ErrorKind = awscommon.ErrorKind

const (
	ErrorCanceled      = awscommon.ErrorCanceled
	ErrorTimeout       = awscommon.ErrorTimeout
	ErrorThrottle      = awscommon.ErrorThrottle
	ErrorAuthorization = awscommon.ErrorAuthorization
	ErrorValidation    = awscommon.ErrorValidation
	ErrorTransient     = awscommon.ErrorTransient
	ErrorUnknown       = awscommon.ErrorUnknown
)

type ClassifiedError = awscommon.ClassifiedError

func ClassifyError(err error) *ClassifiedError { return awscommon.ClassifyError(err) }
