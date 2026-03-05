package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	tekton "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	triggersv1beta1 "github.com/tektoncd/triggers/pkg/apis/triggers/v1beta1"
	triggersclient "github.com/tektoncd/triggers/pkg/client/clientset/versioned"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// getIngresses returns Ingresses that match the given owner, repo, and event type
// using labels: pipeline.tekton.dev/owner, pipeline.tekton.dev/repo, pipeline.tekton.dev/trigger.
// Uses in-cluster config when available, otherwise KUBECONFIG.

func kubeAuthConfig() (*kubernetes.Clientset, context.Context, error) {
	config, err := restConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("rest config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("kube client: %w", err)
	}

	ctx := context.Background()

	return clientset, ctx, nil
}

func tektonAuthConfig() (*tekton.Clientset, context.Context, error) {
	config, err := restConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("rest config: %w", err)
	}

	clientset, err := tekton.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("tekton client: %w", err)
	}

	ctx := context.Background()

	return clientset, ctx, nil
}

func triggersAuthConfig() (*triggersclient.Clientset, context.Context, error) {
	config, err := restConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("rest config: %w", err)
	}

	clientset, err := triggersclient.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("triggers client: %w", err)
	}

	ctx := context.Background()

	return clientset, ctx, nil
}

func getIngresses(owner, repo, eventType string) ([]map[string]any, error) {
	clientset, ctx, err := kubeAuthConfig()
	if err != nil {
		return nil, fmt.Errorf("auth config: %w", err)
	}

	selector := fmt.Sprintf("pipeline.tekton.dev/owner=%s,pipeline.tekton.dev/repo=%s,pipeline.tekton.dev/trigger=%s",
		owner, repo, eventType)

	list, err := clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list ingresses: %w", err)
	}

	out := make([]map[string]any, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		m, err := ingressToMap(item)
		if err != nil {
			return nil, fmt.Errorf("ingress %s: %w", item.Name, err)
		}
		out = append(out, m)
	}
	return out, nil
}

func restConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}
	return clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
}

// ingressToMap converts a typed Ingress into map[string]any via JSON round-trip
// so the result matches the usual Kubernetes resource shape (metadata, spec).
func ingressToMap(obj *networkingv1.Ingress) (map[string]any, error) {
	encoded, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(encoded, &m); err != nil {
		return nil, err
	}
	delete(m, "status")
	if meta, ok := m["metadata"].(map[string]any); ok {
		delete(meta, "managedFields")
	}
	return m, nil
}

func init() {
	_ = scheme.Scheme
}

func GetIngresses(owner, repo, eventType string) ([]map[string]any, error) {
	return getIngresses(owner, repo, eventType)
}

// IngressTriggerURLs returns the URLs (scheme + host + path) you can POST to in order to
// trigger the pipeline. Pass the slice returned by getIngresses/GetIngresses.
// If scheme is empty, "https" is used.
func IngressTriggerURLs(ingresses []map[string]any, scheme string) []string {
	if scheme == "" {
		scheme = "https"
	}
	var urls []string
	for _, ing := range ingresses {
		spec, _ := ing["spec"].(map[string]any)
		if spec == nil {
			continue
		}
		rules, _ := spec["rules"].([]any)
		for _, r := range rules {
			rule, _ := r.(map[string]any)
			if rule == nil {
				continue
			}
			host, _ := rule["host"].(string)
			httpPart, _ := rule["http"].(map[string]any)
			if httpPart == nil {
				continue
			}
			paths, _ := httpPart["paths"].([]any)
			if len(paths) == 0 {
				urls = append(urls, fmt.Sprintf("%s://%s/", scheme, host))
				continue
			}
			for _, p := range paths {
				pathObj, _ := p.(map[string]any)
				path := "/"
				if pathObj != nil {
					if v, _ := pathObj["path"].(string); v != "" {
						path = v
					}
				}
				if !strings.HasPrefix(path, "/") {
					path = "/" + path
				}
				urls = append(urls, fmt.Sprintf("%s://%s%s", scheme, host, path))
			}
		}
	}
	return urls
}

