package memberroll

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/maistra/istio-operator/pkg/apis/maistra/v1"
	"github.com/maistra/istio-operator/pkg/controller/common"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_servicemeshmemberroll")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ServiceMeshMemberRoll Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMemberList{ResourceManager: common.ResourceManager{Client: mgr.GetClient(), Log: log}, scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("servicemeshmemberroll-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ServiceMeshMemberRoll
	err = c.Watch(&source.Kind{Type: &v1.ServiceMeshMemberRoll{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO: should this be moved somewhere else?
	err = mgr.GetFieldIndexer().IndexField(&v1.ServiceMeshMemberRoll{}, "spec.members", func(obj runtime.Object) []string {
		roll := obj.(*v1.ServiceMeshMemberRoll)
		return roll.Spec.Members
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Namespace{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(ns handler.MapObject) []reconcile.Request {
			list := &v1.ServiceMeshMemberRollList{}
			err := mgr.GetClient().List(context.TODO(), client.MatchingField("spec.members", ns.Meta.GetName()), list)
			if err != nil {
				log.Error(err, "Could not list ServiceMeshMemberRolls")
			}

			var requests []reconcile.Request
			for _, pod := range list.Items {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      pod.Name,
						Namespace: pod.Namespace,
					},
				})
			}
			return requests
		}),
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileMemberList{}

// ReconcileMemberList reconciles a ServiceMeshMemberRoll object
type ReconcileMemberList struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	common.ResourceManager
	scheme *runtime.Scheme
}

const (
	finalizer = "istio-operator-MemberRoll"
)

// Reconcile reads that state of the cluster for a ServiceMeshMemberRoll object and makes changes based on the state read
// and what is in the ServiceMeshMemberRoll.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileMemberList) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Processing ServiceMeshMemberRoll")

	// Fetch the ServiceMeshMemberRoll instance
	instance := &v1.ServiceMeshMemberRoll{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) || errors.IsGone(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object
		return reconcile.Result{}, err
	}

	meshList := &v1.ServiceMeshControlPlaneList{}
	err = r.Client.List(context.TODO(), client.InNamespace(instance.Namespace), meshList)
	if err != nil {
		return reconcile.Result{}, err
	}
	if len(meshList.Items) != 1 {
		reqLogger.Error(nil, "cannot reconcile ServiceMeshControlPlane: multiple ServiceMeshControlPlane resources exist in project")
		return reconcile.Result{}, fmt.Errorf("failed to locate single ServiceMeshControlPlane for project %s", instance.Namespace)
	}

	mesh := meshList.Items[0]

	deleted := instance.GetDeletionTimestamp() != nil
	finalizers := instance.GetFinalizers()
	finalizerIndex := common.IndexOf(finalizers, finalizer)
	if deleted {
		if finalizerIndex < 0 {
			return reconcile.Result{}, nil
		}
		reqLogger.Info("Deleting ServiceMeshMemberRoll")
		for _, namespace := range instance.Spec.Members {
			err := r.removeNamespaceFromMesh(namespace, instance.Namespace, reqLogger)
			if err != nil && !(errors.IsNotFound(err) || errors.IsGone(err)) {
				reqLogger.Error(err, "error cleaning up mesh member namespace")
				// XXX: do we prevent removing the finalizer?
			}
		}
		// XXX: for now, nuke the resources, regardless of errors
		finalizers = append(finalizers[:finalizerIndex], finalizers[finalizerIndex+1:]...)
		instance.SetFinalizers(finalizers)
		_ = r.Client.Update(context.TODO(), instance)
		return reconcile.Result{}, nil
	} else if finalizerIndex < 0 {
		reqLogger.V(1).Info("Adding finalizer", "finalizer", finalizer)
		finalizers = append(finalizers, finalizer)
		instance.SetFinalizers(finalizers)
		// add owner reference to the mesh so we can clean up if the mesh gets deleted
		owner := metav1.NewControllerRef(&mesh, v1.SchemeGroupVersion.WithKind("ServiceMeshControlPlane"))
		instance.SetOwnerReferences([]metav1.OwnerReference{*owner})
		err = r.Client.Update(context.TODO(), instance)
		return reconcile.Result{Requeue: err == nil}, err
	}

	reqLogger.Info("Reconciling ServiceMeshMemberRoll")

	if mesh.GetGeneration() == 0 {
		// wait for the mesh to be installed
		return reconcile.Result{}, nil
	}

	allErrors := []error{}
	if instance.Generation != instance.Status.ObservedGeneration {
		// setup projects
		namespaces, err := common.FetchMeshResources(r.Client, corev1.SchemeGroupVersion.WithKind("Namespace"), mesh.Namespace, "")
		if err != nil {
			reqLogger.Error(err, "error listing mesh member namespaces")
			return reconcile.Result{}, err
		}
		requiredNamespaces := toSet(instance.Spec.Members)
		existingNamespaces := nameSet(namespaces.Items)
		for namespaceToRemove := range difference(existingNamespaces, requiredNamespaces) {
			err = r.removeNamespaceFromMesh(namespaceToRemove, mesh.Namespace, reqLogger)
			if err != nil {
				allErrors = append(allErrors, err)
			}
		}
		for namespaceToReconcile := range requiredNamespaces {
			err = r.reconcileNamespaceInMesh(namespaceToReconcile, &mesh, reqLogger)
			if err != nil {
				allErrors = append(allErrors, err)
			}
		}
	} else if mesh.GetGeneration() != instance.Status.ServiceMeshGeneration {
		for namespaceToReconcile := range toSet(instance.Spec.Members) {
			err = r.reconcileNamespaceInMesh(namespaceToReconcile, &mesh, reqLogger)
			if err != nil {
				allErrors = append(allErrors, err)
			}
		}
	} else {
		// nothing to do
		return reconcile.Result{}, nil
	}

	err = utilerrors.NewAggregate(allErrors)
	if err != nil {
		instance.Status.ObservedGeneration = instance.GetGeneration()
		instance.Status.ServiceMeshGeneration = mesh.GetGeneration()
		err = r.Client.Status().Update(context.TODO(), instance)
	}

	return reconcile.Result{}, err
}

