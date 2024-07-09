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

var _ = Describe("Trillian", func() {

	Context("TrillianSpec", func() {
		It("can be created", func() {
			created := generateTrillianObject("trillian-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Trillian{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateTrillianObject("trillian-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Trillian{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			fetched.Spec.Db.Pvc.Name = "new-name"
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateTrillianObject("trillian-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), getKey(created), created)).ToNot(Succeed())
		})

		It("can be created with database secret", func() {
			created := generateTrillianObject("trillian-database-secret")
			created.Spec.Db.DatabaseSecretRef = &LocalObjectReference{
				Name: "database-secret-name",
			}
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())
		})

		Context("is validated", func() {
			It("immutable database create", func() {
				validObject := generateTrillianObject("immutable-create")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				invalidObject := &Trillian{}
				Expect(k8sClient.Get(context.Background(), getKey(validObject), invalidObject)).To(Succeed())
				invalidObject.Spec.Db.Create = pointer.Bool(false)

				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("Field is immutable")))
			})

			It("immutable pvc retain", func() {
				validObject := generateTrillianObject("immutable-retain")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				invalidObject := &Trillian{}
				Expect(k8sClient.Get(context.Background(), getKey(validObject), invalidObject)).To(Succeed())
				invalidObject.Spec.Db.Pvc.Retain = pointer.Bool(false)

				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("Field is immutable")))
			})

			When("database create", func() {
				It("true", func() {
					By("databaseSecretRef is empty", func() {
						validObject := generateTrillianObject("database-secret-1")
						validObject.Spec.Db.Create = pointer.Bool(true)
						validObject.Spec.Db.DatabaseSecretRef = nil
						Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
					})
				})

				It("false", func() {
					By("databaseSecretRef is mandatory", func() {
						invalidObject := generateTrillianObject("database-secret-2")
						invalidObject.Spec.Db.Create = pointer.Bool(false)
						invalidObject.Spec.Db.DatabaseSecretRef = nil
						Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
						Expect(k8sClient.Create(context.Background(), invalidObject)).
							To(MatchError(ContainSubstring("databaseSecretRef cannot be empty")))
					})
				})
			})

			It("checking pvc name", func() {
				invalidObject := generateTrillianObject("trillian3")
				invalidObject.Spec.Db.Pvc.Name = "-invalid-name!"
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("spec.database.pvc.name in body should match")))
			})
		})

		Context("Default settings", func() {
			var (
				trillianInstance         Trillian
				expectedTrillianInstance Trillian
			)

			BeforeEach(func() {
				expectedTrillianInstance = *generateTrillianObject("foo")
			})

			When("CR spec is empty", func() {
				It("creates CR with defaults", func() {
					trillianInstance = Trillian{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "trillian-defaults",
							Namespace: "default",
						},
					}

					Expect(k8sClient.Create(context.Background(), &trillianInstance)).To(Succeed())
					fetched := &Trillian{}
					Expect(k8sClient.Get(context.Background(), getKey(&trillianInstance), fetched)).To(Succeed())
					Expect(fetched.Spec).To(Equal(expectedTrillianInstance.Spec))
				})
			})

			When("CR is fully populated", func() {
				It("outputs the CR", func() {
					storage := k8sresource.MustParse("987Gi")

					trillianInstance = Trillian{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "trillian-full-manifest",
							Namespace: "default",
						},
						Spec: TrillianSpec{
							Db: TrillianDB{
								Create: pointer.Bool(true),
								Pvc: Pvc{
									Retain:       pointer.Bool(true),
									Name:         "storage",
									StorageClass: "storage-class",
									Size:         &storage,
								},
								DatabaseSecretRef: &LocalObjectReference{
									Name: "secret",
								},
							},
							TrillianServer: TrillianServer{
								TLSCertificate: TLSCert{
									CertRef: &SecretKeySelector{
										Key:                  "cert",
										LocalObjectReference: LocalObjectReference{Name: "server-secret"},
									},
									PrivateKeyRef: &SecretKeySelector{
										Key:                  "key",
										LocalObjectReference: LocalObjectReference{Name: "server-secret"},
									},
								},
							},
							TrillianSigner: TrillianSigner{
								TLSCertificate: TLSCert{
									CertRef: &SecretKeySelector{
										Key:                  "cert",
										LocalObjectReference: LocalObjectReference{Name: "signer-secret"},
									},
									PrivateKeyRef: &SecretKeySelector{
										Key:                  "key",
										LocalObjectReference: LocalObjectReference{Name: "signer-secret"},
									},
								},
							},
						},
					}

					Expect(k8sClient.Create(context.Background(), &trillianInstance)).To(Succeed())
					fetchedTrillian := &Trillian{}
					Expect(k8sClient.Get(context.Background(), getKey(&trillianInstance), fetchedTrillian)).To(Succeed())
					Expect(fetchedTrillian.Spec).To(Equal(trillianInstance.Spec))
				})
			})

			When("CR is partially set", func() {

				It("sets spec.persistence.storage if spec.persistence is partially set", func() {

					trillianInstance = Trillian{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "trillian-storage",
							Namespace: "default",
						},
						Spec: TrillianSpec{
							Db: TrillianDB{
								DatabaseSecretRef: &LocalObjectReference{
									Name: "secret",
								},
							},
						},
					}

					expectedTrillianInstance.Spec.Db.DatabaseSecretRef = &LocalObjectReference{
						Name: "secret",
					}

					Expect(k8sClient.Create(context.Background(), &trillianInstance)).To(Succeed())
					fetchedTrillian := &Trillian{}
					Expect(k8sClient.Get(context.Background(), getKey(&trillianInstance), fetchedTrillian)).To(Succeed())
					Expect(fetchedTrillian.Spec).To(Equal(expectedTrillianInstance.Spec))
				})
			})
		})
	})
})

func generateTrillianObject(name string) *Trillian {
	storage := k8sresource.MustParse("5Gi")
	return &Trillian{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: TrillianSpec{
			Db: TrillianDB{
				Create: pointer.Bool(true),
				Pvc: Pvc{
					Retain: pointer.Bool(true),
					Size:   &storage,
				},
			},
		},
	}
}
