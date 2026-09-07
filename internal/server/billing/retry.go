package billing

import (
	"context"
	"strings"

	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbbilling"
	"github.com/cloud-fitter/cloud-fitter/gen/idl/pbtenant"
	"github.com/golang/glog"
)

func IsTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "timeout") ||
		strings.Contains(s, "deadline exceeded") ||
		strings.Contains(s, "i/o timeout")
}

// ListSummaryForExport 供批量导出使用：华为云超时错误重试 1 次；其它云不重试。
func ListSummaryForExport(ctx context.Context, req *pbbilling.ListBillingSummaryReq) (*pbbilling.ListBillingSummaryResp, error) {
	resp, err := ListSummary(ctx, req)
	if err == nil {
		return resp, nil
	}
	if req == nil || req.Provider != pbtenant.CloudProvider_huawei || !IsTimeoutErr(err) {
		return nil, err
	}
	glog.Warningf("billing batch-export huawei timeout, retry once account=%s month=%s err=%v",
		req.AccountName, req.BillingCycle, err)
	resp2, err2 := ListSummary(ctx, req)
	if err2 != nil {
		glog.Errorf("billing batch-export huawei retry fail account=%s month=%s err=%v",
			req.AccountName, req.BillingCycle, err2)
		return nil, err2
	}
	glog.Infof("billing batch-export huawei retry ok account=%s month=%s",
		req.AccountName, req.BillingCycle)
	return resp2, nil
}
