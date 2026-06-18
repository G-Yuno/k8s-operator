/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"reflect"
	"time"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	configmanagerv1 "yuno.org/nodegroup/api/v1"
)

// Definitions to manage status conditions
const (
	typeAvailable = "Available"
	typeDegraded  = "Degraded"
)

const (
	nodeGroupFinalizer       = "config-manager.yuno.org/finalizer"
	labelManageNodeGroupName = "manageNodeGroupName"
	cmNamespace              = "default"
	cmNamePrefix             = "nodegroup-playbook-cm"
	specWithoutIpsDataKey    = "specWithoutIpsData"
)

// NodeGroupReconciler reconciles a NodeGroup object
type NodeGroupReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	Recorder               record.EventRecorder
	AnsiblePlaybookYamlDir string
}

// +kubebuilder:rbac:groups=config-manager.yuno.org,resources=nodegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config-manager.yuno.org,resources=nodegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config-manager.yuno.org,resources=nodegroups/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;delete;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NodeGroup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *NodeGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ctrl.Log.WithName("NodeGroupReconciler").Info("Reconcile.called", "req", req, "req.NamespacedName", req.NamespacedName)
	nodeGroup := &configmanagerv1.NodeGroup{}
	// 从集群中获取：到底是index缓存还是直接clientset

	err := r.Get(ctx, req.NamespacedName, nodeGroup)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			logger.Error(err, "nodeGroup resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// 到这里说明是网络错误
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get nodeGroup")
		return ctrl.Result{}, err
	}

	// 增删改会触发Reconcile
	// 那么增删改的逻辑都要处理
	// 01 判断新增的逻辑 ：根据Status.Conditions为空判断到的
	if nodeGroup.Status.Conditions == nil || len(nodeGroup.Status.Conditions) == 0 {

		logger.Info("SetStatusCondition", "name", req.Name)
		meta.SetStatusCondition(&nodeGroup.Status.Conditions, metav1.Condition{Type: typeAvailable, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		nodeGroup.Status.InstanceNum = len(nodeGroup.Spec.Ips)
		nodeGroup.Status.Phase = string(configmanagerv1.NodeGroupPhasePendingSync)

		if err = r.Status().Update(ctx, nodeGroup); err != nil {
			logger.Error(err,
				"[type=new]Failed to update nodeGroup status",
				"type",
				"new",
				"name",
				req.Name,
			)
			return ctrl.Result{}, err
		}
		logger.Info("set nodeGroup status success",
			"type",
			"new",
			"name",
			req.Name,
		)
		// Let's re-fetch the nodeGroup Custom Resource after updating the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raising the error "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, nodeGroup); err != nil {
			logger.Error(err,
				"Failed to re-fetch nodeGroup",
				"type",
				"new",
				"name",
				req.Name,
			)
			return ctrl.Result{}, err
		}
	}

	// 添加finalizer 处理删除，级联删除，保护措施
	// Let's add a finalizer. Then, we can define some operations which should
	// occur before the custom resource to be deleted.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers
	// https://github.com/kubernetes-sigs/kubebuilder/blob/master/docs/book/src/getting-started/testdata/project/internal/controller/nodeGroup_controller.go
	if !controllerutil.ContainsFinalizer(nodeGroup, nodeGroupFinalizer) {
		logger.Info("Adding Finalizer", "name", req.Name)
		if ok := controllerutil.AddFinalizer(nodeGroup, nodeGroupFinalizer); !ok {
			logger.Error(err, "Failed to add finalizer into the custom resource", "name", req.Name)
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, nodeGroup); err != nil {
			logger.Error(err, "Failed to update custom resource to add finalizer", "name", req.Name)
			return ctrl.Result{}, err
		}
	}

	cmName := fmt.Sprintf("%v-%v", cmNamePrefix, nodeGroup.Name)

	// 删除的逻辑
	// Check if the nodeGroup instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isMarkedToBeDeleted := nodeGroup.GetDeletionTimestamp() != nil
	if isMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(nodeGroup, nodeGroupFinalizer) {
			logger.Info("Performing Finalizer Operations for nodeGroup before delete CR",
				"name", req.Name,
			)

			// 设置状态为删除中
			nodeGroup.Status.Phase = string(configmanagerv1.NodeGroupPhaseDeleting)

			// Let's add here a status "Downgrade" to reflect that this resource began its process to be terminated.
			meta.SetStatusCondition(&nodeGroup.Status.Conditions, metav1.Condition{Type: typeDegraded,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", nodeGroup.Name)})

			if err := r.Status().Update(ctx, nodeGroup); err != nil {
				logger.Error(err, "Failed to update nodeGroup status", "name", req.Name)
				return ctrl.Result{}, err
			}

			// Perform all operations required before removing the finalizer and allow
			// the Kubernetes API to remove the custom resource.
			err = r.doFinalizerOperationsForNodeGroup(nodeGroup, ctx, cmName)
			if err != nil {
				return ctrl.Result{}, err
			}

			// TODO(user): If you add operations to the doFinalizerOperationsFornodeGroup method
			// then you need to ensure that all worked fine before deleting and updating the Downgrade status
			// otherwise, you should requeue here.

			// Re-fetch the nodeGroup Custom Resource before updating the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raising the error "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, nodeGroup); err != nil {
				logger.Error(err, "Failed to re-fetch nodeGroup", "name", req.Name)
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&nodeGroup.Status.Conditions, metav1.Condition{Type: typeDegraded,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", nodeGroup.Name)})

			if err := r.Status().Update(ctx, nodeGroup); err != nil {
				logger.Error(err, "Failed to update nodeGroup status", "name", req.Name)
				return ctrl.Result{}, err
			}

			logger.Info("Removing Finalizer for nodeGroup after successfully perform the operations", "name", req.Name)
			// 做完级联删除后去掉这个finalizer
			if ok := controllerutil.RemoveFinalizer(nodeGroup, nodeGroupFinalizer); !ok {
				logger.Error(err, "Failed to remove finalizer for nodeGroup", "name", req.Name)
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, nodeGroup); err != nil {
				logger.Error(err, "Failed to remove finalizer for nodeGroup", "name", req.Name)
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	labelMap := map[string]string{
		labelManageNodeGroupName: nodeGroup.Name,
	}
	// 生成ansible-playbook yaml
	// 写入configmap

	yamlBs, _ := r.createOrUpdateAnsiblePlaybook(ctx, nodeGroup)

	// 写入yaml文件了
	playbookYmlFile := fmt.Sprintf("%s%s.yml", r.AnsiblePlaybookYamlDir, nodeGroup.Name)
	if err = os.MkdirAll(r.AnsiblePlaybookYamlDir, 0755); err != nil {
		logger.Error(err, "Failed to create playbook dir",
			"name", req.Name,
			"dir", r.AnsiblePlaybookYamlDir,
		)
		return ctrl.Result{}, err
	}
	err = os.WriteFile(playbookYmlFile, yamlBs, 0644)
	if err != nil {
		logger.Error(err, "Failed write playbook yml",
			"name", req.Name,
			"path", playbookYmlFile,
		)
		return ctrl.Result{}, err
	}

	// 新建或更新到configmap中

	cmObj := &corev1.ConfigMap{}
	cmNamespaceName := types.NamespacedName{
		Namespace: cmNamespace,
		Name:      cmName,
	}
	cmObj.Name = cmName
	cmObj.Namespace = cmNamespace
	cmObj.Labels = labelMap
	cmObj.Data = map[string]string{}
	cmObj.Data["playbook.yml"] = string(yamlBs)

	err = r.Get(ctx, cmNamespaceName, cmObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// 没有就创建
			err = r.Create(ctx, cmObj)
			if err != nil {
				logger.Error(err, "nodegroup.playbook-cm.create.failed",
					"np", nodeGroup.Name,
				)
				return ctrl.Result{}, err
			}
			logger.Info("nodegroup.playbook-cm.create.success",
				"np", nodeGroup.Name,
			)
		} else {
			// 到这里说明是网络错误
			logger.Error(err, "nodegroup.playbook-cm.get.failed",
				"np", nodeGroup.Name,
			)
			return ctrl.Result{}, err
		}
	} else {
		// 到这里说明要更新了
		cmObj.Data["playbook.yml"] = string(yamlBs)
		err = r.Update(ctx, cmObj)
		if err != nil {
			logger.Error(err, "nodegroup.playbook-cm.update.failed",
				"np", nodeGroup.Name,
			)
			return ctrl.Result{}, err
		} else {
			logger.Info("nodegroup.playbook-cm.update.success",
				"np", nodeGroup.Name,
			)
		}
	}

	// 这里需要获取这个 np存量的节点对象，diff
	// 不在本次spec.ips中的说明删除了

	var nodeInstances configmanagerv1.NodeInstanceList
	err = r.List(ctx, &nodeInstances, client.MatchingLabels(labelMap))
	if err != nil {
		logger.Error(err, "Failed to list instance by nodegroup",
			"np", req.Name,
		)
		return ctrl.Result{}, err
	}
	logger.Info("success to list instance by nodegroup",
		"np", req.Name,
		"num", len(nodeInstances.Items),
	)

	// 根据list结果准备1个map 作为下面的ips判断逻辑
	nodeInstancesMap := map[string]*configmanagerv1.NodeInstance{}
	syncSuccessNodeNum := 0
	syncFailedNodeNum := 0
	pendingSyncNodeNum := 0

	for _, ins := range nodeInstances.Items {
		switch ins.Status.Phase {
		case string(configmanagerv1.NodeInstancePhaseSyncSuccess):
			syncSuccessNodeNum++
		case string(configmanagerv1.NodeInstancePhaseSyncFailed):
			syncFailedNodeNum++
		case string(configmanagerv1.NodeInstancePhasePendingSync):
			pendingSyncNodeNum++
		}
		nodeInstancesMap[ins.Spec.Ip] = &ins
	}

	thisIpMap := map[string]string{}

	var (
		specWithoutIpsChanged bool
	)
	specWithoutIpsData, specExists := nodeGroup.Annotations[specWithoutIpsDataKey]
	thisNodeGroupSpec := nodeGroup.Spec
	thisNodeGroupSpec.Ips = []string{}
	if specExists {
		oldNodeGroupSpec := &configmanagerv1.NodeGroupSpec{}
		json.Unmarshal([]byte(specWithoutIpsData), oldNodeGroupSpec)

		// 使用reflect.DeepEqual判断前后是否相等，底层为反射
		// 如果相等则提前退出，因为不会走到这里，除非代码有问题
		if !reflect.DeepEqual(&thisNodeGroupSpec, oldNodeGroupSpec) {

			specWithoutIpsChanged = true
			logger.Info("specWithoutIpsChanged",
				"np", req.Name,
				"old", oldNodeGroupSpec,
				"new", thisNodeGroupSpec,
			)
		}
	}

	// 处理真正的新增和修改了
	// 逻辑：遍历ips创建或者更新nodeInstance对象
	for _, ip := range nodeGroup.Spec.Ips {
		thisIpMap[ip] = ip
		ins := &configmanagerv1.NodeInstance{}
		ins.Spec.Ip = ip
		ins.Spec.NodeGroupName = nodeGroup.Name
		ins.Status.Phase = string(configmanagerv1.NodeInstancePhasePendingSync)

		ins.SetLabels(labelMap)

		ins.Name = fmt.Sprintf("%s-%s", nodeGroup.Name, ip)
		ins.Namespace = nodeGroup.Namespace

		insOnK8s, ipExists := nodeInstancesMap[ins.Spec.Ip]

		if !ipExists {
			// 没有就创建
			err = r.Create(ctx, ins)
			if err != nil {
				logger.Error(err, "nodeInstance.create.err",

					"np", req.Name,
					"instance", ins.Name,
				)
				return ctrl.Result{}, err

			}
			logger.Info("nodeInstance.create.success",

				"np", req.Name,
				"instance", ins.Name,
			)
			continue
		}
		//"Reconciler error" err="nodeinstances.config-manager.yuno.org \"nodegroup-sa
		//mple-1.1\" is invalid: metadata.resourceVersion: Invalid value: 0x0: must be specified for an update" controller="nodegroup" co
		//ntrollerGroup="config-manager.yuno.org" controllerKind="NodeGroup" NodeGroup="nodegroup-sample" namespace="" name="nodegroup-
		//sample" reconcileID="d9360c6f-3370-4195-990d-368d0f2bea3a"

		if specWithoutIpsChanged {
			ins.ResourceVersion = insOnK8s.ResourceVersion
			if err = r.Status().Update(ctx, ins); err != nil {
				logger.Error(err, "Failed to update instance",
					"np", req.Name,
					"instance", ins.Name)
				return ctrl.Result{}, err
			}
			logger.Info("nodeInstance.update.success",

				"np", req.Name,
				"instance", ins.Name,
			)
		}

	}

	for ip, ins := range nodeInstancesMap {
		_, exists := thisIpMap[ip]
		if !exists {

			err = r.Delete(ctx, ins)
			if err != nil {
				logger.Error(err, "nodegroup.delete.expired.nodeInstance.failed",
					"np", req.Name,
					"instance", ins.Name,
				)
				return ctrl.Result{}, err
			}
			logger.Error(err, "nodegroup.delete.expired.nodeInstance.success",
				"np", req.Name,
				"instance", ins.Name,
			)

		}
	}

	thisNodeGroupSpecData, _ := json.Marshal(thisNodeGroupSpec)

	if err = r.Get(ctx, req.NamespacedName, nodeGroup); err != nil {
		logger.Error(err,
			"Failed to re-fetch nodeGroup",
			"type",
			"new",
			"name",
			req.Name,
		)
		return ctrl.Result{}, err
	}
	// 3. 关联 Annotations
	if nodeGroup.Annotations != nil {
		nodeGroup.Annotations[specWithoutIpsDataKey] = string(thisNodeGroupSpecData)
	} else {
		nodeGroup.Annotations = map[string]string{specWithoutIpsDataKey: string(thisNodeGroupSpecData)}
	}

	// 再这里就可以更新 status
	if err = r.Update(ctx, nodeGroup); err != nil {
		logger.Error(err, "Failed to update np.syncNum",
			"np", req.Name)
		return ctrl.Result{}, err
	}

	logger.Info("[501]update.np.success", "np", req.Name,
		"npall", nodeGroup.Status,
		"syncFailedNodeNum", syncFailedNodeNum,
		"syncSuccessNodeNum", syncSuccessNodeNum,
		"pendingSyncNodeNum", pendingSyncNodeNum,
		"nodeGroup.Status.Phase", nodeGroup.Status.Phase,
	)

	if err = r.Get(ctx, req.NamespacedName, nodeGroup); err != nil {
		logger.Error(err,
			"Failed to re-fetch nodeGroup",
			"type",
			"new",
			"name",
			req.Name,
		)
		return ctrl.Result{}, err
	}
	nodeGroup.Status.SyncFailedNodeNum = syncFailedNodeNum
	nodeGroup.Status.SyncSuccessNodeNum = syncSuccessNodeNum
	nodeGroup.Status.PendingSyncNodeNum = pendingSyncNodeNum

	if syncSuccessNodeNum+syncFailedNodeNum >= nodeGroup.Status.InstanceNum && !specWithoutIpsChanged {
		nodeGroup.Status.Phase = string(configmanagerv1.NodeGroupPhaseSynced)
	}
	if specWithoutIpsChanged {
		nodeGroup.Status.Phase = string(configmanagerv1.NodeGroupPhasePendingSync)
	}

	if err = r.Status().Update(ctx, nodeGroup); err != nil {
		logger.Error(err, "Failed to update np.status",
			"np", req.Name)
		return ctrl.Result{}, err
	}
	logger.Info("[502]update.np.status.success", "np", req.Name)

	switch nodeGroup.Status.Phase {
	case string(configmanagerv1.NodeGroupPhaseSynced):
		//	 不在需要调谐了
		return ctrl.Result{}, nil
	case string(configmanagerv1.NodeGroupPhasePendingSync):
		//	 不在需要调谐了
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	}

	//
	// TODO(user): your logic here
	return ctrl.Result{}, nil
}

// 生成playbook的yaml
func (r *NodeGroupReconciler) createOrUpdateAnsiblePlaybook(ctx context.Context, cr *configmanagerv1.NodeGroup) ([]byte, error) {
	plays := []yaml.MapSlice{}
	play := yaml.MapSlice{}
	play = append(play, yaml.MapItem{
		Key:   "name",
		Value: "manage-node",
	})
	play = append(play, yaml.MapItem{
		Key:   "hosts",
		Value: "all",
	})

	play = append(play, yaml.MapItem{
		Key:   "user",
		Value: "root",
	})
	play = append(play, yaml.MapItem{
		Key:   "gather_facts",
		Value: false,
	})

	// 准备变量
	vars := yaml.MapSlice{}
	vars = append(vars, yaml.MapItem{
		Key:   "ansible_python_interpreter",
		Value: "/usr/bin/python3",
	})

	hasPackageManagers := false
	hasKernelArgsManagers := false
	hasPostRestartServices := false

	// 软件包托管
	if cr.Spec.PackageManagers != nil && len(cr.Spec.PackageManagers) > 0 {

		hasPackageManagers = true
		pkgs := []string{}
		for _, pm := range cr.Spec.PackageManagers {
			pkgs = append(pkgs, fmt.Sprintf("%v=%v", pm.Name, pm.Version))
		}

		vars = append(vars, yaml.MapItem{
			Key:   "pkgs",
			Value: pkgs,
		})
	}

	// 内核参数托管
	if cr.Spec.KernelArgsManagers != nil && len(cr.Spec.KernelArgsManagers) > 0 {
		hasKernelArgsManagers = true
		kmap := map[string]interface{}{}
		for _, km := range cr.Spec.KernelArgsManagers {
			kmap[km.Key] = km.Value
		}
		vars = append(vars, yaml.MapItem{
			Key:   "sysctl_config",
			Value: kmap,
		})
	}
	// 服务托管
	if cr.Spec.PostRestartServices != nil && len(cr.Spec.PostRestartServices) > 0 {
		hasPostRestartServices = true
		vars = append(vars, yaml.MapItem{
			Key:   "services",
			Value: cr.Spec.PostRestartServices,
		})
	}

	// 把环境变量塞入
	play = append(play, yaml.MapItem{
		Key:   "vars",
		Value: vars,
	})

	tasks := []yaml.MapSlice{}

	taskPkg := yaml.MapSlice{}
	taskPkg = append(taskPkg,
		yaml.MapItem{
			Key:   "name",
			Value: "step01 sync packages",
		},
		yaml.MapItem{
			Key: "apt",
			Value: map[string]string{
				"name": `{{ pkgs }}`,
			},
		},
	)

	taskSysctl := yaml.MapSlice{}

	taskSysctl = append(taskSysctl,
		yaml.MapItem{
			Key:   "name",
			Value: "step02 Change various sysctl-settings",
		},

		yaml.MapItem{
			Key: "sysctl",
			Value: map[string]string{
				"name":         `{{ item.key }}`,
				"value":        `{{ item.value }}`,
				"sysctl_set":   "yes",
				"state":        "present",
				"reload":       "yes",
				"ignoreerrors": "yes",
			},
		},
		yaml.MapItem{
			Key:   "with_dict",
			Value: `{{ sysctl_config }}`,
		},
	)

	// 文件的任务不固定

	for _, fm := range cr.Spec.FileManagers {
		taskFile := yaml.MapSlice{}
		taskFile = append(taskFile,
			yaml.MapItem{
				Key:   "name",
				Value: fmt.Sprintf("Ansible | Creating a file with content :%v", fm.Path),
			},
			yaml.MapItem{
				Key: "copy",
				Value: map[string]string{
					"dest":    fm.Path,
					"mode":    fm.Mode,
					"content": fm.Content,
				},
			},
		)
		tasks = append(tasks, taskFile)

	}

	taskService := yaml.MapSlice{}
	/*
	   - name: restart service
	     systemd:
	       name: "{{ item }}"
	       state: restarted
	       daemon_reload: yes
	       enabled: yes
	     with_items:
	       - '{{ services }}'
	*/
	taskService = append(taskService,
		yaml.MapItem{
			Key:   "name",
			Value: "step04 restart service",
		},

		yaml.MapItem{
			Key: "systemd",
			Value: map[string]interface{}{
				"name":          `{{ item }}`,
				"state":         "restarted",
				"daemon_reload": "yes",
				"enabled":       "yes",
			},
		},
		yaml.MapItem{
			Key: "with_items",
			Value: []string{
				`{{ services }}`,
			},
		},
	)

	if hasPackageManagers {
		tasks = append(tasks, taskPkg)

	}
	if hasKernelArgsManagers {
		tasks = append(tasks, taskSysctl)

	}

	if hasPostRestartServices {
		tasks = append(tasks, taskService)

	}

	play = append(play, yaml.MapItem{
		Key:   "tasks",
		Value: tasks,
	})

	plays = append(plays, play)
	return yaml.Marshal(plays)

}

// finalizeMemcached will perform the required operations before delete the CR.
// 级联删除处理 清理回收的函数
// nodegroup级联删除的时候  处理 nodeInstance 在这里做
//

func (r *NodeGroupReconciler) doFinalizerOperationsForNodeGroup(cr *configmanagerv1.NodeGroup, ctx context.Context, cmName string) error {

	logger := ctrl.Log
	ctrl.Log.Info("doFinalizerOperationsForNodeGroup ", "name", cr.Name)
	for _, ip := range cr.Spec.Ips {

		instanceName := fmt.Sprintf("%s-%s", cr.Name, ip)
		instanceNamespaceName := types.NamespacedName{
			Namespace: "/",
			Name:      instanceName,
		}
		ins := &configmanagerv1.NodeInstance{}

		err := r.Get(ctx, instanceNamespaceName, ins)
		if err == nil {
			err = r.Delete(ctx, ins)
			if err != nil {
				logger.Error(err, "nodegroup.gc.delete.instance.failed",
					"np", cr.Name,
					"ins", ins.Name,
				)
				return err
			}
			logger.Info("nodegroup.gc.delete.instance.success",
				"np", cr.Name,
				"ins", ins.Name,
			)
			continue
		}

	}

	cmObj := &corev1.ConfigMap{}
	cmNamespaceName := types.NamespacedName{
		Namespace: cmNamespace,
		Name:      cmName,
	}
	err := r.Get(ctx, cmNamespaceName, cmObj)
	if err == nil {
		err = r.Delete(ctx, cmObj)
		if err != nil {
			logger.Error(err, "nodegroup.gc.delete.cm.failed",
				"np", cr.Name,
				"cm", cmName,
			)
			return err
		}
		logger.Info("nodegroup.gc.delete.cm.success",
			"np", cr.Name,
			"cmName", cmName,
		)
	}

	time.Sleep(5 * time.Second)

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&configmanagerv1.NodeGroup{}).
		Complete(r)
}
