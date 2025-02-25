package actions

import (
	"testing"

	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/controller/labels"
	"golang.org/x/exp/maps"
	v13 "k8s.io/api/apps/v1"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/api/v1alpha1"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	componentName  = "component"
	deploymentName = "instance"

	rbacName = "fulcio"
)

func TestSimpleDeploymen(t *testing.T) {
	g := NewWithT(t)
	instance := createInstance()
	labels := labels.For(componentName, DeploymentName, instance.Name)
	deployment, err := createDeployment(instance, labels)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(deployment).ShouldNot(BeNil())
	g.Expect(deployment.Labels).Should(Equal(labels))
	g.Expect(deployment.Name).Should(Equal(DeploymentName))
	g.Expect(deployment.Spec.Template.Spec.ServiceAccountName).Should(Equal(rbacName))

	// private key password
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).ShouldNot(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("PASSWORD"),
	})), "PASSWORD env should not be set")

	// oidc-info volume
	oidcVolume := findVolume("ca-trust", deployment.Spec.Template.Spec.Volumes)
	g.Expect(oidcVolume).ShouldNot(BeNil())
	g.Expect(oidcVolume.VolumeSource.Projected.Sources).Should(BeEmpty())
}

func TestPrivateKeyPassword(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Status.Certificate.PrivateKeyPasswordRef = &v1alpha1.SecretKeySelector{
		LocalObjectReference: v1alpha1.LocalObjectReference{
			Name: "secret",
		},
		Key: "key",
	}
	labels := labels.For(componentName, deploymentName, instance.Name)
	deployment, err := createDeployment(instance, labels)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(deployment).ShouldNot(BeNil())

	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("PASSWORD"),
	})), "PASSWORD env should be set")
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("SSL_CERT_DIR"),
	})))
}

func TestTrustedCA(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Spec.TrustedCA = &v1alpha1.LocalObjectReference{Name: "trusted"}
	labels := labels.For(componentName, deploymentName, instance.Name)
	deployment, err := createDeployment(instance, labels)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(deployment).ShouldNot(BeNil())

	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("SSL_CERT_DIR"),
	})))

	oidcVolume := findVolume("ca-trust", deployment.Spec.Template.Spec.Volumes)
	g.Expect(oidcVolume).ShouldNot(BeNil())
	g.Expect(oidcVolume.VolumeSource.Projected.Sources).Should(HaveLen(1))
	g.Expect(oidcVolume.VolumeSource.Projected.Sources[0].ConfigMap.Name).Should(Equal("trusted"))
}

func TestTrustedCAByAnnotation(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Annotations = make(map[string]string)
	instance.Annotations[annotations.TrustedCA] = "trusted-annotation"
	labels := labels.For(componentName, deploymentName, instance.Name)
	deployment, err := createDeployment(instance, labels)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(deployment).ShouldNot(BeNil())

	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("SSL_CERT_DIR"),
	})))

	oidcVolume := findVolume("ca-trust", deployment.Spec.Template.Spec.Volumes)
	g.Expect(oidcVolume).ShouldNot(BeNil())
	g.Expect(oidcVolume.VolumeSource.Projected.Sources).Should(HaveLen(1))
	g.Expect(oidcVolume.VolumeSource.Projected.Sources[0].ConfigMap.Name).Should(Equal("trusted-annotation"))
}

func TestMissingPrivateKey(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Status.Certificate.PrivateKeyRef = nil
	labels := labels.For(componentName, deploymentName, instance.Name)
	deployment, err := createDeployment(instance, labels)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(deployment).Should(BeNil())
}

func TestCtlogConfig(t *testing.T) {
	tests := []struct {
		name   string
		args   v1alpha1.CtlogService
		verify func(Gomega, *v13.Deployment, error)
	}{
		{
			name: "missing address",
			args: v1alpha1.CtlogService{
				Port:   ptr.To(int32(1234)),
				Prefix: "prefix",
			},
			verify: func(g Gomega, deployment *v13.Deployment, err error) {
				g.Expect(err).Should(HaveOccurred())
				g.Expect(err).Should(MatchError(utils.CtlogAddressNotSpecified))
			},
		},
		{
			name: "missing port",
			args: v1alpha1.CtlogService{
				Address: "http://address",
				Prefix:  "prefix",
			},
			verify: func(g Gomega, deployment *v13.Deployment, err error) {
				g.Expect(err).Should(HaveOccurred())
				g.Expect(err).Should(MatchError(utils.CtlogPortNotSpecified))
			},
		},
		{
			name: "missing prefix",
			args: v1alpha1.CtlogService{
				Address: "http://address",
				Port:    ptr.To(int32(1234)),
			},
			verify: func(g Gomega, deployment *v13.Deployment, err error) {
				g.Expect(err).Should(HaveOccurred())
				g.Expect(err).Should(MatchError(utils.CtlogPrefixNotSpecified))
			},
		},
		{
			name: "valid",
			args: v1alpha1.CtlogService{
				Address: "http://address",
				Port:    ptr.To(int32(1234)),
				Prefix:  "prefix",
			},
			verify: func(g Gomega, deployment *v13.Deployment, err error) {
				g.Expect(err).Should(Succeed())
				g.Expect(deployment.Spec.Template.Spec.Containers[0].Args).Should(ContainElement(Equal("--ct-log-url=http://address:1234/prefix")))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			instance := createInstance()
			instance.Spec.Ctlog = tt.args
			deployment, err := createDeployment(instance, map[string]string{})
			tt.verify(g, deployment, err)
		})
	}
}

func findVolume(name string, volumes []v12.Volume) *v12.Volume {
	for _, v := range volumes {
		if v.Name == name {
			return &v
		}
	}
	return nil
}

func createInstance() *v1alpha1.Fulcio {
	port := int32(80)
	return &v1alpha1.Fulcio{
		ObjectMeta: v1.ObjectMeta{
			Name:      "name",
			Namespace: "default",
		},
		Spec: v1alpha1.FulcioSpec{
			Ctlog: v1alpha1.CtlogService{
				Address: "http://ctlog.default.svc",
				Port:    &port,
				Prefix:  "prefix",
			},
		},
		Status: v1alpha1.FulcioStatus{
			ServerConfigRef: &v1alpha1.LocalObjectReference{Name: "config"},
			Certificate: &v1alpha1.FulcioCert{
				PrivateKeyRef: &v1alpha1.SecretKeySelector{
					Key:                  "private",
					LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
				},
				CARef: &v1alpha1.SecretKeySelector{
					Key:                  "cert",
					LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
				},
			},
		},
	}
}

func createDeployment(instance *v1alpha1.Fulcio, labels map[string]string) (*v13.Deployment, error) {
	testAction := deployAction{}
	d := &v13.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      DeploymentName,
			Namespace: instance.Namespace,
		},
	}

	ensures := []func(*v13.Deployment) error{
		testAction.ensureDeployment(instance, RBACName, labels),
		ensure.Labels[*v13.Deployment](maps.Keys(labels), labels),
		deployment.Proxy(),
		deployment.TrustedCA(instance.GetTrustedCA(), "fulcio-server"),
	}
	for _, en := range ensures {
		err := en(d)
		if err != nil {
			return nil, err
		}
	}
	return d, nil
}
