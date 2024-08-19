package actions

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	common "github.com/securesign/operator/internal/testing/common/tsa"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_NewNtpMonitoringAction(t *testing.T) {
	g := NewWithT(t)

	action := NewNtpMonitoringAction()
	g.Expect(action).ToNot(BeNil())
}

func Test_NTPName(t *testing.T) {
	g := NewWithT(t)

	action := NewNtpMonitoringAction()
	g.Expect(action.Name()).To(Equal("ntpMonitoring"))
}

func Test_NTPCanHandle(t *testing.T) {
	g := NewWithT(t)
	tests := []struct {
		name     string
		testCase func(*rhtasv1alpha1.TimestampAuthority)
		expected bool
	}{
		{
			name:     "Default condition",
			testCase: func(instance *rhtasv1alpha1.TimestampAuthority) {},
			expected: true,
		},
		{
			name: "Creating condition",
			testCase: func(instance *rhtasv1alpha1.TimestampAuthority) {
				instance.Status.Conditions[0].Reason = constants.Creating
			},
			expected: true,
		},
		{
			name: "NTPMonitoring status is different to spec",
			testCase: func(instance *rhtasv1alpha1.TimestampAuthority) {
				instance.Status.NTPMonitoring = &rhtasv1alpha1.NTPMonitoring{
					Enabled: true,
					Config: &rhtasv1alpha1.NtpMonitoringConfig{
						RequestAttempts: 1,
						RequestTimeout:  5,
						NumServers:      4,
						ServerThreshold: 3,
						MaxTimeDelta:    6,
						Period:          60,
						Servers:         []string{"time.apple.com", "time.google.com"},
					},
				}
			},
			expected: true,
		},
		{
			name: "Pending condition",
			testCase: func(instance *rhtasv1alpha1.TimestampAuthority) {
				instance.Status.Conditions[0].Reason = constants.Pending
			},
			expected: false,
		},
		{
			name: "NTPMonitoring is disabled",
			testCase: func(instance *rhtasv1alpha1.TimestampAuthority) {
				instance.Spec.NTPMonitoring.Enabled = false
			},
			expected: false,
		},
		{
			name: "NTPMonitoring config is nil",
			testCase: func(instance *rhtasv1alpha1.TimestampAuthority) {
				instance.Spec.NTPMonitoring.Config = nil
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewNtpMonitoringAction()
			instance := common.GenerateTSAInstance()
			tt.testCase(instance)
			g.Expect(action.CanHandle(context.TODO(), instance)).To(Equal(tt.expected))
		})
	}
}

