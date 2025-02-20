package tree

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	. "github.com/onsi/gomega"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"k8s.io/utils/ptr"
)

type namedTest struct {
	name string
	run  func(t *testing.T)
}

var tests []namedTest

var defaultWrapper = Wrapper[*v1alpha1.Rekor](
	func(rekor *v1alpha1.Rekor) *int64 {
		return rekor.Spec.TreeID
	},
	func(rekor *v1alpha1.Rekor) *int64 {
		return rekor.Status.TreeID
	},
	func(rekor *v1alpha1.Rekor, i *int64) {
		rekor.Status.TreeID = i
	},
	func(rekor *v1alpha1.Rekor) *v1alpha1.TrillianService {
		return &rekor.Spec.Trillian
	},
)

var (
	nnObject = types.NamespacedName{Name: "test", Namespace: "default"}
	nnResult = types.NamespacedName{Name: fmt.Sprintf(configMapResultMask, "test", "test"), Namespace: "default"}
)

func init() {
	tests = []namedTest{
		{name: "manual", run: testManual},
		{name: "rbac", run: testRbac},
		{name: "configmap", run: testConfigMap},
		{name: "create-job", run: testCreateJob},
		{name: "monitor-job", run: testMonitorJob},
		{name: "extract-result", run: testExtractResult},
	}
}

type pre struct {
	warmUp bool
	before func(context.Context, Gomega, client.WithWatch)
}
type want struct {
	result *action.Result
	verify func(context.Context, Gomega, client.WithWatch)
}

func testManual(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "not-set",
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					r := v1alpha1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())
					g.Expect(r.Spec.TreeID).Should(BeNil())
					g.Expect(r.Status.TreeID).Should(BeNil())
				},
			},
		},
		{
			desc: "set",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					r := v1alpha1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())

					r.Spec.TreeID = ptr.To(int64(123456789))
					g.Expect(c.Update(ctx, &r)).To(Succeed())
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					r := v1alpha1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())
					g.Expect(r.Spec.TreeID).ShouldNot(BeNil())
					g.Expect(r.Status.TreeID).ShouldNot(BeNil())
					g.Expect(*r.Spec.TreeID).Should(Equal(int64(123456789)))
					g.Expect(*r.Status.TreeID).Should(Equal(int64(123456789)))

					cond := meta.FindStatusCondition(r.GetConditions(), JobCondition)
					g.Expect(cond).Should(BeNil())
				},
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *resolveTree[*v1alpha1.Rekor], ctx context.Context, rekor *v1alpha1.Rekor) *action.Result {
			return r.handleManual(ctx, rekor)
		}))
	}
}

func testRbac(t *testing.T) {
	for _, tc := range []struct {
		desc string
		want want
	}{
		{desc: "ensure", want: want{
			result: testAction.Continue(),
			verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
				nn := types.NamespacedName{Name: fmt.Sprintf(RBACNameMask, "test"), Namespace: "default"}
				g.Expect(c.Get(ctx, nn, &corev1.ServiceAccount{})).To(Succeed())
				g.Expect(c.Get(ctx, nn, &rbacv1.Role{})).To(Succeed())
				g.Expect(c.Get(ctx, nn, &rbacv1.RoleBinding{})).To(Succeed())
			},
		}},
	} {
		t.Run(tc.desc, testRunner(pre{}, tc.want, func(r *resolveTree[*v1alpha1.Rekor], ctx context.Context, rekor *v1alpha1.Rekor) *action.Result {
			return r.handleRbac(ctx, rekor)
		}))
	}
}

