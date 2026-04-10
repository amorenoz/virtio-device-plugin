package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"
)

const (
	resourceName = "virtio.test.io/vhost-net0"
	baseDir      = "/var/run/virtiodp-test"
	numDevices   = 10
)

var (
	testenv     env.Environment
	clusterName = "virtiodp-e2e"
	image       = "localhost/virtio-device-plugin:e2e"
)

func TestMain(m *testing.M) {
	cfg, err := envconf.NewFromFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create config: %v\n", err)
		os.Exit(1)
	}

	testenv = env.NewWithConfig(cfg)

	// Create a kind cluster, load the image and deploy your DP
	testenv.Setup(
		envfuncs.CreateCluster(kind.NewProvider(), clusterName),
		loadImage,
		deployDevicePlugin,
	)
	testenv.Finish(
		envfuncs.DestroyCluster(clusterName),
	)

	os.Exit(testenv.Run(m))
}

func loadImage(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	loadFn := envfuncs.LoadImageToCluster(clusterName, image)
	return loadFn(ctx, cfg)
}

func deployDevicePlugin(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	client := cfg.Client()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "virtio-dp-config",
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"config.json": fmt.Sprintf(`{
				"resourcePrefix": "virtio.test.io",
				"resourceList": [{
					"resourceName": "vhost-net0",
					"numDevices": %d,
					"baseDir": "%s"
				}]
			}`, numDevices, baseDir),
		},
	}
	if err := client.Resources().Create(ctx, cm); err != nil {
		return ctx, fmt.Errorf("creating configmap: %w", err)
	}

	//privileged := true
	hostPathDirOrCreate := corev1.HostPathDirectoryOrCreate
	hostPathDir := corev1.HostPathUnset
	labels := map[string]string{"app": "virtio-device-plugin"}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "virtio-device-plugin",
			Namespace: "kube-system",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:            "virtiodp",
						Image:           image,
						ImagePullPolicy: corev1.PullNever,
						Args:            []string{"--config-file=/etc/virtiodp/config.json", "--log-level=debug"},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "plugins-registry", MountPath: "/var/lib/kubelet/plugins_registry"},
							{Name: "config", MountPath: "/etc/virtiodp"},
							{Name: "base-dir", MountPath: baseDir},
							{Name: "devinfo", MountPath: "/var/run/k8s.cni.cncf.io/devinfo"},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "plugins-registry", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/lib/kubelet/plugins_registry", Type: &hostPathDir}}},
						{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "virtio-dp-config"}}}},
						{Name: "base-dir", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: baseDir, Type: &hostPathDirOrCreate}}},
						{Name: "devinfo", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/k8s.cni.cncf.io/devinfo", Type: &hostPathDirOrCreate}}},
					},
				},
			},
		},
	}
	if err := client.Resources().Create(ctx, ds); err != nil {
		return ctx, fmt.Errorf("creating daemonset: %w", err)
	}

	if err := waitForDaemonSet(ctx, client.Resources(), "kube-system", "virtio-device-plugin", 120*time.Second); err != nil {
		return ctx, fmt.Errorf("waiting for daemonset: %w", err)
	}

	return ctx, nil
}

func waitForDaemonSet(ctx context.Context, r *resources.Resources, namespace, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var ds appsv1.DaemonSet
		if err := r.Get(ctx, name, namespace, &ds); err != nil {
			return err
		}
		if ds.Status.DesiredNumberScheduled > 0 && ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timeout waiting for daemonset %s/%s to be ready", namespace, name)
}
