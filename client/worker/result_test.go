package worker

import (
	"errors"
	"testing"

	workerv1 "github.com/gonotelm-lab/flow/api/worker/v1"
	"github.com/stretchr/testify/require"
)

type fakeResult struct{}

func (fakeResult) isResult() {}

func TestResolveReport_OkResult(t *testing.T) {
	action, payload := ResolveReport(OkResult{Data: []byte("ok")}, nil)
	require.Equal(t, workerv1.ReportAction_SUCCESS, action)
	require.Equal(t, []byte("ok"), payload)
}

func TestResolveReport_OkResultNilData(t *testing.T) {
	action, payload := ResolveReport(OkResult{}, nil)
	require.Equal(t, workerv1.ReportAction_SUCCESS, action)
	require.Nil(t, payload)
}

func TestResolveReport_ErrorResult(t *testing.T) {
	action, payload := ResolveReport(ErrorResult{Data: []byte("boom")}, nil)
	require.Equal(t, workerv1.ReportAction_FAIL, action)
	require.Equal(t, []byte("boom"), payload)
}

func TestResolveReport_ErrorResultNilData(t *testing.T) {
	action, payload := ResolveReport(ErrorResult{}, nil)
	require.Equal(t, workerv1.ReportAction_FAIL, action)
	require.Nil(t, payload)
}

func TestResolveReport_NilResultWithError(t *testing.T) {
	action, payload := ResolveReport(nil, errors.New("fail"))
	require.Equal(t, workerv1.ReportAction_FAIL, action)
	require.Equal(t, []byte("fail"), payload)
}

func TestResolveReport_NilResultNilError(t *testing.T) {
	action, payload := ResolveReport(nil, nil)
	require.Equal(t, workerv1.ReportAction_SUCCESS, action)
	require.Nil(t, payload)
}

func TestResolveReport_UnknownResultWithError(t *testing.T) {
	action, payload := ResolveReport(fakeResult{}, errors.New("wrapped"))
	require.Equal(t, workerv1.ReportAction_FAIL, action)
	require.Equal(t, []byte("wrapped"), payload)
}

func TestResolveReport_UnknownResultNilError(t *testing.T) {
	action, payload := ResolveReport(fakeResult{}, nil)
	require.Equal(t, workerv1.ReportAction_FAIL, action)
	require.Equal(t, []byte("unknown result"), payload)
}