func testConfigMap(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "not-exists",
			pre: pre{
				warmUp: false,
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					g.Expect(c.Get(ctx, nnResult, &corev1.ConfigMap{})).To(Succeed())
				},
			}},
		{
			desc: "exists",
			pre: pre{
				warmUp: true,
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					g.Expect(c.Get(ctx, nnResult, &corev1.ConfigMap{})).To(Succeed())
				},
			}},
		{
			desc: "ignore-changes-data",
			pre: pre{
				warmUp: true,
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{}
					g.Expect(c.Get(ctx, nnResult, cm)).To(Succeed())

					cm.Data = map[string]string{
						"foo": "bar",
					}
					g.Expect(c.Update(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{}
					g.Expect(c.Get(ctx, nnResult, cm)).To(Succeed())

					g.Expect(cm.Data["foo"]).To(Equal("bar"))
				},
			}},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *resolveTree[*v1alpha1.Rekor], ctx context.Context, rekor *v1alpha1.Rekor) *action.Result {
			return r.handleConfigMap(ctx, rekor)
		}))
	}
}

func testCreateJob(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "requeue",
			want: want{
				result: testAction.Requeue(),
			},
		},
		{
			desc: "continue",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							Annotations: map[string]string{
								jobReferenceAnnotation: "job-name",
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Continue(),
			},
		},
		{
			desc: "ensure",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					jobs := &v1.JobList{}
					g.Expect(c.List(ctx, jobs, client.InNamespace("default"))).To(Succeed())
					g.Expect(jobs.Items).To(HaveLen(1))
					jobName := jobs.Items[0].Name

					cm := &corev1.ConfigMap{}
					g.Expect(c.Get(ctx, nnResult, cm)).To(Succeed())
					g.Expect(cm.Annotations[jobReferenceAnnotation]).To(Equal(jobName))
				},
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *resolveTree[*v1alpha1.Rekor], ctx context.Context, rekor *v1alpha1.Rekor) *action.Result {
			return r.handleJob(ctx, rekor)
		}))
	}
}

func testMonitorJob(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "requeue: missing configmap",
			want: want{
				result: testAction.Requeue(),
			},
		},
		{
			desc: "requeue: missing reference",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Requeue(),
			},
		},
		{
			desc: "requeue: missing job",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							Annotations: map[string]string{
								jobReferenceAnnotation: "job",
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Requeue(),
			},
		},
		{
			desc: "requeue: job not completed",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							Annotations: map[string]string{
								jobReferenceAnnotation: "job",
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())

					job := v1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "job",
							Namespace: nnResult.Namespace,
						},
					}
					g.Expect(c.Create(ctx, &job)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Requeue(),
			},
		},
		{
			desc: "job failed",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							Annotations: map[string]string{
								jobReferenceAnnotation: "job",
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())

					job := v1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "job",
							Namespace: nnResult.Namespace,
						},
						Status: v1.JobStatus{
							Conditions: []v1.JobCondition{
								{
									Status: corev1.ConditionTrue, Type: v1.JobComplete,
								},
								{
									Status: corev1.ConditionTrue, Type: v1.JobFailed,
								},
							},
						},
					}
					g.Expect(c.Create(ctx, &job)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Error(reconcile.TerminalError(JobFailed)),
			},
		},
		{
			desc: "continue",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							Annotations: map[string]string{
								jobReferenceAnnotation: "job",
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
					job := v1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "job",
							Namespace: nnResult.Namespace,
						},
						Status: v1.JobStatus{
							Conditions: []v1.JobCondition{
								{
									Status: corev1.ConditionTrue, Type: v1.JobComplete,
								},
							},
						},
					}
					g.Expect(c.Create(ctx, &job)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Continue(),
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *resolveTree[*v1alpha1.Rekor], ctx context.Context, rekor *v1alpha1.Rekor) *action.Result {
			return r.handleJobFinished(ctx, rekor)
		}))
	}
}

