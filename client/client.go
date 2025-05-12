package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	import pb "github.com/abdomassoun/container-live-migration/pb"

	"google.golang.org/grpc"
)

func main() {
	conn, err := grpc.Dial("192.168.122.101:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewMigrationServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Ask controller to checkpoint the pod
	resp, err := client.RequestCheckpoint(ctx, &pb.CheckpointRequest{
		Namespace:      "default",
		PodName:        "nginx-pod",
		ContainerName:  "nginx",
		K8SToken:       os.Getenv("K8S_TOKEN"),
	})
	if err != nil || !resp.Success {
		log.Fatalf("Checkpoint request failed: %v", resp.Message)
	}
	log.Println("Checkpoint received at:", resp.CheckpointTarPath)

	// Simulate steps 2-6 (untar, buildah...) before reporting back
	// Assume nginx-restored image and nginx-restored.yaml are ready
	yaml := `
apiVersion: v1
kind: Pod
metadata:
    name: nginx-restored
    namespace: default
    labels:
     app: test
spec:
    containers:
    - name: nginx
      image: localhost/nginx-restored
      imagePullPolicy: Never
      ports:
      - containerPort: 80
    nodeSelector:
      kubernetes.io/hostname: worker-node2
    shareProcessNamespace: true
`

	// Step 2: Notify image built
	ack, err := client.NotifyImageBuilt(context.Background(), &pb.ImageBuiltNotification{
		ImageName:    "nginx-restored",
		Namespace:    "default",
		PodName:      "nginx-pod",
		YamlManifest: yaml,
	})
	if err != nil || !ack.Success {
		log.Fatalf("Image notification failed: %v", ack.Message)
	}
	log.Println("Migration completed:", ack.Message)
}