func (r *ReconcileMemberList) removeNamespaceFromMesh(namespace string, meshNamespace string, reqLogger logr.Logger) error {
	logger := reqLogger.WithValues("namespace", namespace)
	logger.Info("cleaning up resources in namespace removed from mesh")

	// get namespace
	namespaceResource := &unstructured.Unstructured{}
	namespaceResource.SetAPIVersion(corev1.SchemeGroupVersion.String())
	namespaceResource.SetKind("Namespace")
	err := r.Client.Get(context.TODO(), client.ObjectKey{Name: namespace}, namespaceResource)
	if err != nil {
		if errors.IsNotFound(err) || errors.IsGone(err) {
			logger.Error(nil, "namespace to remove from mesh is missing")
			return nil
		}
		logger.Error(err, "error retrieving namespace to remove from mesh")
		return err
	}

	allErrors := []error{}

	// XXX: Disable for now.  This should not be required when using CNI plugin
	// remove service accounts from SCC
	// saList, err := common.FetchMeshResources(r.Client, corev1.SchemeGroupVersion.WithKind("ServiceAccount"), meshNamespace, namespace)
	// if err == nil {
	// 	saNames := nameList(saList.Items)
	// 	err = r.RemoveUsersFromSCC("anyuid", saNames...)
	// 	if err != nil {
	// 		logger.Error(err, "error removing ServiceAccounts associated with mesh from anyuid SecurityContextConstraints", "ServiceAccounts", saNames)
	// 		allErrors = append(allErrors, err)
	// 	}
	// 	err = r.RemoveUsersFromSCC("privileged", saNames...)
	// 	if err != nil {
	// 		logger.Error(err, "error removing ServiceAccounts associated with mesh from privileged SecurityContextConstraints", "ServiceAccounts", saNames)
	// 		allErrors = append(allErrors, err)
	// 	}
	// } else {
	// 	logger.Error(err, "error could not retrieve ServiceAccounts associated with mesh")
	// 	allErrors = append(allErrors, err)
	// }

	// delete role bindings
	rbList, err := common.FetchMeshResources(r.Client, rbacv1.SchemeGroupVersion.WithKind("RoleBinding"), meshNamespace, namespace)
	if err == nil {
		for _, rb := range rbList.Items {
			err = r.Client.Delete(context.TODO(), &rb)
			if err != nil {
				logger.Error(err, "error removing RoleBinding associated with mesh", "RoleBinding", rb.GetName())
				allErrors = append(allErrors, err)
			}
		}
	} else {
		logger.Error(err, "error could not retrieve RoleBindings associated with mesh")
		allErrors = append(allErrors, err)
	}

	// delete network policies

	// remove mesh labels
	common.DeleteLabel(namespaceResource, common.MemberOfKey)
	common.DeleteLabel(namespaceResource, common.LegacyMemberOfKey)
	err = r.Client.Update(context.TODO(), namespaceResource)
	if err != nil {
		logger.Error(err, "error member-of label from member namespace")
		allErrors = append(allErrors, err)
	}

	return utilerrors.NewAggregate(allErrors)
}

