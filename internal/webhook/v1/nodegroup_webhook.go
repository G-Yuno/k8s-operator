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

package v1

import (
	"context"
	"fmt"
	"net"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	configmanagerv1 "yuno.org/nodegroup/api/v1"
)

// nolint:unused
// log is for logging in this package.
var nodegrouplog = logf.Log.WithName("nodegroup-resource")

// SetupNodeGroupWebhookWithManager registers the webhook for NodeGroup in the manager.
func SetupNodeGroupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &configmanagerv1.NodeGroup{}).
		WithValidator(&NodeGroupCustomValidator{}).
		WithDefaulter(&NodeGroupCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-config-manager-yuno-org-v1-nodegroup,mutating=true,failurePolicy=fail,sideEffects=None,groups=config-manager.yuno.org,resources=nodegroups,verbs=create;update,versions=v1,name=mnodegroup-v1.kb.io,admissionReviewVersions=v1

// NodeGroupCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind NodeGroup when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type NodeGroupCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind NodeGroup.
func (d *NodeGroupCustomDefaulter) Default(_ context.Context, obj *configmanagerv1.NodeGroup) error {
	nodegrouplog.Info("Defaulting for NodeGroup", "name", obj.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-config-manager-yuno-org-v1-nodegroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=config-manager.yuno.org,resources=nodegroups,verbs=create;update,versions=v1,name=vnodegroup-v1.kb.io,admissionReviewVersions=v1

// NodeGroupCustomValidator struct is responsible for validating the NodeGroup resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NodeGroupCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NodeGroup.
func (v *NodeGroupCustomValidator) ValidateCreate(_ context.Context, obj *configmanagerv1.NodeGroup) (admission.Warnings, error) {
	nodegrouplog.Info("Validation for NodeGroup upon creation", "name", obj.GetName())

	for _, ip := range obj.Spec.Ips {
		address := net.ParseIP(ip)
		if address == nil {
			msg := fmt.Sprintf("ip.wrong:%v", ip)
			nodegrouplog.Info("validate create ip.wrong", "name", obj.Name, "ip", ip)
			return nil, fmt.Errorf(msg)
		}
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NodeGroup.
func (v *NodeGroupCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *configmanagerv1.NodeGroup) (admission.Warnings, error) {
	nodegrouplog.Info("Validation for NodeGroup upon update", "name", newObj.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NodeGroup.
func (v *NodeGroupCustomValidator) ValidateDelete(_ context.Context, obj *configmanagerv1.NodeGroup) (admission.Warnings, error) {
	nodegrouplog.Info("Validation for NodeGroup upon deletion", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