func testExtractResult(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "requeue: missing configmap",
			want: want{
				result: testAction.Requeue(),
			},
		},
		{
			desc: "requeue: missing data",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							Annotations: map[string]string{
								jobReferenceAnnotation: "job",
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Requeue(),
			},
		},
		{
			desc: "error: corrupted data",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							Annotations: map[string]string{
								jobReferenceAnnotation: "job",
							},
						},
						Data: map[string]string{
							"tree_id": "not-a-number",
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Error(reconcile.TerminalError(
					&strconv.NumError{Func: "ParseInt", Num: "not-a-number", Err: strconv.ErrSyntax},
				)),
			},
		},
		{
			desc: "success: update TreeID status",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							Annotations: map[string]string{
								jobReferenceAnnotation: "job",
							},
						},
						Data: map[string]string{
							"tree_id": "123456789",
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					r := v1alpha1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())

					g.Expect(r.Status.TreeID).ToNot(BeNil())
					g.Expect(*r.Status.TreeID).To(Equal(int64(123456789)))
				},
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *resolveTree[*v1alpha1.Rekor], ctx context.Context, rekor *v1alpha1.Rekor) *action.Result {
			return r.handleExtractJobResult(ctx, rekor)
		}))
	}
}

type handleFn func(*resolveTree[*v1alpha1.Rekor], context.Context, *v1alpha1.Rekor) *action.Result

func testRunner(pre pre, want want, handleFn handleFn) func(t *testing.T) {
	return func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		instance := &v1alpha1.Rekor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nnObject.Name,
				Namespace: nnObject.Namespace,
			},
			Spec: v1alpha1.RekorSpec{
				Trillian: v1alpha1.TrillianService{
					Address: "trillian-logserver",
					Port:    ptr.To(int32(8091)),
				},
			},
		}

		c := testAction.FakeClientBuilder().
			WithObjects(instance).
			WithStatusSubresource(instance).
			Build()

		a := testAction.PrepareAction(c, NewResolveTreeAction("test", defaultWrapper))
		ra := a.(*resolveTree[*v1alpha1.Rekor])

		if pre.warmUp {
			handleFn(ra, ctx, instance)
		}

		if pre.before != nil {
			pre.before(ctx, g, c)
		}

		g.Expect(c.Get(ctx, nnObject, instance)).To(Succeed())

		if got := handleFn(ra, ctx, instance); !reflect.DeepEqual(got, want.result) {
			t.Errorf("CanHandle() = %v, want %v", got, want.result)
		}
		if want.verify != nil {
			want.verify(ctx, g, c)
		}
	}
}

func TestResolveTree_CanHandle(t *testing.T) {
	tests := []struct {
		name         string
		condition    metav1.ConditionStatus
		canHandle    bool
		treeID       *int64
		statusTreeID *int64
	}{
		{
			name:      "spec.treeID is not nil and status.treeID is nil",
			condition: metav1.ConditionTrue,
			canHandle: true,
			treeID:    ptr.To(int64(123456)),
		}, {
			name:         "spec.treeID != status.treeID",
			condition:    metav1.ConditionTrue,
			canHandle:    true,
			treeID:       ptr.To(int64(123456)),
			statusTreeID: ptr.To(int64(654321)),
		}, {
			name:         "spec.treeID is nil and status.treeID is not nil",
			condition:    metav1.ConditionTrue,
			canHandle:    false,
			statusTreeID: ptr.To(int64(654321)),
		}, {
			name:      "spec.treeID is nil and status.treeID is nil",
			condition: metav1.ConditionTrue,
			canHandle: true,
		}, {
			name:         "status condition is false",
			condition:    metav1.ConditionFalse,
			canHandle:    true,
			statusTreeID: ptr.To(int64(654321)),
		}, {
			name:         "status condition is Unknown",
			condition:    metav1.ConditionUnknown,
			canHandle:    true,
			statusTreeID: ptr.To(int64(654321)),
		}, {
			name:         "empty status",
			condition:    "",
			canHandle:    true,
			statusTreeID: ptr.To(int64(654321)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewResolveTreeAction("test", defaultWrapper))
			instance := v1alpha1.Rekor{
				Spec: v1alpha1.RekorSpec{
					TreeID: tt.treeID,
				},
				Status: v1alpha1.RekorStatus{
					TreeID: tt.statusTreeID,
				},
			}
			if tt.condition != "" {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   JobCondition,
					Status: tt.condition,
				})
			}

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestResolveTree_Handle(t *testing.T) {
	for _, nt := range tests {
		t.Run(nt.name, func(t *testing.T) {
			nt.run(t)
		})
	}
}