func (r *ReconcileMemberList) reconcileNamespaceInMesh(namespace string, mesh *v1.ServiceMeshControlPlane, reqLogger logr.Logger) error {
	logger := reqLogger.WithValues("namespace", namespace)
	logger.Info("configuring namespace for use with mesh")

	// get namespace
	namespaceResource := &unstructured.Unstructured{}
	namespaceResource.SetAPIVersion(corev1.SchemeGroupVersion.String())
	namespaceResource.SetKind("Namespace")
	err := r.Client.Get(context.TODO(), client.ObjectKey{Name: namespace}, namespaceResource)
	if err != nil {
		if errors.IsNotFound(err) || errors.IsGone(err) {
			logger.Error(nil, "namespace to configure with mesh is missing")
			return nil
		}
		logger.Error(err, "error retrieving namespace to configure with mesh")
		return err
	}

	allErrors := []error{}

	// add network policies

	// add role bindings
	err = r.reconcileRoleBindings(namespace, mesh, logger)
	if err != nil {
		allErrors = append(allErrors, err)
	}

	// XXX: Disable for now.  This should not be required when using CNI plugin
	// add service accounts to SCC
	// err = r.reconcilePodServiceAccounts(namespace, mesh, logger)
	// if err != nil {
	// 	allErrors = append(allErrors, err)
	// }

	// add mesh labels
	if !common.HasLabel(namespaceResource, common.MemberOfKey) {
		common.SetLabel(namespaceResource, common.MemberOfKey, mesh.Namespace)
		common.SetLabel(namespaceResource, common.LegacyMemberOfKey, mesh.Namespace)
		err = r.Client.Update(context.TODO(), namespaceResource)
		if err != nil {
			allErrors = append(allErrors, err)
		}
	}

	return utilerrors.NewAggregate(allErrors)
}

