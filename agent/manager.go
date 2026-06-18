package agent

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	configmanagerv1 "yuno.org/nodegroup/api/v1"
)

type NodeConfigManager struct {
	DpkgFileName            string
	NodeGroup               *configmanagerv1.NodeGroup
	NodeInstance            *configmanagerv1.NodeInstance
	HostIp                  string
	SyncPeriodSeconds       int
	DynamicClient           *dynamic.DynamicClient
	K8sClientTimeOutSeconds int
	ProcSysPath             string
	ContainerHostRootPrefix string
}

func NewNodeConfigManager(dpkgFileName, hostIp, procSysPath, containerHostRootPrefix string, syncPeriodSeconds, k8sClientTimeOutSeconds int, dynamicClient *dynamic.DynamicClient) *NodeConfigManager {

	nm := &NodeConfigManager{
		DpkgFileName:            dpkgFileName,
		DynamicClient:           dynamicClient,
		HostIp:                  hostIp,
		SyncPeriodSeconds:       syncPeriodSeconds,
		K8sClientTimeOutSeconds: k8sClientTimeOutSeconds,
		ProcSysPath:             procSysPath,
		ContainerHostRootPrefix: containerHostRootPrefix,
	}
	return nm
}

func (nm *NodeConfigManager) StartConfigManager(ctx context.Context) error {
	// 每隔 多长时间去执行一下 RunSyncCloudResource ，直到 ctx.Done
	go wait.UntilWithContext(ctx, nm.SyncNpNiDiffNode, time.Duration(nm.SyncPeriodSeconds)*time.Second)
	<-ctx.Done()
	klog.Infof("收到其他任务退出信号 退出")
	return nil

}

// 给一个超时秒数 返回一个 带超时时间的ctx 和 CancelFunc
func (nm *NodeConfigManager) GenTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(nm.K8sClientTimeOutSeconds)*time.Second)
}

func (nm *NodeConfigManager) SyncNpNiDiffNode(ctx context.Context) {

	// 判断节点上托管的对象是否发生变化
	hasdiff := false

	gvkNp := schema.GroupVersionResource{
		Group:    configmanagerv1.GroupVersion.Group,
		Version:  configmanagerv1.GroupVersion.Version,
		Resource: "nodegroups",
	}
	gvkNi := schema.GroupVersionResource{
		Group:    configmanagerv1.GroupVersion.Group,
		Version:  configmanagerv1.GroupVersion.Version,
		Resource: "nodeinstances",
	}

	ctx1, _ := nm.GenTimeoutContext()
	unstructuredList, err := nm.DynamicClient.Resource(gvkNp).List(ctx1, metav1.ListOptions{})
	if err != nil {
		klog.ErrorS(err, "dyclient.list.np.err")
		return
	}

	// 参考argorollout 用dyclient转化crd对象 D:\nyy_work\go_path\src\github.com\argoproj\argo-rollouts\utils\tolerantinformer\tollerantinformer.go
	npFound := false
	for _, unstructuredOne := range unstructuredList.Items {
		if npFound {
			break
		}
		var nps configmanagerv1.NodeGroup
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredOne.Object, &nps)
		klog.InfoS("FromUnstructured", "nps.Spec.ips", nps.Spec.Ips)

		for _, ip := range nps.Spec.Ips {
			if ip == nm.HostIp {

				nm.NodeGroup = &nps
				npFound = true
				break
			}
		}
	}
	if nm.NodeGroup == nil {
		klog.ErrorS(err, "get.NodeGroupSpec.nil")
		return
	}

	// 拿到ni对象
	ctx2, _ := nm.GenTimeoutContext()
	niName := fmt.Sprintf("%v-%v", nm.NodeGroup.Name, nm.HostIp)

	unstructuredNi, err := nm.DynamicClient.Resource(gvkNi).Get(ctx2, niName, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "dyclient.get.ni.err")
		return
	}

	targetNi := &configmanagerv1.NodeInstance{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredNi.Object, targetNi)
	if err != nil {
		klog.ErrorS(err, "dyclient.get.ni.FromUnstructured.err")
		return
	}
	if targetNi == nil {
		klog.ErrorS(err, "dyclient.get.ni.nil.err")
		return
	}

	klog.InfoS("dyclient.get.ni.np.success")
	var diffMsg string
	// 01 处理dpkg
	diffMsgDpkg, checkItemsDpkg, err := nm.CheckPkgs()
	if err != nil {
		klog.ErrorS(err, "dyclient.CheckPkgs.err")
		return
	}
	if diffMsgDpkg != "" {
		klog.InfoS("CheckPkgs.has.diff.", "diffMsgDpkg", diffMsgDpkg)
		hasdiff = true
	} else {
		klog.InfoS("CheckPkgs.no.diff.", "checkItemsDpkg", checkItemsDpkg)

	}
	diffMsg += diffMsgDpkg
	//
	// 02 处理sysctl
	diffMsgSysctl, checkItemsSysctl, err := nm.CheckSysctl()
	if err != nil {
		klog.ErrorS(err, "dyclient.CheckSysctl.err")
		return
	}
	if diffMsgSysctl != "" {
		klog.InfoS("CheckSysctl.has.diff.", "diffMsgSysctl", diffMsgSysctl)
		hasdiff = true
	} else {
		klog.InfoS("CheckSysctl.no.diff.", "checkItemsSysctl", checkItemsSysctl)

	}
	diffMsg += diffMsgSysctl

	// 03 处理文件
	diffMsgFile, checkItemsFile, err := nm.CheckFiles()
	if err != nil {
		klog.ErrorS(err, "dyclient.CheckFiles.err")
		return
	}
	if diffMsgFile != "" {
		klog.InfoS("CheckFiles.has.diff", "diffMsgFile", diffMsgFile)
		hasdiff = true
	} else {
		klog.InfoS("CheckFiles.no.diff.", "checkItemsFile", checkItemsFile)

	}
	diffMsg += diffMsgFile

	// 如果出现diff patch ni对象，状态修改
	if hasdiff {

		ctx3, _ := nm.GenTimeoutContext()
		statusPatchStringTem := `[{"op": "replace","path":"/status/phase","value":"%v"}]`
		statusPatchString := fmt.Sprintf(statusPatchStringTem, configmanagerv1.NodeInstancePhasePendingSync)
		unstructuredNi, err = nm.DynamicClient.Resource(gvkNi).Patch(ctx3, niName, types.JSONPatchType, []byte(statusPatchString), metav1.PatchOptions{}, "status")
		if err != nil {
			klog.ErrorS(err, "config.diff.patch.ni.failed", "diffMsg", diffMsg)
		} else {
			klog.InfoS("config.diff.patch.ni.success", "diffMsg", diffMsg)

		}
	} else {
		klog.InfoS("config.check.success.no.diff")
	}

}
