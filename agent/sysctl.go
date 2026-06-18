package agent

import (
	"encoding/json"
	"fmt"
	"github.com/lorenzosaino/go-sysctl"
	"k8s.io/klog/v2"
)

func (nm *NodeConfigManager) CheckSysctl() (diffMsg string, checkItems string, err error) {
	// 先构建go-sysctl client
	sysctlClient, err := sysctl.NewClient(nm.ProcSysPath)
	if err != nil {
		klog.ErrorS(err, "CheckSysctl.NewClient.err",
			"nm.ProcSysPath", nm.ProcSysPath,
		)
		return
	}
	checkItemsBs, _ := json.Marshal(nm.NodeGroup.Spec.KernelArgsManagers)
	checkItems = string(checkItemsBs)

	// 从 np中获取所有内核参数 遍历get当前值对比
	for _, km := range nm.NodeGroup.Spec.KernelArgsManagers {
		var actualValue string
		actualValue, err = sysctlClient.Get(km.Key)
		if err != nil {
			klog.ErrorS(err, "CheckSysctl.sysctlClient.getKey.err",
				"nm.ProcSysPath", nm.ProcSysPath,
				"key", km.Key,
			)
			return
		}
		if actualValue != km.Value {
			diffMsg = fmt.Sprintf("%v\n[sysctl.key.missmatch][key=%v desired=%v actual=%v]",
				diffMsg,
				km.Key,
				km.Value,
				actualValue,
			)
		}

	}
	return

}