func eventListenerToMap(obj *triggersv1beta1.EventListener) (map[string]any, error) {
	encoded, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(encoded, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func getEventListeners(owner, repo, eventType string) ([]map[string]any, error) {
	clientset, ctx, err := triggersAuthConfig()
	if err != nil {
		return nil, fmt.Errorf("auth config: %w", err)
	}

	selector := fmt.Sprintf("pipeline.tekton.dev/owner=%s,pipeline.tekton.dev/repo=%s,pipeline.tekton.dev/trigger=%s",
		owner, repo, eventType)

	list, err := clientset.TriggersV1beta1().EventListeners("").List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list event listeners: %w", err)
	}

	out := make([]map[string]any, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		m, err := eventListenerToMap(item)
		if err != nil {
			return nil, fmt.Errorf("event listener %s: %w", item.Name, err)
		}
		out = append(out, m)
	}
	return out, nil
}

func GetEventListeners(owner, repo, eventType string) ([]map[string]any, error) {
	return getEventListeners(owner, repo, eventType)
}

func GetEventListenerURLs(eventListeners []map[string]any) ([]string, error) {
	out := make([]string, 0, len(eventListeners))

	for _, eventListener := range eventListeners {
		status, ok := eventListener["status"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("status not found")
		}

		address, ok := status["address"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("address not found")
		}

		url, ok := address["url"].(string)
		if !ok {
			return nil, fmt.Errorf("url not found")
		}
		out = append(out, url)
	}
	return out, nil
}

func TriggerPipeline(url string, body any) (string, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal body: %w", err)
	}

	bodyBytes := bytes.NewBuffer(jsonBody)
	bodyReader := bytes.NewReader(bodyBytes.Bytes())

	req, err := http.NewRequest(http.MethodPost, url, bodyReader)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("status code: %d", resp.StatusCode)
	}

	body_, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	var result struct {
		EventID string `json:"eventID"`
	}
	if err := json.Unmarshal(body_, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return result.EventID, nil
}

func GetPipelineRuns(eventId string) ([]map[string]any, error) {
	clientset, ctx, err := tektonAuthConfig()
	if err != nil {
		return nil, fmt.Errorf("auth config: %w", err)
	}

	selector := fmt.Sprintf("triggers.tekton.dev/triggers-eventid=%s", eventId)

	list, err := clientset.TektonV1().PipelineRuns("").List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list pipeline runs: %w", err)
	}

	out := make([]map[string]any, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		m, err := pipelineRunToMap(item)
		if err != nil {
			return nil, fmt.Errorf("pipeline run %s: %w", item.Name, err)
		}
		out = append(out, m)
	}

	return out, nil
}

func pipelineRunToMap(obj *tektonv1.PipelineRun) (map[string]any, error) {
	encoded, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(encoded, &m); err != nil {
		return nil, err
	}
	// keep status so GetPipelineRunStatus can read it
	if meta, ok := m["metadata"].(map[string]any); ok {
		delete(meta, "managedFields")
	}
	return m, nil
}

func GetPipelineRunStatus(pipelineRun map[string]any) (map[string]any, error) {
	status, ok := pipelineRun["status"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("status not found")
	}
	return status, nil
}

func GetPipelineRunsStatus(pipelineRuns []map[string]any) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(pipelineRuns))
	for _, pipelineRun := range pipelineRuns {
		status, err := GetPipelineRunStatus(pipelineRun)
		if err != nil {
			return nil, fmt.Errorf("get pipeline run status: %w", err)
		}
		out = append(out, status)
	}
	return out, nil
}

func PipelineRunCompleted(status map[string]any) bool {
	conditions, ok := status["conditions"].([]any)
	if !ok {
		return false
	}

	for _, condition := range conditions {
		condition, ok := condition.(map[string]any)
		if !ok {
			continue
		}

		if !(condition["reason"] == "Succeeded" && condition["status"] == "True" && condition["type"] == "Succeeded") {
			return false
		}
	}
	return true
}

func PipelineRunsCompleted(statuses []map[string]any) bool {
	for _, status := range statuses {
		completed := PipelineRunCompleted(status)
		if !completed {
			return false
		}
	}
	return true
}

func DisplayPipelineRunStatus(pipelineRuns []map[string]any) {
	for _, pipelineRun := range pipelineRuns {
		status, err := GetPipelineRunStatus(pipelineRun)
		if err != nil {
			fmt.Printf("[ERROR] - get pipeline run status: %v\n", err)
			return
		}
		conditions, ok := status["conditions"].([]any)
		if !ok {
			fmt.Println("[ERROR] - conditions not found")
			return
		}

		for _, condition := range conditions {
			condition, ok := condition.(map[string]any)
			if !ok {
				continue
			}
			fmt.Printf("[%s] %s\n", pipelineRun["metadata"].(map[string]any)["name"], condition["reason"])
		}

	}
}