func Test_NTPHandle(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority])
		testCase func(Gomega, action.Action[*rhtasv1alpha1.TimestampAuthority], client.WithWatch, *rhtasv1alpha1.TimestampAuthority) bool
	}{
		{
			name: "Succeeds with config specified",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Creating
				return common.TsaTestSetup(instance, t, nil, NewNtpMonitoringAction(), []client.Object{}...)
			},
			testCase: func(g Gomega, _ action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				if !g.Expect(instance.Status.NTPMonitoring).ToNot(BeNil()) {
					t.Error("Status NTP Monitoring Config should not be nil")
					return false
				}
				cm := &corev1.ConfigMap{}
				if err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Status.NTPMonitoring.Config.NtpConfigRef.Name, Namespace: instance.GetNamespace()}, cm); err != nil {
					t.Errorf("Unable to find config map: %s", err)
					return false
				}

				if !g.Expect(instance.Status.NTPMonitoring.Config.NtpConfigRef.Name).To(Equal(cm.Name)) {
					t.Errorf("Config Map name mismatch: expected %v, got %v", instance.Status.NTPMonitoring.Config.NtpConfigRef.Name, cm.Name)
					return false
				}

				return g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Message).To(Equal("NTP monitoring configured"))
			},
		},
		{
			name: "Succeeds with config provided",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Creating
				instance.Spec.NTPMonitoring = rhtasv1alpha1.NTPMonitoring{
					Enabled: true,
					Config: &rhtasv1alpha1.NtpMonitoringConfig{
						NtpConfigRef: &rhtasv1alpha1.LocalObjectReference{
							Name: "ntp-config",
						},
					},
				}

				config := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ntp-config",
						Namespace: instance.GetNamespace(),
					},
					Data: map[string]string{"ntp-config.yaml": ""},
				}

				obj := []client.Object{}
				obj = append(obj, config)
				return common.TsaTestSetup(instance, t, nil, NewNtpMonitoringAction(), obj...)
			},
			testCase: func(g Gomega, _ action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				if !g.Expect(instance.Status.NTPMonitoring).ToNot(BeNil()) {
					t.Error("Status NTP Monitoring Config should not be nil")
					return false
				}

				if !g.Expect(instance.Status.NTPMonitoring.Config.NtpConfigRef.Name).To(Equal(instance.Spec.NTPMonitoring.Config.NtpConfigRef.Name)) {
					t.Errorf("Config Map mismatch: expected %v, got %v", instance.Status.NTPMonitoring.Config.NtpConfigRef.Name, instance.Spec.NTPMonitoring.Config.NtpConfigRef.Name)
					return false
				}

				cm := &corev1.ConfigMap{}
				if err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Status.NTPMonitoring.Config.NtpConfigRef.Name, Namespace: instance.GetNamespace()}, cm); err != nil {
					t.Errorf("Unable to find config map: %s", err)
					return false
				}
				return g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Message).To(Equal("NTP monitoring configured"))
			},
		},
		{
			name: "should update configuration",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Creating
				return common.TsaTestSetup(instance, t, nil, NewNtpMonitoringAction(), []client.Object{}...)
			},
			testCase: func(g Gomega, a action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				if !g.Expect(instance.Status.NTPMonitoring).ToNot(BeNil()) {
					t.Error("Status NTP Monitoring Config should not be nil")
					return false
				}
				cm := &corev1.ConfigMap{}
				if err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Status.NTPMonitoring.Config.NtpConfigRef.Name, Namespace: instance.GetNamespace()}, cm); err != nil {
					t.Errorf("Unable to find config map: %s", err)
					return false
				}

				if !g.Expect(instance.Status.NTPMonitoring.Config.NtpConfigRef.Name).To(Equal(cm.Name)) {
					t.Errorf("Config Map name mismatch: expected %v, got %v", instance.Status.NTPMonitoring.Config.NtpConfigRef.Name, cm.Name)
					return false
				}

				instance.Spec.NTPMonitoring.Config.NumServers = 2
				if err := client.Update(context.TODO(), instance); err != nil {
					t.Errorf("Error updating instance: %s", err)
					return false
				}
				_ = a.Handle(context.TODO(), instance)

				if err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, instance); err != nil {
					t.Errorf("Error re-fetching instance: %s", err)
					return false
				}

				if !g.Expect(instance.Spec.NTPMonitoring.Config.NumServers).To(Equal(2)) {
					t.Errorf("NumServers mismatch: expected %v, got %v", 2, instance.Spec.NTPMonitoring.Config.NumServers)
					return false
				}

				return g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Message).To(Equal("NTP monitoring configured"))
			},
		},
		{
			name: "should delete old config",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Creating
				return common.TsaTestSetup(instance, t, nil, NewNtpMonitoringAction(), []client.Object{}...)
			},
			testCase: func(g Gomega, a action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				if !g.Expect(instance.Status.NTPMonitoring).ToNot(BeNil()) {
					t.Error("Status NTP Monitoring Config should not be nil")
					return false
				}

				cm := &corev1.ConfigMap{}
				if err := client.Get(context.TODO(), types.NamespacedName{Name: instance.Status.NTPMonitoring.Config.NtpConfigRef.Name, Namespace: instance.GetNamespace()}, cm); err != nil {
					t.Errorf("Unable to find config map: %s", err)
					return false
				}

				oldConfigMapName := instance.Status.NTPMonitoring.Config.NtpConfigRef.Name

				instance.Spec.NTPMonitoring.Config.NumServers = 2
				if err := client.Update(context.TODO(), instance); err != nil {
					t.Errorf("Error updating instance: %s", err)
					return false
				}
				_ = a.Handle(context.TODO(), instance)

				newConfigMapName := instance.Status.NTPMonitoring.Config.NtpConfigRef.Name
				if !g.Expect(newConfigMapName).ToNot(Equal(oldConfigMapName)) {
					t.Error("New ConfigMap should have a different name from the old ConfigMap")
					return false
				}

				err := client.Get(context.TODO(), types.NamespacedName{Name: oldConfigMapName, Namespace: instance.GetNamespace()}, &corev1.ConfigMap{})
				if !errors.IsNotFound(err) {
					t.Error("Old ConfigMap should be deleted")
					return false
				}

				return g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Message).To(Equal("NTP monitoring configured"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			instance := common.GenerateTSAInstance()
			client, action := tt.setup(instance)
			g.Expect(client).NotTo(BeNil())
			g.Expect(action).NotTo(BeNil())
			g.Expect(tt.testCase(g, action, client, instance)).To(BeTrue())
		})
	}
}
