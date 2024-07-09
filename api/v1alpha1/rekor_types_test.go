package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	_ "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("Rekor", func() {

	Context("RekorSpec", func() {
		It("can be created", func() {
			created := generateRekorObject("rekor-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Rekor{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateRekorObject("rekor-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Rekor{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			var id int64 = 1234567890123456789
			fetched.Spec.TreeID = &id
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateRekorObject("rekor-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), getKey(created), created)).ToNot(Succeed())
		})

		When("changing external access setting", func() {
			It("enabled false->true", func() {
				created := generateRekorObject("rekor-access-1")
				created.Spec.ExternalAccess.Enabled = false
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = true
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateRekorObject("rekor-access-2")
				created.Spec.ExternalAccess.Enabled = true
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = false
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		When("changing Rekor Search UI", func() {
			It("enabled false->true", func() {
				created := generateRekorObject("rekor-ui-1")
				created.Spec.RekorSearchUI.Enabled = pointer.Bool(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.RekorSearchUI.Enabled = pointer.Bool(true)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateRekorObject("rekor-ui-2")
				created.Spec.RekorSearchUI.Enabled = pointer.Bool(true)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.RekorSearchUI.Enabled = pointer.Bool(false)
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		When("changing monitoring", func() {
			It("enabled false->true", func() {
				created := generateRekorObject("rekor-monitoring-1")
				created.Spec.Monitoring.Enabled = false
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Enabled = true
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateRekorObject("rekor-monitoring-2")
				created.Spec.Monitoring.Enabled = true
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Enabled = false
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		Context("is validated", func() {
			It("cron syntax", func() {
				invalidObject := generateRekorObject("backfill-schedule")
				invalidObject.Spec.BackFillRedis.Schedule = "@invalid"

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("schedule in body should match")))
			})

			It("immutable pvc retain", func() {
				validObject := generateRekorObject("immutable-retain")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				invalidObject := &Rekor{}
				Expect(k8sClient.Get(context.Background(), getKey(validObject), invalidObject)).To(Succeed())
				invalidObject.Spec.Pvc.Retain = pointer.Bool(false)

				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("Field is immutable")))
			})

			It("checking pvc name", func() {
				invalidObject := generateRekorObject("rekor3")
				invalidObject.Spec.Pvc.Name = "-invalid-name!"
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("spec.pvc.name in body should match")))
			})
		})

		Context("Default settings", func() {
			var (
				rekorInstance         Rekor
				expectedRekorInstance Rekor
			)

			BeforeEach(func() {
				expectedRekorInstance = *generateRekorObject("foo")
			})

			When("CR spec is empty", func() {
				It("creates CR with defaults", func() {
					rekorInstance = Rekor{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "rekor-defaults",
							Namespace: "default",
						},
					}

					Expect(k8sClient.Create(context.Background(), &rekorInstance)).To(Succeed())
					fetched := &Rekor{}
					Expect(k8sClient.Get(context.Background(), getKey(&rekorInstance), fetched)).To(Succeed())
					Expect(fetched.Spec.Pvc.Name).To(Equal(expectedRekorInstance.Spec.Pvc.Name))
					Expect(fetched.Spec.Pvc.Size).To(Equal(expectedRekorInstance.Spec.Pvc.Size))
					Expect(*fetched.Spec.RekorSearchUI.Enabled).To(BeTrue())
				})
			})

			When("CR is fully populated", func() {
				It("outputs the CR", func() {
					storage := k8sresource.MustParse("987Gi")
					tree := int64(1269875)
					port := int32(8091)

					rekorInstance = Rekor{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "rekor-full-manifest",
							Namespace: "default",
						},
						Spec: RekorSpec{
							Monitoring: MonitoringConfig{
								Enabled: true,
							},
							ExternalAccess: ExternalAccess{
								Enabled: true,
								Host:    "hostname",
							},
							RekorSearchUI: RekorSearchUI{
								Enabled: pointer.Bool(true),
							},
							BackFillRedis: BackFillRedis{
								Enabled:  pointer.Bool(true),
								Schedule: "* */2 * * 0-3",
							},
							TreeID: &tree,
							Pvc: Pvc{
								Name:         "name",
								Size:         &storage,
								StorageClass: "name",
								Retain:       pointer.Bool(true),
							},
							Signer: RekorSigner{
								KMS: "secret",
								KeyRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{
										Name: "secret",
									},
									Key: "key",
								},
								PasswordRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{
										Name: "secret",
									},
									Key: "key",
								},
							},
							Trillian: TrillianService{
								Address: "trillian-system.default.svc",
								Port:    &port,
							},
							TLSCertificate: TLSCert{
								CACertRef: &LocalObjectReference{
									Name: "ca-configmap",
								},
							},
						},
					}

					Expect(k8sClient.Create(context.Background(), &rekorInstance)).To(Succeed())
					fetchedRekor := &Rekor{}
					Expect(k8sClient.Get(context.Background(), getKey(&rekorInstance), fetchedRekor)).To(Succeed())
					Expect(fetchedRekor.Spec).To(Equal(rekorInstance.Spec))
				})
			})

			When("CR is partially set", func() {

				It("sets spec.pvc.storage if spec.pvc is partially set", func() {
					rekorInstance = Rekor{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "rekor-storage",
							Namespace: "default",
						},
						Spec: RekorSpec{
							Pvc: Pvc{
								Name: "custom-name",
							},
						},
					}

					expectedRekorInstance.Spec.Pvc.Name = "custom-name"

					Expect(k8sClient.Create(context.Background(), &rekorInstance)).To(Succeed())
					fetchedRekor := &Rekor{}
					Expect(k8sClient.Get(context.Background(), getKey(&rekorInstance), fetchedRekor)).To(Succeed())
					Expect(fetchedRekor.Spec.Pvc.Name).To(Equal(expectedRekorInstance.Spec.Pvc.Name))
					Expect(fetchedRekor.Spec.Pvc.Size).To(Equal(expectedRekorInstance.Spec.Pvc.Size))
					Expect(*fetchedRekor.Spec.RekorSearchUI.Enabled).To(BeTrue())
				})
			})
		})
	})
})

func generateRekorObject(name string) *Rekor {
	storage := k8sresource.MustParse("5Gi")
	return &Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: RekorSpec{
			BackFillRedis: BackFillRedis{
				Enabled:  pointer.Bool(true),
				Schedule: "0 0 * * *",
			},
			Signer: RekorSigner{
				KMS: "secret",
			},
			Pvc: Pvc{
				Retain: pointer.Bool(true),
				Size:   &storage,
			},
			Trillian: TrillianService{
				Port: pointer.Int32(int32(8091)),
			},
		},
	}
}
