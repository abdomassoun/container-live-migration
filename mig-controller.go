package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type MigrationRequest struct {
	SourceNodeIP string `json:"source_node_ip"`
	PodName      string `json:"pod_name"`
	Container    string `json:"container"`
	Namespace    string `json:"namespace"`
}

type CheckpointResponse struct {
	Items []string `json:"items"`
}

func main() {
	http.HandleFunc("/trigger-migration", handleMigrationRequest)
	log.Println("Controller listening on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleMigrationRequest(w http.ResponseWriter, r *http.Request) {
	var req MigrationRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	log.Printf("Received migration request: %+v\n", req)

	// Step 1: Trigger checkpoint on source node
	token := os.Getenv("K8S_TOKEN")
	checkpointURL := fmt.Sprintf("https://%s:10250/checkpoint/%s/%s/%s", req.SourceNodeIP, req.Namespace, req.PodName, req.Container)
	httpReq, _ := http.NewRequest("POST", checkpointURL, nil)
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		http.Error(w, "Failed checkpoint", 500)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var checkpointResp CheckpointResponse
	json.Unmarshal(body, &checkpointResp)
	if len(checkpointResp.Items) == 0 {
		http.Error(w, "No checkpoint file found", 500)
		return
	}
	filename := checkpointResp.Items[0]
	log.Println("Checkpoint file created:", filename)

	// Step 2: Notify target node
	targetNotifyURL := "http://<TARGET-NODE-IP>:8081/restore" // replace or make configurable
	payload := fmt.Sprintf(`{"filename":"%s", "container_image":"%s"}`, filename, req.Container)
	_, err = http.Post(targetNotifyURL, "application/json", strings.NewReader(payload))
	if err != nil {
		log.Println("Failed to notify target node:", err)
	}

	// Step 3: Wait for confirmation (in production, better use long polling, gRPC stream, or WS)
	log.Println("Waiting for confirmation... (mocked here)")
	// This should be a real event listener in production
	select {}

	// Step 4: Redeploy pod on target
	log.Println("Deleting old pod...")
	run("kubectl", "delete", "pod", req.PodName)
	log.Println("Applying new pod spec...")
	run("kubectl", "apply", "-f", "nginx-restored.yaml")
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

