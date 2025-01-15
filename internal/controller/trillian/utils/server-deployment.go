package trillianUtils

import (
	"context"
	"errors"
	"strconv"

	"github.com/securesign/operator/internal/images"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateLogServerDeployment(ctx context.Context, client client.Client, instance *v1alpha1.Trillian, image string, dpName string, sa string, labels map[string]string) (*apps.Deployment, error) {
	if instance.Status.Db.DatabaseSecretRef == nil {
		return nil, errors.New("reference to database secret is not set")
	}
	replicas := int32(1)
	containerPorts := []core.ContainerPort{
		{
			Protocol:      core.ProtocolTCP,
			ContainerPort: 8091,
		},
	}

	if instance.Spec.Monitoring.Enabled {
		containerPorts = append(containerPorts, core.ContainerPort{
			Protocol:      core.ProtocolTCP,
			ContainerPort: 8090,
		})
	}

	dep := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: core.PodSpec{
					ServiceAccountName: sa,
					InitContainers: []core.Container{
						{
							Name:  "wait-for-trillian-db",
							Image: images.Registry.Get(images.TrillianNetcat),
							Env: []core.EnvVar{
								{
									Name: "MYSQL_HOSTNAME",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: SecretHost,
											LocalObjectReference: core.LocalObjectReference{
												Name: instance.Status.Db.DatabaseSecretRef.Name,
											},
										},
									},
								},
								{
									Name: "MYSQL_PORT",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: SecretPort,
											LocalObjectReference: core.LocalObjectReference{
												Name: instance.Status.Db.DatabaseSecretRef.Name,
											},
										},
									},
								},
							},
							Command: []string{
								"sh",
								"-c",
								"until nc -z -v -w30 $MYSQL_HOSTNAME $MYSQL_PORT; do echo \"Waiting for MySQL to start\"; sleep 5; done;",
							},
						},
					},
					Containers: []core.Container{
						{
							Args: []string{
								"--storage_system=mysql",
								"--quota_system=mysql",
								"--mysql_uri=$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOSTNAME):$(MYSQL_PORT))/$(MYSQL_DATABASE)",
								"--rpc_endpoint=0.0.0.0:" + strconv.Itoa(int(actions.ServerPort)),
								"--http_endpoint=0.0.0.0:" + strconv.Itoa(int(actions.MetricsPort)),
								"--alsologtostderr",
							},
							Name:  dpName,
							Image: image,
							Ports: containerPorts,
							// Env variables from secret trillian-mysql
							Env: []core.EnvVar{
								{
									Name: "MYSQL_USER",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: SecretUser,
											LocalObjectReference: core.LocalObjectReference{
												Name: instance.Status.Db.DatabaseSecretRef.Name,
											},
										},
									},
								},
								{
									Name: "MYSQL_PASSWORD",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: SecretPassword,
											LocalObjectReference: core.LocalObjectReference{
												Name: instance.Status.Db.DatabaseSecretRef.Name,
											},
										},
									},
								},
								{
									Name: "MYSQL_HOSTNAME",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: SecretHost,
											LocalObjectReference: core.LocalObjectReference{
												Name: instance.Status.Db.DatabaseSecretRef.Name,
											},
										},
									},
								},
								{
									Name: "MYSQL_PORT",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: SecretPort,
											LocalObjectReference: core.LocalObjectReference{
												Name: instance.Status.Db.DatabaseSecretRef.Name,
											},
										},
									},
								},
								{
									Name: "MYSQL_DATABASE",
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											Key: SecretDatabaseName,
											LocalObjectReference: core.LocalObjectReference{
												Name: instance.Status.Db.DatabaseSecretRef.Name,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// TLS communication to database
	if UseTLS(instance) {
		caPath, err := CAPath(ctx, client, instance)
		if err != nil {
			return nil, errors.New("failed to get CA path: " + err.Error())
		}

		dep.Spec.Template.Spec.Containers[0].Args = append(dep.Spec.Template.Spec.Containers[0].Args, "--mysql_tls_ca", caPath)

		mysqlServerName := "$(MYSQL_HOSTNAME)." + instance.Namespace + ".svc"
		if !*instance.Spec.Db.Create {
			mysqlServerName = "$(MYSQL_HOSTNAME)"
		}
		dep.Spec.Template.Spec.Containers[0].Args = append(dep.Spec.Template.Spec.Containers[0].Args, "--mysql_server_name", mysqlServerName)
	}

	utils.SetProxyEnvs(dep)
	return dep, nil
}
