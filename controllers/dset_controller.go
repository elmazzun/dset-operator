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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dsetv1alpha1 "github.com/user/repo/api/v1alpha1"
)

// DSetReconciler reconciles a DSet object
type DSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dset.example.com,resources=dsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dset.example.com,resources=dsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dset.example.com,resources=dsets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DSet object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *DSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// TODO(user): your logic here

	// Lookup the DSet instance for this reconcile request
	dset := &dsetv1alpha1.DSet{}
	err := r.Get(ctx, req.NamespacedName, dset)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			log.Info("DSet resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.Error(err, "Failed to get DSet")
		return ctrl.Result{}, err
	}
	log.Info("Got DSet instance")

	// Check if the DaemonSet already exists, if not create a new one
	found := &appsv1.DaemonSet{}
	err = r.Get(ctx, types.NamespacedName{Name: dset.Name, Namespace: dset.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		// Create a new DaemonSet
		daemonset, err := r.daemonsetForDSet(dset)
		if err != nil {
			log.Error(err, "Failed to define new DaemonSet resource for DSet")
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
				MatchLabels: map[string]string{
					"daemonset":     dset.Name + "-daemonset-job",
					"job":           dset.Name,
					"daemonset-job": "true",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					// Labels: map[string]string{
					// 	"daemonset":     dset.Name + "-daemonset-job",
					// 	"job":           dset.Name,
					// 	"daemonset-job": "true",
					// },
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "job-container",
							Image: image,
							// Args:  dset.Spec.Args,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "basetarget",
									MountPath: "/basetarget",
								},
							},
						},
					},
					// Volumes: []corev1.Volume{
					// 	{
					// 		Name: "basetarget",
					// 		VolumeSource: corev1.VolumeSource{
					// 			HostPath: &corev1.HostPathVolumeSource{
					// 				Path: dset.Spec.MountPath,
					// 			},
					// 		},
					// 	},
					// },
				},
			},
		},
	}

	return deploy, nil
}
