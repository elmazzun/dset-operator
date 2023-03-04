/*
Copyright 2023.

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

// https://github.com/operator-framework/operator-sdk/blob/latest/testdata/go/v3/memcached-operator/controllers/memcached_controller.go

package controllers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dsetv1alpha1 "github.com/user/repo/api/v1alpha1"
)

const dsetFinalizer = "dset.example.com/finalizer"

// Definitions to manage status conditions
const (
	// typeAvailableDSet represents the status
	// of the Deployment reconciliation
	typeAvailableDSet = "Available"
	// typeDegradedDSet represents the status used when the custom resource
	// is deleted and the finalizer operations are must to occur.
	typeDegradedDSet = "Degraded"
)

// DSetReconciler reconciles a DSet object
type DSetReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// The following markers are used to generate the rules permissions (RBAC) on
// config/rbac using controller-gen when the command <make manifests> is executed.
//+kubebuilder:rbac:groups=dset.example.com,resources=dsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dset.example.com,resources=dsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dset.example.com,resources=dsets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.

// It is essential for the controller's reconciliation loop to be idempotent.
// By following the Operator pattern you will create Controllers which provide
// a reconcile function responsible for synchronizing resources until the
// desired state is reached on the cluster. Breaking this recommendation goes
// against the design principles of controller-runtime and may lead to
// unforeseen consequences such as resources becoming stuck and requiring
// manual intervention.

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *DSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the DSet instance for this reconcile request
	// The purpose is check if the Custom Resource for the Kind DSet
	// is applied on the cluster; if not we return nil to stop the reconciliation
	dset := &dsetv1alpha1.DSet{}
	err := r.Get(ctx, req.NamespacedName, dset)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means
			// that it was deleted or not created; in this way, we will stop
			// the reconciliation
			log.Info("DSet resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.Error(err, "Failed to get DSet")
		return ctrl.Result{}, err
	}
	log.Info("Got DSet instance")

	// Let's just set the status as Unknown when no status are available
	if dset.Status.Conditions == nil || len(dset.Status.Conditions) == 0 {
		meta.SetStatusCondition(&dset.Status.Conditions, metav1.Condition{
			Type:    typeAvailableDSet,
			Status:  metav1.ConditionUnknown,
			Reason:  "Reconciling",
			Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, dset); err != nil {
			log.Error(err, "Failed to update DSet status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the DSet Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, dset); err != nil {
			log.Error(err, "Failed to re-fetch dset")
			return ctrl.Result{}, err
		}
	}

	// Let's add a finalizer. Then, we can define some operations which should
	// occurs before the custom resource to be deleted.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers
	if !controllerutil.ContainsFinalizer(dset, dsetFinalizer) {
		log.Info("Adding Finalizer for DSet")
		if ok := controllerutil.AddFinalizer(dset, dsetFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, dset); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Check if the DSet instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isDSetMarkedToBeDeleted := dset.GetDeletionTimestamp() != nil
	if isDSetMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(dset, dsetFinalizer) {
			log.Info("Performing Finalizer Operations for DSet before delete CR")

			// Let's add here an status "Downgrade" to define that this resource begin its process to be terminated.
			meta.SetStatusCondition(
				&dset.Status.Conditions,
				metav1.Condition{
					Type:    typeDegradedDSet,
					Status:  metav1.ConditionUnknown,
					Reason:  "Finalizing",
					Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", dset.Name),
				},
			)

			if err := r.Status().Update(ctx, dset); err != nil {
				log.Error(err, "Failed to update DSet status")
				return ctrl.Result{}, err
			}

			// Perform all operations required before remove the finalizer and allow
			// the Kubernetes API to remove the custom resource.
			r.doFinalizerOperationsForDSet(dset)

			// TODO(user): If you add operations to the doFinalizerOperationsForDSet method
			// then you need to ensure that all worked fine before deleting and updating the Downgrade status
			// otherwise, you should requeue here.

			// Re-fetch the dset Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, dset); err != nil {
				log.Error(err, "Failed to re-fetch dset")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(
				&dset.Status.Conditions,
				metav1.Condition{
					Type:   typeDegradedDSet,
					Status: metav1.ConditionTrue, Reason: "Finalizing",
					Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", dset.Name),
				},
			)

			if err := r.Status().Update(ctx, dset); err != nil {
				log.Error(err, "Failed to update DSet status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for DSet after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(dset, dsetFinalizer); !ok {
				log.Error(err, "Failed to remove finalizer for DSet")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, dset); err != nil {
				log.Error(err, "Failed to remove finalizer for DSet")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Check if the DaemonSet already exists, if not create a new one
	found := &appsv1.DaemonSet{}
	err = r.Get(ctx, types.NamespacedName{Name: dset.Name, Namespace: dset.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		// Create a new DaemonSet
		daemonset, err := r.daemonsetForDSet(dset)
		if err != nil {
			log.Error(err, "Failed to define new DaemonSet resource for DSet")
			// The following implementation will update the status
			meta.SetStatusCondition(&dset.Status.Conditions,
				metav1.Condition{
					Type:   typeAvailableDSet,
					Status: metav1.ConditionFalse, Reason: "Reconciling",
					Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", dset.Name, err),
				},
			)

			if err := r.Status().Update(ctx, dset); err != nil {
				log.Error(err, "Failed to update DSet status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		log.Info("Creating a new DaemonSet",
			"DaemonSet.Namespace", daemonset.Namespace,
			"DaemonSet.Name", daemonset.Name)
		if err = r.Create(ctx, daemonset); err != nil {
			log.Error(err, "Failed to create new DaemonSet",
				"DaemonSet.Namespace", daemonset.Namespace,
				"DaemonSet.Name", daemonset.Name)
			return ctrl.Result{}, err
		}
		// DaemonSet created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		log.Info("DaemonSet created successfully")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get DaemonSet")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	if err := r.Status().Update(ctx, dset); err != nil {
		log.Error(err, "Failed to update DSet status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dsetv1alpha1.DSet{}). // DSet is the primary resource to watch
		Owns(&appsv1.DaemonSet{}). // DaemonSet is the secondary resource to watch
		Complete(r)
}

// imageForDset gets the Operand image which is managed by this controller
// from the DSET_IMAGE environment variable defined in the config/manager/manager.yaml
func imageForDset() (string, error) {
	var imageEnvVar = "DSET_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "", fmt.Errorf("Unable to find %s environment variable with the image", imageEnvVar)
	}
	fmt.Printf("Found DSET_IMAGE %s", image)
	return image, nil
}

// labelsForDset returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForDset(name string) map[string]string {
	var imageTag string
	image, err := imageForDset()
	if err == nil {
		imageTag = strings.Split(image, ":")[1]
	}
	return map[string]string{"app.kubernetes.io/name": "DSet",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/part-of":    "dset-operator",
		"app.kubernetes.io/created-by": "controller-manager",
	}
}

// finalizeDSet will perform the required operations before delete the CR.
func (r *DSetReconciler) doFinalizerOperationsForDSet(cr *dsetv1alpha1.DSet) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// Note: It is not recommended to use finalizers with the purpose of delete
	// resources which are created and managed in the reconciliation. These
	// ones, such as the Deployment created on this reconcile, are defined as
	// depended of the custom resource. See that we use the method
	// ctrl.SetControllerReference. to set the ownerRef which means that the
	// Deployment will be deleted by the Kubernetes API.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

// Return a DSet DaemonSet object
func (r *DSetReconciler) daemonsetForDSet(dset *dsetv1alpha1.DSet) (*appsv1.DaemonSet, error) {
	labels := labelsForDset(dset.Name)

	// Get the Operand image
	image, err := imageForDset()
	if err != nil {
		return nil, err
	}

	deploy := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dset.Name,
			Namespace: dset.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						// IMPORTANT: seccomProfile was introduced with Kubernetes 1.19
						// If you are looking for to produce solutions to be supported
						// on lower versions you must remove this option.
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "dset-controller",
							Image:           image,
							ImagePullPolicy: corev1.PullNever,
							// Ensure restrictive context for the container
							// More info: https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
							SecurityContext: &corev1.SecurityContext{
								// WARNING: Ensure that the image used defines an UserID in the Dockerfile
								// otherwise the Pod will not run and will fail with "container has
								// runAsNonRoot and image has non-numeric user"". If you want your workloads
								// admitted in namespaces enforced with the restricted mode in OpenShift/OKD vendors
								// then, you MUST ensure that the Dockerfile defines a User ID OR you MUST
								// leave the "RunAsNonRoot" and "RunAsUser" fields empty.
								RunAsNonRoot: &[]bool{true}[0],
								// The memcached image does not use a non-zero numeric user as the default user.
								// Due to RunAsNonRoot field being set to true, we need to force the user in the
								// container to a non-zero numeric user. We do this using the RunAsUser field.
								// However, if you are looking to provide solution for K8s vendors like OpenShift
								// be aware that you cannot run under its restricted-v2 SCC if you set this value.
								RunAsUser:                &[]int64{1001}[0],
								AllowPrivilegeEscalation: &[]bool{false}[0],
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{
										"ALL",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(dset, deploy, r.Scheme); err != nil {
		return nil, err
	}

	return deploy, nil
}
