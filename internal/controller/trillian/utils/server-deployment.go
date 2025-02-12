package trillianUtils

import (
	"errors"
	"strconv"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func EnsureServerDeployment(instance *v1alpha1.Trillian, image string, name string, sa string, labels map[string]string, args ...string) func(deployment *apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		if instance.Status.Db.DatabaseSecretRef == nil {
			return errors.New("reference to database secret is not set")
		}

		spec := &dp.Spec
		spec.Replicas = utils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		initContainer := kubernetes.FindInitContainerByNameOrCreate(&template.Spec, "wait-for-trillian-db")
		initContainer.Image = constants.TrillianNetcatImage

		hostnameEnv := kubernetes.FindEnvByNameOrCreate(initContainer, "MYSQL_HOSTNAME")
		hostnameEnv.ValueFrom = &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{
				Key: actions.SecretHost,
				LocalObjectReference: core.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}

		portEnv := kubernetes.FindEnvByNameOrCreate(initContainer, "MYSQL_PORT")
		portEnv.ValueFrom = &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{
				Key: actions.SecretPort,
				LocalObjectReference: core.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}
		initContainer.Command = []string{
			"sh",
			"-c",
			"until nc -z -v -w30 $MYSQL_HOSTNAME $MYSQL_PORT; do echo \"Waiting for MySQL to start\"; sleep 5; done;",
		}

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, name)
		container.Image = image

		container.Args = append([]string{
			"--storage_system=mysql",
			"--quota_system=mysql",
			"--mysql_uri=$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOSTNAME):$(MYSQL_PORT))/$(MYSQL_DATABASE)",
			"--rpc_endpoint=0.0.0.0:" + strconv.Itoa(int(actions.ServerPort)),
			"--http_endpoint=0.0.0.0:" + strconv.Itoa(int(actions.MetricsPort)),
			"--alsologtostderr",
		}, args...)

		//Ports = containerPorts
		// Env variables from secret trillian-mysql
		userEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_USER")
		userEnv.ValueFrom = &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{
				Key: actions.SecretUser,
				LocalObjectReference: core.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}

		passwordEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_PASSWORD")
		passwordEnv.ValueFrom = &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{
				Key: actions.SecretPassword,
				LocalObjectReference: core.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}

		hostEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_HOSTNAME")
		hostEnv.ValueFrom = &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{
				Key: actions.SecretHost,
				LocalObjectReference: core.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}

		containerPortEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_PORT")
		containerPortEnv.ValueFrom = &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{
				Key: actions.SecretPort,
				LocalObjectReference: core.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}

		dbEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_DATABASE")
		dbEnv.ValueFrom = &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{
				Key: actions.SecretDatabaseName,
				LocalObjectReference: core.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}

		podNameEnv := kubernetes.FindEnvByNameOrCreate(container, "POD_NAME")
		podNameEnv.ValueFrom = &core.EnvVarSource{
			FieldRef: &core.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.name",
			},
		}

		namespaceEnv := kubernetes.FindEnvByNameOrCreate(container, "NAMESPACE")
		namespaceEnv.ValueFrom = &core.EnvVarSource{
			FieldRef: &core.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.namespace",
			},
		}

		port := kubernetes.FindPortByNameOrCreate(container, "8091-tcp")
		port.ContainerPort = 8091
		port.Protocol = core.ProtocolTCP

		if instance.Spec.Monitoring.Enabled {
			monitoring := kubernetes.FindPortByNameOrCreate(container, "monitoring")
			monitoring.ContainerPort = 8090
			monitoring.Protocol = core.ProtocolTCP
		}
		return nil
	}
}
