package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
)

func TestDevicePluginSingleDevice(t *testing.T) {
	f := features.New("single-device-allocation").
		Assess("node reports devices", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			client := cfg.Client()

			var nodes corev1.NodeList
			if err := client.Resources().List(ctx, &nodes); err != nil {
				t.Fatalf("listing nodes: %v", err)
			}
			if len(nodes.Items) == 0 {
				t.Fatal("no nodes found")
			}

			node := &nodes.Items[0]
			err := waitForResource(client.Resources(), node.Name, resourceName, numDevices, 60*time.Second)
			if err != nil {
				t.Fatalf("waiting for resource on node: %v", err)
			}

			return ctx
		}).
		Assess("pod gets mount", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			client := cfg.Client()

			pod := newTestPod("test-single", 1)
			if err := client.Resources().Create(ctx, pod); err != nil {
				t.Fatalf("creating pod: %v", err)
			}
			t.Cleanup(func() {
				_ = client.Resources().Delete(context.Background(), pod)
			})

			err := wait.For(
				conditions.New(client.Resources()).PodReady(pod),
				wait.WithTimeout(60*time.Second),
			)
			if err != nil {
				t.Fatalf("waiting for pod ready: %v", err)
			}

			stdout, stderr, err := execInPod(cfg, pod.Namespace, pod.Name, "test", []string{"mount"})
			if err != nil {
				t.Fatalf("exec in pod: %v (stderr: %s)", err, stderr)
			}
			if !strings.Contains(stdout, baseDir) {
				t.Fatalf("expected mount containing %s, got:\n%s", baseDir, stdout)
			}
			t.Logf("mount found: %s", extractMountLine(stdout, baseDir))

			return ctx
		}).
		Assess("device directory exists on host", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			out, err := execOnKindNode(clusterName+"-control-plane",
				"sh", "-c", fmt.Sprintf("find %s/vhost-net0/ -mindepth 1 -maxdepth 1 -type d", baseDir))
			if err != nil {
				t.Fatalf("listing device dir on host: %v (output: %s)", err, out)
			}
			out = strings.TrimSpace(out)
			if out == "" {
				t.Fatal("no device directories found on host")
			}

			return ctx
		}).
		Assess("devinfo file exists with correct content", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			out, err := execOnKindNode(clusterName+"-control-plane", "sh", "-c",
				"find /var/run/k8s.cni.cncf.io/devinfo/dp/ -name '*device.json' -exec cat {} \\;")
			if err != nil {
				t.Fatalf("reading devinfo: %v (output: %s)", err, out)
			}
			var devInfo nadv1.DeviceInfo
			err = json.Unmarshal([]byte(out), &devInfo)
			if err != nil {
				t.Fatalf("unmarshaling deviceInfo: %v", err)
			}

			if devInfo.Type != "vhost-user" {
				t.Errorf("devinfo is not type vhost-user: %s", out)
			}
			if !strings.Contains(devInfo.VhostUser.Path, baseDir) {
				t.Errorf("wrong devinfo vhost user path: %s", out)
			}
			if devInfo.VhostUser.Mode != "client" {
				t.Errorf("wrong devinfo vhost user mode: %s", out)
			}

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

func TestDevicePluginMultiDevice(t *testing.T) {
	f := features.New("multi-device-allocation").
		Assess("pod requesting 2 devices gets 2 mounts", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			client := cfg.Client()

			pod := newTestPod("test-multi", 2)
			if err := client.Resources().Create(ctx, pod); err != nil {
				t.Fatalf("creating pod: %v", err)
			}
			t.Cleanup(func() {
				_ = client.Resources().Delete(context.Background(), pod)
			})

			err := wait.For(
				conditions.New(client.Resources()).PodReady(pod),
				wait.WithTimeout(60*time.Second),
			)
			if err != nil {
				t.Fatalf("waiting for pod ready: %v", err)
			}

			stdout, stderr, err := execInPod(cfg, pod.Namespace, pod.Name, "test", []string{"mount"})
			if err != nil {
				t.Fatalf("exec in pod: %v (stderr: %s)", err, stderr)
			}

			count := countLinesWith(stdout, baseDir)
			if count < 2 {
				t.Fatalf("expected at least 2 mounts for %s, got %d\n%s", baseDir, count, stdout)
			}

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

// --- helpers ---

func newTestPod(name string, deviceCount int) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:    "test",
				Image:   "busybox",
				Command: []string{"sleep", "3600"},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName(resourceName): resource.MustParse(fmt.Sprintf("%d", deviceCount)),
					},
				},
			}},
		},
	}
}

func waitForResource(r *resources.Resources, nodeName, resName string, expected int, timeout time.Duration) error {
	condition := func(ctx context.Context) (bool, error) {
		var node corev1.Node
		if err := r.Get(ctx, nodeName, "", &node); err != nil {
			return false, err
		}
		capacity := node.Status.Capacity[corev1.ResourceName(resName)]
		return capacity.Value() == int64(expected), nil
	}

	return wait.For(condition, wait.WithTimeout(timeout))
}

func execInPod(cfg *envconf.Config, namespace, podName, container string, command []string) (string, string, error) {
	args := []string{
		"--kubeconfig", cfg.KubeconfigFile(),
		"-n", namespace,
		"exec", podName,
		"-c", container,
		"--",
	}
	args = append(args, command...)
	cmd := exec.Command("kubectl", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func execOnKindNode(nodeName string, command ...string) (string, error) {
	args := make([]string, 0, 2+len(command))
	args = append(args, "exec", nodeName)
	args = append(args, command...)
	out, err := exec.Command("docker", args...).CombinedOutput()
	return string(out), err
}

func extractMountLine(mountOutput, pattern string) string {
	for _, line := range strings.Split(mountOutput, "\n") {
		if strings.Contains(line, pattern) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func countLinesWith(content, pattern string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, pattern) {
			count++
		}
	}
	return count
}
