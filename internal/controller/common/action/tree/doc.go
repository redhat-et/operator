/*
Package tree implements the "resolve tree" action.

This action manages the creation of Merkle trees using the Trillian service. It encapsulates
all the necessary steps from setting up required resources to launching and monitoring the
tree creation process and updates the custom resource with the new tree information. The main
purpose of this action is to ensure that Merkle trees are reliably created and tracked as part
of the operator's reconciliation process.

The action is implemented as a generic type to work with any custom resource that satisfies
the [apis.ConditionsAwareObject] and [apis.TlsClient] interfaces.

Workflow:
 1. Check if the tree is already resolved. If a tree ID exists in the resource, update the status and exit.
 2. Prepare the environment by creating or updating the necessary RBAC resources (ServiceAccount, Role, and RoleBinding)
    and setting up a ConfigMap used to store the result of the tree creation job.
 3. Launch the tree creation job by submitting a Kubernetes Job that executes the Trillian tree creation script. Configuration,
    such as the ConfigMap name, admin server address, and TLS settings, is passed to the job via environment variables.
 4. Monitor and handle the job by waiting for its completion. If the job is still running or fails, requeue the reconciliation.
 5. Process the job results by extracting the tree ID from the ConfigMap, updating the custom resource status with the new tree ID,
    and recording a success event.

Usage:

	wrapper := tree.Wrapper[*v1alpha1.Rekor](
		func(obj *v1alpha1.Rekor) *int64 {
			return obj.Spec.TreeID
		},
		func(obj *v1alpha1.Rekor) *int64 {
			return obj.Status.TreeID
		},
		func(rekor *v1alpha1.Rekor, i *int64) {
			obj.Status.TreeID = i
		},
		func(obj *v1alpha1.Rekor) *v1alpha1.TrillianService {
			return &obj.Spec.Trillian
		})
	tree.NewResolveTreeAction[*v1alpha1.Rekor]("rekor", wrapper)
*/
package tree