func (r *ReconcileMemberList) reconcileRoleBindings(namespace string, mesh *v1.ServiceMeshControlPlane, reqLogger logr.Logger) error {
	meshRoleBindingsList, err := common.FetchOwnedResources(r.Client, rbacv1.SchemeGroupVersion.WithKind("RoleBindingList"), mesh.Namespace, mesh.Namespace)
	if err != nil {
		reqLogger.Error(err, "could not read RoleBinding resources for mesh")
		return err
	}

	namespaceRoleBindings, err := common.FetchMeshResources(r.Client, rbacv1.SchemeGroupVersion.WithKind("RoleBindingList"), mesh.Namespace, namespace)
	if err != nil {
		reqLogger.Error(err, "error retrieving mesh RoleBindingList")
		return err
	}

	allErrors := []error{}

	// add required role bindings
	existingRoleBindings := nameSet(namespaceRoleBindings.Items)
	addedRoleBindings := map[string]struct{}{}
	requiredRoleBindings := map[string]struct{}{}
	for _, meshRoleBinding := range meshRoleBindingsList.Items {
		roleBindingName := meshRoleBinding.GetName()
		if _, ok := existingRoleBindings[roleBindingName]; !ok {
			reqLogger.Info("creating RoleBinding for mesh ServiceAccount", "RoleBinding", roleBindingName)
			roleBinding := &unstructured.Unstructured{}
			roleBinding.SetGroupVersionKind(rbacv1.SchemeGroupVersion.WithKind("RoleBinding"))
			roleBinding.SetNamespace(namespace)
			roleBinding.SetName(meshRoleBinding.GetName())
			roleBinding.SetLabels(meshRoleBinding.GetLabels())
			roleBinding.SetAnnotations(meshRoleBinding.GetAnnotations())
			if subjects, ok, _ := unstructured.NestedSlice(meshRoleBinding.UnstructuredContent(), "subjects"); ok {
				unstructured.SetNestedSlice(roleBinding.UnstructuredContent(), subjects, "subjects")
			}
			if roleRef, ok, _ := unstructured.NestedFieldNoCopy(meshRoleBinding.UnstructuredContent(), "roleRef"); ok {
				unstructured.SetNestedField(roleBinding.UnstructuredContent(), roleRef, "roleRef")
			}
			common.SetLabel(roleBinding, common.MemberOfKey, mesh.Namespace)
			err = r.Client.Create(context.TODO(), roleBinding)
			if err == nil {
				addedRoleBindings[roleBindingName] = struct{}{}
			} else {
				reqLogger.Error(err, "error creating RoleBinding for mesh ServiceAccount", "RoleBinding", roleBindingName)
				allErrors = append(allErrors, err)
			}
		} // XXX: else if existingRoleBinding.annotations[mesh-generation] != meshRoleBinding.annotations[generation] then update?
		requiredRoleBindings[roleBindingName] = struct{}{}
	}

	existingRoleBindings = merge(existingRoleBindings, addedRoleBindings)

	// delete obsolete role bindings
	for roleBindingName := range difference(existingRoleBindings, requiredRoleBindings) {
		r.Log.Info("deleting RoleBinding for mesh ServiceAccount", "RoleBinding", roleBindingName)
		roleBinding := &unstructured.Unstructured{}
		roleBinding.SetGroupVersionKind(rbacv1.SchemeGroupVersion.WithKind("RoleBinding"))
		roleBinding.SetName(roleBindingName)
		roleBinding.SetNamespace(namespace)
		err = r.Client.Delete(context.TODO(), roleBinding, client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil && !(errors.IsNotFound(err) || errors.IsGone(err)) {
			reqLogger.Error(err, "error deleting RoleBinding for mesh ServiceAccount", "RoleBinding", roleBindingName)
			allErrors = append(allErrors, err)
		}
	}

	// if there were errors, we've logged them and there's not really anything we can do, as we're in an uncertain state
	// maybe a following reconcile will add the required role binding that failed.  if it was a delete that failed, we're
	// just leaving behind some cruft.
	return utilerrors.NewAggregate(allErrors)
}

func (r *ReconcileMemberList) reconcilePodServiceAccounts(namespace string, mesh *v1.ServiceMeshControlPlane, reqLogger logr.Logger) error {
	// scan for pods with injection labels
	serviceAccounts := map[string]struct{}{}
	podList := &unstructured.UnstructuredList{}
	podList.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("PodList"))
	err := r.Client.List(context.TODO(), client.InNamespace(namespace), podList)
	if err == nil {
		// update account privileges used by deployments with injection labels
		for _, pod := range podList.Items {
			if val, ok := pod.GetAnnotations()["sidecar.istio.io/inject"]; ok && (val == "y" || val == "yes" || val == "true" || val == "on") {
				// XXX: this is pretty hacky.  we need to recreate the logic that determines whether or not injection is
				// enabled on the pod.  maybe we just have the user add ServiceAccounts to the ServiceMeshMember spec
				if podSA, ok, err := unstructured.NestedString(pod.UnstructuredContent(), "spec", "serviceAccountName"); ok || err == nil {
					if len(podSA) == 0 {
						podSA = "default"
					}
					serviceAccounts[podSA] = struct{}{}
				}
			}
		}
	} else {
		// skip trying to add, but delete whatever's left
		reqLogger.Error(err, "cannot update ServiceAccount SCC settings: error occurred scanning for Pods")
	}

	meshServiceAccounts, err := common.FetchMeshResources(r.Client, corev1.SchemeGroupVersion.WithKind("ServiceAccount"), mesh.Namespace, namespace)
	currentlyManagedServiceAccounts := nameSet(meshServiceAccounts.Items)
	if err != nil {
		// worst case, we'll try to associate the service accounts again
		reqLogger.Error(err, "cannot list ServiceAcccounts configured for use with mesh")
	}

	allErrors := []error{}

	if len(serviceAccounts) > 0 {
		// add labels before we add the ServiceAccount to the SCCs
		erroredServiceAccounts := map[string]struct{}{}
		for saName := range serviceAccounts {
			if _, ok := currentlyManagedServiceAccounts[saName]; ok {
				continue
			}
			saResource := &unstructured.Unstructured{}
			saResource.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ServiceAccount"))
			err = r.Client.Get(context.TODO(), client.ObjectKey{Name: saName, Namespace: namespace}, saResource)
			if err != nil {
				erroredServiceAccounts[saName] = struct{}{}
				reqLogger.Error(err, "error retrieving ServiceAccount to configure SCC", "ServiceAccount", saName)
				allErrors = append(allErrors, err)
			} else if !common.HasLabel(saResource, common.MemberOfKey) {
				common.SetLabel(saResource, common.MemberOfKey, mesh.Namespace)
				err = r.Client.Update(context.TODO(), saResource)
				if err != nil {
					erroredServiceAccounts[saName] = struct{}{}
					reqLogger.Error(err, "error setting label on ServiceAccount to configure SCC", "ServiceAccount", saName)
					allErrors = append(allErrors, err)
				}
			}
		}

		// XXX: use privileged and anyuid for now
		serviceAccountsToUpdate := toList(difference(serviceAccounts, erroredServiceAccounts))
		_, err = r.AddUsersToSCC("privileged", serviceAccountsToUpdate...)
		if err != nil {
			reqLogger.Error(err, "error adding ServiceAccounts to privileged SecurityContextConstraints", "ServiceAccounts", serviceAccountsToUpdate)
			allErrors = append(allErrors, err)
		}
		_, err = r.AddUsersToSCC("anyuid", serviceAccountsToUpdate...)
		if err != nil {
			reqLogger.Error(err, "error adding ServiceAccounts to anyuid SecurityContextConstraints", "ServiceAccounts", serviceAccountsToUpdate)
			allErrors = append(allErrors, err)
		}
	}

	// remove unused service accounts that may have been previously configured
	removedServiceAccounts := difference(currentlyManagedServiceAccounts, serviceAccounts)
	removedServiceAccountsList := toList(removedServiceAccounts)
	if err := r.RemoveUsersFromSCC("privileged", removedServiceAccountsList...); err != nil {
		reqLogger.Error(err, "error removing unused ServiceAccounts from privileged SecurityContextConstraints", "ServiceAccounts", removedServiceAccountsList)
		allErrors = append(allErrors, err)
	}
	if err := r.RemoveUsersFromSCC("anyuid", removedServiceAccountsList...); err != nil {
		reqLogger.Error(err, "error removing unused ServiceAccounts from anyuid SecurityContextConstraints", "ServiceAccounts", removedServiceAccountsList)
		allErrors = append(allErrors, err)
	}

	// Remove the labels, now that we've removed them from the SCCs
	for _, saResource := range meshServiceAccounts.Items {
		if _, ok := removedServiceAccounts[saResource.GetName()]; !ok {
			continue
		}
		common.DeleteLabel(&saResource, common.MemberOfKey)
		err = r.Client.Update(context.TODO(), &saResource)
		if err != nil {
			reqLogger.Error(err, "error removing member-of label from ServiceAccount", "ServiceAccount", saResource.GetName())
			// don't return these errors
		}
	}

	return utilerrors.NewAggregate(allErrors)
}

func toSet(values []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func toList(set map[string]struct{}) []string {
	list := make([]string, 0, len(set))
	for val := range set {
		list = append(list, val)
	}
	return list
}

func difference(source, remove map[string]struct{}) map[string]struct{} {
	diff := map[string]struct{}{}
	for val := range source {
		if _, ok := remove[val]; !ok {
			diff[val] = struct{}{}
		}
	}
	return diff
}

func merge(set1, set2 map[string]struct{}) map[string]struct{} {
	merged := map[string]struct{}{}
	for val := range set1 {
		merged[val] = struct{}{}
	}
	for val := range set2 {
		merged[val] = struct{}{}
	}
	return merged
}

func nameList(items []unstructured.Unstructured) []string {
	list := make([]string, 0, len(items))
	for _, object := range items {
		list = append(list, object.GetName())
	}
	return list
}

func nameSet(items []unstructured.Unstructured) map[string]struct{} {
	set := map[string]struct{}{}
	for _, object := range items {
		set[object.GetName()] = struct{}{}
	}
	return set
}