package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	pb "github.com/abdomassoun/container-live-migration/pb"

	"google.golang.org/grpc"
)

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	conn, err := grpc.Dial("192.168.122.101:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewMigrationServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Request checkpoint
	resp, err := client.RequestCheckpoint(ctx, &pb.CheckpointRequest{
		Namespace:     "default",
		PodName:       "nginx-pod",
		ContainerName: "nginx",
		K8SToken:      os.Getenv("K8S_TOKEN"),
	})
	if err != nil || !resp.Success {
		log.Fatalf("Checkpoint request failed: %v", resp.Message)
	}
	log.Println("Checkpoint received at:", resp.CheckpointTarPath)

	checkpointFile := resp.CheckpointTarPath
	checkpointPath := filepath.Join("/mnt/nfs/checkpoints", checkpointFile)

	// Step 2: Untar the checkpoint to /tmp/restore
	restoreDir := "/tmp/restore"
	diffDir := filepath.Join(restoreDir, "diff")
	os.MkdirAll(diffDir, 0755)

	if err := runCommand("sudo", "tar", "-xf", checkpointPath, "-C", restoreDir); err != nil {
		log.Fatalf("Failed to extract checkpoint: %v", err)
	}

	// Step 3: Extract rootfs-diff.tar to diff/
	if err := runCommand("sudo", "tar", "-xf", filepath.Join(restoreDir, "rootfs-diff.tar"), "-C", diffDir); err != nil {
		log.Fatalf("Failed to extract rootfs-diff.tar: %v", err)
	}

	// Step 4: Create container from base image
	newContainerOut, err := exec.Command("sudo", "buildah", "from", "nginx").Output()
	if err != nil {
		log.Fatalf("Failed to create container: %v", err)
	}
	newContainer := strings.TrimSpace(string(newContainerOut))

	// Step 5: Copy diff into the container
	if err := runCommand("sudo", "buildah", "copy", newContainer, diffDir); err != nil {
		log.Fatalf("Failed to copy diff: %v", err)
	}

	// Step 6: Configure the container
	if err := runCommand("sudo", "buildah", "config", "--cmd", "/usr/sbin/nginx -g 'daemon off;'", newContainer); err != nil {
		log.Fatalf("Failed to configure container: %v", err)
	}

	// Step 7: Commit the image
	if err := runCommand("sudo", "buildah", "commit", newContainer, "nginx-restored"); err != nil {
		log.Fatalf("Failed to commit image: %v", err)
	}

	// Step 8: Notify the controller
	ack, err := client.NotifyImageBuilt(context.Background(), &pb.ImageBuiltNotification{
		ImageName:    "nginx-restored",
		Namespace:    "default",
		PodName:      "nginx-pod",
		YamlManifest: "", // <-- YAML not needed, controller handles it
	})
	if err != nil || !ack.Success {
		log.Fatalf("Image notification failed: %v", ack.Message)
	}
	log.Println("Migration completed:", ack.Message)
}
