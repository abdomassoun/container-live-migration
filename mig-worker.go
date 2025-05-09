package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type RestoreRequest struct {
	Filename       string `json:"filename"`
	ContainerImage string `json:"container_image"`
}

func main() {
	http.HandleFunc("/restore", handleRestore)
	log.Println("Target node listening on port 8081...")
	log.Fatal(http.ListenAndServe(":8081", nil))
}

func handleRestore(w http.ResponseWriter, r *http.Request) {
	var req RestoreRequest
	json.NewDecoder(r.Body).Decode(&req)

	log.Println("Restoring from checkpoint:", req.Filename)

	restoreDir := "/tmp/restore"
	diffDir := "/tmp/restore/diff"
	os.MkdirAll(diffDir, 0755)

	run("sudo", "tar", "-xf", req.Filename, "-C", restoreDir)
	run("sudo", "tar", "-xf", restoreDir+"/rootfs-diff.tar", "-C", diffDir)

	containerID := strings.TrimSpace(runOut("sudo", "buildah", "from", req.ContainerImage))
	run("sudo", "buildah", "copy", containerID, diffDir+"/")
	run("sudo", "buildah", "config", "--cmd", "/usr/sbin/nginx -g 'daemon off;'", containerID)
	run("sudo", "buildah", "commit", containerID, "nginx-restored")

	log.Println("Restoration complete. Notifying controller...")

	// Notify controller
	controllerURL := "http://<CONTROLLER-NODE-IP>:8080/migration-complete" // replace or make dynamic
	http.Post(controllerURL, "application/json", strings.NewReader(`{"status":"done"}`))
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func runOut(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		log.Fatalf("Error running %v: %v", args, err)
	}
	return string(out)
}

