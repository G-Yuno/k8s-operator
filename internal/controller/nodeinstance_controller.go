/*
Copyright 2026.

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
	"fmt"
	"time"

	"github.com/apenella/go-ansible/v2/pkg/execute"
	"github.com/apenella/go-ansible/v2/pkg/execute/result/transformer"
	"github.com/apenella/go-ansible/v2/pkg/playbook"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	configmanagerv1 "yuno.org/nodegroup/api/v1"
)

// NodeInstanceReconciler reconciles a NodeInstance object
type NodeInstanceReconciler struct {
	client.Client
	Scheme                            *runtime.Scheme
	AnsiblePlaybookYamlDir            string
	AnsiblePlaybookExecTimeoutSeconds int
}

// +kubebuilder:rbac:groups=config-manager.yuno.org,resources=nodeinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config-manager.yuno.org,resources=nodeinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config-manager.yuno.org,resources=nodeinstances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NodeInstance object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *NodeInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	nodeInstance := &configmanagerv1.NodeInstance{}
	//return ctrl.Result{}, nil
	// 从集群中获取：到底是index缓存还是直接clientset

	err := r.Get(ctx, req.NamespacedName, nodeInstance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			logger.Error(err, "nodeInstance resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get nodeInstance")
		return ctrl.Result{}, err
	}

	if nodeInstance.Status.Conditions == nil || len(nodeInstance.Status.Conditions) == 0 {

		logger.Info("SetStatusCondition", "name", req.Name)
		meta.SetStatusCondition(&nodeInstance.Status.Conditions, metav1.Condition{Type: typeAvailable, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})

		// 不需要设置 初始状态了：因为是由上层的np设置的
		nodeInstance.Status.Phase = string(configmanagerv1.NodeInstancePhasePendingSync)

		if err = r.Status().Update(ctx, nodeInstance); err != nil {
			logger.Error(err,
				"[type=new]Failed to update nodeInstance status",
				"type",
				"new",
				"name",
				req.Name,
			)
			return ctrl.Result{}, err
		}
		logger.Info("set nodeInstance status success",
			"type",
			"new",
			"name",
			req.Name,
		)
		// Let's re-fetch the nodeInstance Custom Resource after updating the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raising the error "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err = r.Get(ctx, req.NamespacedName, nodeInstance); err != nil {
			logger.Error(err,
				"Failed to re-fetch nodeInstance",
				"type",
				"new",
				"name",
				req.Name,
			)
			return ctrl.Result{}, err
		}
	}

	targetPhase := ""

	// 根据当前的状态进行判断 pendingSync的时候才需要去同步
	switch nodeInstance.Status.Phase {
	case string(configmanagerv1.NodeInstancePhasePendingSync):

		// 开始做的ansible的工作
		// https://github.com/apenella/go-ansible/blob/master/examples/ansibleplaybook-with-timeout/ansibleplaybook-with-timeout.go
		ctxan, cancel := context.WithTimeout(context.Background(), time.Duration(r.AnsiblePlaybookExecTimeoutSeconds)*time.Second)
		defer cancel()

		ansiblePlaybookOptions := &playbook.AnsiblePlaybookOptions{
			User:          "root",
			Connection:    "smart",
			Inventory:     fmt.Sprintf("%v,", nodeInstance.Spec.Ip),
			SSHCommonArgs: "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
		}

		playbookYmlFile := fmt.Sprintf("%s%s.yml", r.AnsiblePlaybookYamlDir, nodeInstance.Spec.NodeGroupName)

		// --ssh-common-args='-o StrictHostKeyChecking=no'
		playbookCmd := playbook.NewAnsiblePlaybookCmd(
			playbook.WithPlaybooks(playbookYmlFile),
			playbook.WithPlaybookOptions(ansiblePlaybookOptions),
		)

		exec := execute.NewDefaultExecute(
			execute.WithCmd(playbookCmd),
			execute.WithErrorEnrich(playbook.NewAnsiblePlaybookErrorEnrich()),
			execute.WithTransformers(
				transformer.Prepend("Go-ansible example"),
			),
		)

		err = exec.Execute(ctxan)
		if err != nil {
			logger.Error(err, "nodeInstance.playbook.exec.failed",
				"np", nodeInstance.Spec.NodeGroupName,
				"ni", nodeInstance.Name,
			)
			targetPhase = string(configmanagerv1.NodeInstancePhaseSyncFailed)
		} else {
			logger.Info("nodeInstance.playbook.exec.success",
				"np", nodeInstance.Spec.NodeGroupName,
				"ni", nodeInstance.Name,
			)
			targetPhase = string(configmanagerv1.NodeInstancePhaseSyncSuccess)
		}
	default:
		return ctrl.Result{}, nil
	}

	if err = r.Get(ctx, req.NamespacedName, nodeInstance); err != nil {
		logger.Error(err,
			"Failed to re-fetch nodeInstance",
			"type",
			"new",
			"name",
			req.Name,
		)
		return ctrl.Result{}, err
	}
	nodeInstance.Status.Phase = targetPhase

	if err = r.Status().Update(ctx, nodeInstance); err != nil {
		logger.Error(err, "Failed to update nodeInstance",
			"ni", req.Name,
			"np", nodeInstance.Spec.NodeGroupName,
		)
		return ctrl.Result{}, err
	}
	//time.Sleep(5 * time.Second)
	logger.Info("after exec success  updated nodeInstance",
		"ni", req.Name,
		"np", nodeInstance.Spec.NodeGroupName,
	)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&configmanagerv1.NodeInstance{}).
		Named("nodeinstance").
		Complete(r)
}
