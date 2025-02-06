package actions

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"k8s.io/utils/ptr"

	"github.com/google/trillian"
	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResolveTree_CanHandle(t *testing.T) {
	tests := []struct {
		name         string
		phase        string
		canHandle    bool
		treeID       *int64
		statusTreeID *int64
	}{
		{
			name:      "spec.treeID is not nil and status.treeID is nil",
			phase:     constants.Creating,
			canHandle: true,
			treeID:    ptr.To(int64(123456)),
		},
		{
			name:         "spec.treeID != status.treeID",
			phase:        constants.Creating,
			canHandle:    true,
			treeID:       ptr.To(int64(123456)),
			statusTreeID: ptr.To(int64(654321)),
		},
		{
			name:         "spec.treeID is nil and status.treeID is not nil",
			phase:        constants.Creating,
			canHandle:    false,
			statusTreeID: ptr.To(int64(654321)),
		},
		{
			name:      "spec.treeID is nil and status.treeID is nil",
			phase:     constants.Creating,
			canHandle: true,
		},
		{
			name:      "no phase condition",
			phase:     "",
			canHandle: false,
		},
		{
			name:      constants.Ready,
			phase:     constants.Ready,
			canHandle: true,
		},
		{
			name:      constants.Pending,
			phase:     constants.Pending,
			canHandle: false,
		},
		{
			name:      constants.Creating,
			phase:     constants.Creating,
			canHandle: true,
		},
		{
			name:      constants.Initialize,
			phase:     constants.Initialize,
			canHandle: false,
		},
		{
			name:      constants.Failure,
			phase:     constants.Failure,
			canHandle: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewResolveTreeAction())
			instance := rhtasv1alpha1.CTlog{
				Spec: rhtasv1alpha1.CTlogSpec{
					TreeID: tt.treeID,
				},
				Status: rhtasv1alpha1.CTlogStatus{
					TreeID: tt.statusTreeID,
				},
			}
			if tt.phase != "" {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   constants.Ready,
					Reason: tt.phase,
				})
			}

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestResolveTree_Handle(t *testing.T) {
	g := NewWithT(t)
	type env struct {
		spec         rhtasv1alpha1.CTlogSpec
		statusTreeId *int64
		createTree   createTree
	}
	type want struct {
		result *action.Result
		verify func(Gomega, *rhtasv1alpha1.CTlog)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "create a new tree",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					TreeID:   nil,
					Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(8091))},
				},
				createTree: mockCreateTree(&trillian.Tree{TreeId: 5555555}, nil, nil),
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, ctlog *rhtasv1alpha1.CTlog) {
					g.Expect(ctlog.Spec.TreeID).Should(BeNil())
					g.Expect(ctlog.Status.TreeID).ShouldNot(BeNil())
					g.Expect(ctlog.Status.TreeID).To(HaveValue(BeNumerically(">", 0)))
					g.Expect(ctlog.Status.TreeID).To(HaveValue(BeNumerically("==", 5555555)))
				},
			},
		},
		{
			name: "update tree",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					TreeID:   ptr.To(int64(123456)),
					Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(8091))},
				},
				statusTreeId: ptr.To(int64(654321)),
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, ctlog *rhtasv1alpha1.CTlog) {
					g.Expect(ctlog.Spec.TreeID).ShouldNot(BeNil())
					g.Expect(ctlog.Status.TreeID).ShouldNot(BeNil())
					g.Expect(ctlog.Spec.TreeID).To(HaveValue(BeNumerically(">", 0)))
					g.Expect(ctlog.Spec.TreeID).To(HaveValue(BeNumerically("==", *ctlog.Status.TreeID)))
				},
			},
		},
		{
			name: "use tree from spec",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					TreeID:   ptr.To(int64(123456)),
					Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(8091))},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, ctlog *rhtasv1alpha1.CTlog) {
					g.Expect(ctlog.Spec.TreeID).ShouldNot(BeNil())
					g.Expect(ctlog.Status.TreeID).ShouldNot(BeNil())
					g.Expect(ctlog.Spec.TreeID).To(HaveValue(BeNumerically(">", 0)))
					g.Expect(ctlog.Spec.TreeID).To(HaveValue(BeNumerically("==", *ctlog.Status.TreeID)))
					g.Expect(ctlog.Status.TreeID).To(HaveValue(BeNumerically("==", 123456)))
				},
			},
		},
		{
			name: "unable to create a new tree",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					TreeID:   nil,
					Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(8091))},
				},
				createTree: mockCreateTree(nil, errors.New("timeout error"), nil),
			},
			want: want{
				result: testAction.Error(fmt.Errorf("could not create trillian tree: timeout error")),
				verify: func(g Gomega, ctlog *rhtasv1alpha1.CTlog) {
					g.Expect(ctlog.Spec.TreeID).Should(BeNil())
					g.Expect(ctlog.Status.TreeID).Should(BeNil())
				},
			},
		},
		{
			name: "resolve trillian address",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(8091))},
				},
				createTree: mockCreateTree(&trillian.Tree{TreeId: 5555555}, nil, func(displayName string, trillianURL string, deadline int64) {
					g.Expect(trillianURL).Should(Equal(fmt.Sprintf("%s.%s.svc:%d", actions.LogserverDeploymentName, "default", 8091)))
				}),
			},
			want: want{
				result: testAction.StatusUpdate(),
			},
		},
		{
			name: "custom trillian address",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(1234)), Address: "custom-address.namespace.svc"},
				},
				createTree: mockCreateTree(&trillian.Tree{TreeId: 5555555}, nil, func(displayName string, trillianURL string, deadline int64) {
					g.Expect(trillianURL).Should(Equal(fmt.Sprintf("custom-address.namespace.svc:%d", 1234)))
				}),
			},
			want: want{
				result: testAction.StatusUpdate(),
			},
		},
		{
			name: "trillian port not specified",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					Trillian: rhtasv1alpha1.TrillianService{Port: nil},
				},
			},
			want: want{
				result: testAction.Error(fmt.Errorf("resolve treeID: %v", utils.TrillianPortNotSpecified)),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			instance := &rhtasv1alpha1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ctlog",
					Namespace: "default",
				},
				Spec: tt.env.spec,
				Status: rhtasv1alpha1.CTlogStatus{
					TreeID: tt.env.statusTreeId,
					Conditions: []metav1.Condition{
						{
							Type:   constants.Ready,
							Reason: constants.Creating,
						},
					},
				},
			}

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				Build()

			a := testAction.PrepareAction(c, NewResolveTreeAction(func(t *resolveTreeAction) {
				if tt.env.createTree == nil {
					t.createTree = mockCreateTree(nil, errors.New("createTree should not be executed"), nil)
				} else {
					t.createTree = tt.env.createTree
				}
			}))

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}
			if tt.want.verify != nil {
				tt.want.verify(g, instance)
			}
		})
	}
}

func mockCreateTree(tree *trillian.Tree, err error, verify func(displayName string, trillianURL string, deadline int64)) createTree {
	return func(ctx context.Context, displayName string, trillianURL string, deadline int64) (*trillian.Tree, error) {
		if verify != nil {
			verify(displayName, trillianURL, deadline)
		}
		return tree, err
	}
}
