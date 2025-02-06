package db

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/labels"
	actions2 "github.com/securesign/operator/internal/controller/trillian/actions"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandleSecret_CanHandle(t *testing.T) {
	tests := []struct {
		name      string
		instance  rhtasv1alpha1.Trillian
		condition metav1.ConditionStatus
		canHandle bool
	}{
		{
			name:      "no phase condition",
			canHandle: true,
		},
		{
			name:      "ConditionUnknown",
			condition: metav1.ConditionUnknown,
			canHandle: true,
		},
		{
			name:      "ConditionTrue: status.db.databaseSecretRef == nil",
			condition: metav1.ConditionTrue,
			instance: rhtasv1alpha1.Trillian{
				Status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						DatabaseSecretRef: nil,
					},
				},
			},
			canHandle: true,
		},
		{
			name:      "ConditionTrue: status.db.databaseSecretRef != nil",
			condition: metav1.ConditionTrue,
			instance: rhtasv1alpha1.Trillian{
				Status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{
							Name: "connection",
						},
					},
				},
			},
			canHandle: false,
		},
		{
			name:      "ConditionTrue: status.db.databaseSecretRef != spec.db.databaseSecretRef",
			condition: metav1.ConditionTrue,
			instance: rhtasv1alpha1.Trillian{
				Spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{
							Name: "new-connection",
						},
					},
				},
				Status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{
							Name: "connection",
						},
					},
				},
			},
			canHandle: true,
		},
		{
			name:      "ConditionTrue: status.db.databaseSecretRef == spec.db.databaseSecretRef",
			condition: metav1.ConditionTrue,
			instance: rhtasv1alpha1.Trillian{
				Spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{
							Name: "connection",
						},
					},
				},
				Status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{
							Name: "connection",
						},
					},
				},
			},
			canHandle: false,
		},
		{
			name:      "ConditionFalse",
			condition: metav1.ConditionFalse,
			canHandle: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewHandleSecretAction())

			instance := tt.instance
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   actions2.DbCondition,
				Status: tt.condition,
			})

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestHandleSecret_Handle(t *testing.T) {
	namespacedName := types.NamespacedName{Namespace: "default", Name: "trillian"}
	type env struct {
		spec    rhtasv1alpha1.TrillianSpec
		status  rhtasv1alpha1.TrillianStatus
		objects []client.Object
	}
	type want struct {
		result *action.Result
		verify func(Gomega, client.WithWatch, <-chan watch.Event)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "external: missing spec.db.databaseSecretRef",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(false),
						DatabaseSecretRef: nil,
					},
				},
			},
			want: want{
				result: testAction.Error(reconcile.TerminalError(ErrMissingDBConfiguration)),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					condition := meta.FindStatusCondition(instance.GetConditions(), actions2.DbCondition)
					g.Expect(condition.Status).Should(Equal(metav1.ConditionFalse))
					g.Expect(condition.Reason).Should(Equal(constants.Failure))

					g.Expect(events).To(BeEmpty())
				},
			},
		},
		{
			name: "external: set spec.db.databaseSecretRef",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(false),
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "connection"},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					condition := meta.FindStatusCondition(instance.GetConditions(), actions2.DbCondition)
					g.Expect(condition.Status).Should(Equal(metav1.ConditionTrue))
					g.Expect(condition.Reason).Should(Equal(constants.Ready))

					g.Expect(instance.Status.Db.DatabaseSecretRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Db.DatabaseSecretRef.Name).To(Equal("connection"))

					g.Expect(events).To(BeEmpty())
				},
			},
		},
		{
			name: "external: modify spec.db.databaseSecretRef",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(false),
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "new-connection"},
					},
				},
				status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(false),
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "old-connection"},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					condition := meta.FindStatusCondition(instance.GetConditions(), actions2.DbCondition)
					g.Expect(condition.Status).Should(Equal(metav1.ConditionTrue))
					g.Expect(condition.Reason).Should(Equal(constants.Ready))

					g.Expect(instance.Status.Db.DatabaseSecretRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Db.DatabaseSecretRef.Name).To(Equal("new-connection"))

					g.Expect(events).To(BeEmpty())
				},
			},
		},
		{
			name: "external: unmodified spec.db.databaseSecretRef",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(false),
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "connection"},
					},
				},
				status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(false),
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "connection"},
					},
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					g.Expect(instance.Status.Db.DatabaseSecretRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Db.DatabaseSecretRef.Name).To(Equal("connection"))

					g.Expect(events).To(BeEmpty())
				},
			},
		},
		{
			name: "managed: set spec.db.databaseSecretRef",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(true),
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "connection"},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					condition := meta.FindStatusCondition(instance.GetConditions(), actions2.DbCondition)
					g.Expect(condition.Status).Should(Equal(metav1.ConditionFalse))
					g.Expect(condition.Reason).Should(Equal(constants.Pending))

					g.Expect(instance.Status.Db.DatabaseSecretRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Db.DatabaseSecretRef.Name).To(Equal("connection"))

					g.Expect(events).To(BeEmpty())
				},
			},
		},
		{
			name: "managed: empty spec.db.databaseSecretRef",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(true),
						DatabaseSecretRef: nil,
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					condition := meta.FindStatusCondition(instance.GetConditions(), actions2.DbCondition)
					g.Expect(condition.Status).Should(Equal(metav1.ConditionFalse))
					g.Expect(condition.Reason).Should(Equal(constants.Pending))

					g.Expect(events).To(HaveLen(1))
					event := <-events
					g.Expect(event.Type).To(Equal(watch.Added))
					secret := event.Object.(*core.Secret)

					g.Expect(instance.Status.Db.DatabaseSecretRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Db.DatabaseSecretRef.Name).To(Equal(secret.Name))
				},
			},
		},
		{
			name: "managed: update spec.db.databaseSecretRef",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(true),
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "new-connection"},
					},
				},
				status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(true),
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "old-connection"},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					condition := meta.FindStatusCondition(instance.GetConditions(), actions2.DbCondition)
					g.Expect(condition.Status).Should(Equal(metav1.ConditionFalse))
					g.Expect(condition.Reason).Should(Equal(constants.Pending))

					g.Expect(instance.Status.Db.DatabaseSecretRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Db.DatabaseSecretRef.Name).To(Equal("new-connection"))

					g.Expect(events).To(BeEmpty())
				},
			},
		},
		{
			name: "managed: unmodified spec.db.databaseSecretRef",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(true),
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "connection"},
					},
				},
				status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(true),
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "connection"},
					},
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					condition := meta.FindStatusCondition(instance.GetConditions(), actions2.DbCondition)
					g.Expect(condition.Status).Should(Equal(metav1.ConditionFalse))
					g.Expect(condition.Reason).Should(Equal(constants.Pending))

					g.Expect(instance.Status.Db.DatabaseSecretRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Db.DatabaseSecretRef.Name).To(Equal("connection"))

					g.Expect(events).To(BeEmpty())
				},
			},
		},
		{
			name: "managed: unmodified generated db connection",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create: ptr.To(true),
					},
				},
				status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						Create:            ptr.To(true),
						DatabaseSecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "connection"},
					},
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					condition := meta.FindStatusCondition(instance.GetConditions(), actions2.DbCondition)
					g.Expect(condition.Status).Should(Equal(metav1.ConditionFalse))
					g.Expect(condition.Reason).Should(Equal(constants.Pending))

					g.Expect(instance.Status.Db.DatabaseSecretRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Db.DatabaseSecretRef.Name).To(Equal("connection"))

					g.Expect(events).To(BeEmpty())
				},
			},
		},
		{
			name: "managed: SECURESIGN_1455: link unassigned db-connection secret",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create: ptr.To(true),
					},
				},
				status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						Create: ptr.To(true),
					},
				},
				objects: []client.Object{
					&core.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "unlinked-connection",
							Namespace: "default",
							Labels: map[string]string{
								labels.LabelAppInstance:  "trillian",
								labels.LabelAppComponent: actions2.DbComponentName,
								labels.LabelAppName:      actions2.DbDeploymentName,
								labels.LabelAppPartOf:    constants.AppName,
								labels.LabelAppManagedBy: "controller-manager",
								labels.LabelResource:     dbConnectionResource,
							},
							Annotations: map[string]string{
								annotationDatabase: "trillian",
								annotationUser:     "mysql",
								annotationPort:     strconv.Itoa(port),
								annotationHost:     host,
							},
						},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					condition := meta.FindStatusCondition(instance.GetConditions(), actions2.DbCondition)
					g.Expect(condition.Status).Should(Equal(metav1.ConditionFalse))
					g.Expect(condition.Reason).Should(Equal(constants.Pending))

					g.Expect(instance.Status.Db.DatabaseSecretRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Db.DatabaseSecretRef.Name).To(Equal("unlinked-connection"))

					g.Expect(events).To(BeEmpty())
				},
			},
		},
		{
			name: "managed: SECURESIGN_1455: delete old db-connection secret",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create: ptr.To(true),
					},
				},
				status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						Create: ptr.To(true),
					},
				},
				objects: []client.Object{
					&core.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "old-connection-1",
							Namespace: "default",
							Labels: map[string]string{
								labels.LabelAppInstance:  "trillian",
								labels.LabelAppComponent: actions2.DbComponentName,
								labels.LabelAppName:      actions2.DbDeploymentName,
								labels.LabelAppPartOf:    constants.AppName,
								labels.LabelAppManagedBy: "controller-manager",
								labels.LabelResource:     dbConnectionResource,
							},
						},
					},
					&core.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "old-connection-2",
							Namespace: "default",
							Labels: map[string]string{
								labels.LabelAppInstance:  "trillian",
								labels.LabelAppComponent: actions2.DbComponentName,
								labels.LabelAppName:      actions2.DbDeploymentName,
								labels.LabelAppPartOf:    constants.AppName,
								labels.LabelAppManagedBy: "controller-manager",
								labels.LabelResource:     dbConnectionResource,
							},
							Annotations: map[string]string{
								annotationDatabase: "old",
								annotationUser:     "mysql",
								annotationPort:     strconv.Itoa(port),
								annotationHost:     "old",
							},
						},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					condition := meta.FindStatusCondition(instance.GetConditions(), actions2.DbCondition)
					g.Expect(condition.Status).Should(Equal(metav1.ConditionFalse))
					g.Expect(condition.Reason).Should(Equal(constants.Pending))

					g.Expect(events).To(HaveLen(3))

					var newName string
					for event := range events {
						switch event.Type {
						case watch.Deleted:
							g.Expect(event.Object.(*core.Secret).Name).To(ContainSubstring("old-connection"))
						case watch.Added:
							newName = event.Object.(*core.Secret).Name
							g.Expect(newName).To(ContainSubstring(dbConnectionSecretName))
						default:
							g.Expect(true).Should(BeFalse(), "should not be executed")
						}
					}

					g.Expect(instance.Status.Db.DatabaseSecretRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Db.DatabaseSecretRef.Name).To(Equal(newName))

				},
			},
		},
		{
			name: "managed: SECURESIGN_1455: link valid and delete old db-connection secret",
			env: env{
				spec: rhtasv1alpha1.TrillianSpec{
					Db: rhtasv1alpha1.TrillianDB{
						Create: ptr.To(true),
					},
				},
				status: rhtasv1alpha1.TrillianStatus{
					Db: rhtasv1alpha1.TrillianDB{
						Create: ptr.To(true),
					},
				},
				objects: []client.Object{
					&core.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "unlinked-connection",
							Namespace: "default",
							Labels: map[string]string{
								labels.LabelAppInstance:  "trillian",
								labels.LabelAppComponent: actions2.DbComponentName,
								labels.LabelAppName:      actions2.DbDeploymentName,
								labels.LabelAppPartOf:    constants.AppName,
								labels.LabelAppManagedBy: "controller-manager",
								labels.LabelResource:     dbConnectionResource,
							},
							Annotations: map[string]string{
								annotationDatabase: "trillian",
								annotationUser:     "mysql",
								annotationPort:     strconv.Itoa(port),
								annotationHost:     host,
							},
						},
					},
					&core.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "old-connection",
							Namespace: "default",
							Labels: map[string]string{
								labels.LabelAppInstance:  "trillian",
								labels.LabelAppComponent: actions2.DbComponentName,
								labels.LabelAppName:      actions2.DbDeploymentName,
								labels.LabelAppPartOf:    constants.AppName,
								labels.LabelAppManagedBy: "controller-manager",
								labels.LabelResource:     dbConnectionResource,
							},
							Annotations: map[string]string{
								annotationDatabase: "old",
								annotationUser:     "mysql",
								annotationPort:     strconv.Itoa(port),
								annotationHost:     "old",
							},
						},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					instance := &rhtasv1alpha1.Trillian{}
					g.Expect(cli.Get(context.TODO(), namespacedName, instance)).To(Succeed())

					condition := meta.FindStatusCondition(instance.GetConditions(), actions2.DbCondition)
					g.Expect(condition.Status).Should(Equal(metav1.ConditionFalse))
					g.Expect(condition.Reason).Should(Equal(constants.Pending))

					g.Expect(events).To(HaveLen(1))
					for event := range events {
						switch event.Type {
						case watch.Deleted:
							g.Expect(event.Object.(*core.Secret).Name).To(ContainSubstring("old-connection"))
						default:
							g.Expect(true).Should(BeFalse(), "should not be executed")
						}

						g.Expect(instance.Status.Db.DatabaseSecretRef).ShouldNot(BeNil())
						g.Expect(instance.Status.Db.DatabaseSecretRef.Name).To(Equal("unlinked-connection"))
					}
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()
			instance := &rhtasv1alpha1.Trillian{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "trillian",
					Namespace: "default",
				},
				Spec:   tt.env.spec,
				Status: tt.env.status,
			}

			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   actions2.DbCondition,
				Status: metav1.ConditionFalse,
				Reason: constants.Pending,
			})

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithObjects(tt.env.objects...).
				Build()

			watchSecrets, err := c.Watch(ctx, &core.SecretList{}, client.InNamespace(instance.Namespace))
			g.Expect(err).ShouldNot(HaveOccurred())

			a := testAction.PrepareAction(c, NewHandleSecretAction())

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}

			watchSecrets.Stop()
			if tt.want.verify != nil {
				tt.want.verify(g, c, watchSecrets.ResultChan())
			}
		})
	}
}
