package container

import (
	"os"
	"strings"
)

// IsContainerised returns true if the current process is likely running inside a container.
// it checks for common container signals like /.dockerenv, container-related cgroup entries,
// and Kubernetes environment variables.
func IsContainerised() bool {
	return hasDockerEnvFile() || isInContainerCGroup() || isInKubernetesPod()
}

// hasDockerEnvFile checks if the /.dockerenv file exists, which _should be_ present in most Docker containers.
func hasDockerEnvFile() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// isInContainerCGroup checks for container-related strings in /proc/1/cgroup (e.g. docker, containerd, kubepods).
func isInContainerCGroup() bool {
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, "docker") ||
		strings.Contains(content, "containerd") ||
		strings.Contains(content, "kubepods")
}

// isInKubernetesPod checks for Kubernetes-specific environment variable.
func isInKubernetesPod() bool {
	return os.Getenv("KUBERNETES_SERVICE_HOST") != ""
}
