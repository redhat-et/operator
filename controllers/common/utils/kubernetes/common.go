package kubernetes

import (
	"os"
	"path/filepath"
)

const (
	inContainerNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	kubeConfigEnvVar         = "KUBECONFIG"
)

func FilterCommonLabels(labels map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range labels {
		if key == "app.kubernetes.io/part-of" || key == "app.kubernetes.io/instance" {
			out[key] = value
		}
	}
	return out
}

func getDefaultKubeConfigFile() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, ".kube", "config"), nil
}

func ContainerMode() (bool, error) {
	// When kube config is set, container mode is not used
	if os.Getenv(kubeConfigEnvVar) != "" {
		return false, nil
	}
	// Use container mode only when the kubeConfigFile does not exist and the container namespace file is present
	configFile, err := getDefaultKubeConfigFile()
	if err != nil {
		return false, err
	}
	configFilePresent := true
	_, err = os.Stat(configFile)
	if err != nil && os.IsNotExist(err) {
		configFilePresent = false
	} else if err != nil {
		return false, err
	}
	if !configFilePresent {
		_, err := os.Stat(inContainerNamespaceFile)
		if os.IsNotExist(err) {
			return false, nil
		}
		return true, err
	}
	return false, nil
}
