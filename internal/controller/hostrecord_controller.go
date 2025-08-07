/*
Copyright 2025 nofy.

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

    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    logf "sigs.k8s.io/controller-runtime/pkg/log"

    dnsv1alpha1 "github.com/no-fy/infoblox-operator/api/v1alpha1"
    ibx "github.com/no-fy/infoblox-operator/internal/infoblox"
)

// HostRecordReconciler reconciles a HostRecord object
type HostRecordReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dns.infoblox.example,resources=hostrecords,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.infoblox.example,resources=hostrecords/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.infoblox.example,resources=hostrecords/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HostRecord object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *HostRecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := logf.FromContext(ctx)

    var hr dnsv1alpha1.HostRecord
    if err := r.Get(ctx, req.NamespacedName, &hr); err != nil {
        // Object deleted
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Setup finalizer to support deletion of Infoblox record if desired
    finalizer := "dns.infoblox.example/finalizer"
    if hr.DeletionTimestamp.IsZero() {
        if !containsString(hr.Finalizers, finalizer) {
            hr.Finalizers = append(hr.Finalizers, finalizer)
            if err := r.Update(ctx, &hr); err != nil {
                return ctrl.Result{}, err
            }
        }
    } else {
        // Handle delete
        if containsString(hr.Finalizers, finalizer) && hr.Status.Ref != "" {
            // best-effort deletion
            credNS := hr.Spec.CredentialsSecret.Namespace
            if credNS == "" {
                credNS = hr.Namespace
            }
            username, password, err := r.loadCredentials(ctx, credNS, hr.Spec.CredentialsSecret.Name)
            if err == nil {
                c := ibx.NewClient(hr.Spec.WAPIEndpoint, username, password, hr.Spec.InsecureTLS)
                _ = c.DeleteByRef(ctx, hr.Status.Ref)
            }
            hr.Finalizers = removeString(hr.Finalizers, finalizer)
            if err := r.Update(ctx, &hr); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // Read credentials
    credNS := hr.Spec.CredentialsSecret.Namespace
    if credNS == "" {
        credNS = hr.Namespace
    }
    username, password, err := r.loadCredentials(ctx, credNS, hr.Spec.CredentialsSecret.Name)
    if err != nil {
        logger.Error(err, "failed to load credentials")
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }

    client := ibx.NewClient(hr.Spec.WAPIEndpoint, username, password, hr.Spec.InsecureTLS)

    // Check if already exists
    existing, err := client.GetHostRecord(ctx, hr.Spec.FQDN, hr.Spec.DNSView)
    if err != nil {
        logger.Error(err, "failed to query Infoblox")
        return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
    }

    if existing == nil {
        // Create
        nextAvailable := ""
        if hr.Spec.IP == "" && hr.Spec.NetworkCIDR != "" {
            nextAvailable = hr.Spec.NetworkCIDR
        }
        ref, ip, err := client.CreateHostRecord(ctx, hr.Spec.FQDN, hr.Spec.DNSView, hr.Spec.TTL, hr.Spec.IP, nextAvailable, hr.Spec.ExtAttrs)
        if err != nil {
            logger.Error(err, "failed to create host record")
            return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
        }
        hr.Status.Ref = ref
        if ip != "" {
            hr.Status.AllocatedIP = ip
        }
        if err := r.Status().Update(ctx, &hr); err != nil {
            logger.Error(err, "status update failed")
            return ctrl.Result{Requeue: true}, nil
        }
        return ctrl.Result{}, nil
    }

    // Exists: ensure status is populated
    if hr.Status.Ref == "" {
        hr.Status.Ref = existing.Ref
    }
    if hr.Status.AllocatedIP == "" && len(existing.IPv4Addrs) > 0 {
        hr.Status.AllocatedIP = existing.IPv4Addrs[0].IPv4Addr
    }
    if err := r.Status().Update(ctx, &hr); err != nil {
        logger.Error(err, "status update failed")
        return ctrl.Result{Requeue: true}, nil
    }

    return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HostRecordReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1alpha1.HostRecord{}).
		Named("hostrecord").
		Complete(r)
}

// loadCredentials reads username/password from a Secret
func (r *HostRecordReconciler) loadCredentials(ctx context.Context, namespace, name string) (string, string, error) {
    var s corev1.Secret
    if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &s); err != nil {
        return "", "", err
    }
    userBytes, okU := s.Data["username"]
    passBytes, okP := s.Data["password"]
    if !okU || !okP {
        return "", "", fmt.Errorf("secret %s/%s missing keys 'username' and/or 'password'", namespace, name)
    }
    return string(userBytes), string(passBytes), nil
}

func containsString(list []string, s string) bool {
    for _, item := range list {
        if item == s {
            return true
        }
    }
    return false
}

func removeString(list []string, s string) []string {
    out := make([]string, 0, len(list))
    for _, item := range list {
        if item != s {
            out = append(out, item)
        }
    }
    return out
}
