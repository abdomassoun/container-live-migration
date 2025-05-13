package main

import (
	"context"
	"fmt"
	"log"
	"net"
	// "os"
	"os/exec"

	pb "github.com/abdomassoun/container-live-migration/pb"

	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedMigrationServiceServer
	k8sToken string
}

func (s *server) RequestCheckpoint(ctx context.Context, req *pb.CheckpointRequest) (*pb.CheckpointResponse, error) {
	url := fmt.Sprintf("https://192.168.122.102:10250/checkpoint/%s/%s/%s", req.Namespace, req.PodName, req.ContainerName)

	cmd := exec.Command("curl", "-k", "-X", "POST",
		"-H", "Authorization: Bearer "+s.k8sToken, url)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return &pb.CheckpointResponse{
			Success: false,
			Message: fmt.Sprintf("Error running curl: %v", err),
		}, nil
	}

	return &pb.CheckpointResponse{
		Success: true,
		Message: "Checkpoint triggered successfully",
		CheckpointTarPath: string(out),
	}, nil
}

func (s *server) NotifyImageBuilt(ctx context.Context, in *pb.ImageBuiltNotification) (*pb.MigrationAck, error) {
	pod := in.GetPodName()
	ns := in.GetNamespace()
	yaml := in.GetYamlManifest()

	delCmd := exec.Command("kubectl", "delete", "pod", pod, "-n", ns)
	if out, err := delCmd.CombinedOutput(); err != nil {
		return &pb.MigrationAck{
			Success: false,
			Message: fmt.Sprintf("Failed to delete pod: %v\nOutput: %s", err, out),
		}, nil
	}

	applyCmd := exec.Command("kubectl", "apply", "-f", yaml)
	if out, err := applyCmd.CombinedOutput(); err != nil {
		return &pb.MigrationAck{
			Success: false,
			Message: fmt.Sprintf("Failed to apply manifest: %v\nOutput: %s", err, out),
		}, nil
	}

	return &pb.MigrationAck{
		Success: true,
		Message: "Pod deleted and redeployed successfully.",
	}, nil
}

func initK8sCheckpointAccess() (string, error) {
	// 1. git clone
	if _, err := exec.Command("git", "clone", "https://github.com/abdomassoun/k8s-checkpoint-access.git").CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to clone repo: %v", err)
	}

	// 2. helm install
	if out, err := exec.Command("helm", "install", "checkpoint-access", "./k8s-checkpoint-access",
		"--namespace", "kube-system", "--create-namespace").CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to install helm chart: %v\nOutput: %s", err, out)
	}

	// 3. get token
	out, err := exec.Command("kubectl", "-n", "kube-system", "create", "token", "checkpoint-sa", "--duration=3h").Output()
	if err != nil {
		return "", fmt.Errorf("failed to create token: %v", err)
	}

	return string(out), nil
}

func main() {
	k8sToken, err := initK8sCheckpointAccess()
	if err != nil {
		log.Fatalf("Initialization failed: %v", err)
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterMigrationServiceServer(s, &server{
		k8sToken: k8sToken,
	})

	fmt.Println("Migration Controller Server running on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
