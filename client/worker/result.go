package worker

import (
	"context"

	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
)

type Result interface {
	isResult()
}

type OkResult struct {
	Data []byte
}

type ErrorResult struct {
	Data []byte
}

func (OkResult) isResult()    {}
func (ErrorResult) isResult() {}

type HandleFunc func(ctx context.Context, payload []byte) (Result, error)

func ResolveReport(result Result, err error) (workerv1.ReportAction, []byte) {
	if result != nil {
		switch r := result.(type) {
		case OkResult:
			return workerv1.ReportAction_SUCCESS, r.Data
		case ErrorResult:
			return workerv1.ReportAction_FAIL, r.Data
		}
	}
	if err != nil {
		return workerv1.ReportAction_FAIL, []byte(err.Error())
	}
	if result == nil {
		return workerv1.ReportAction_SUCCESS, nil
	}
	return workerv1.ReportAction_FAIL, []byte("unknown result")
}
