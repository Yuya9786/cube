package task

import (
	"context"
	"io"
	"log"
	"math"
	"os"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
)

type State int

const (
	Pending State = iota
	Scheduled
	Running
	Completed
	Failed
)

type Task struct {
	ID            uuid.UUID
	ContainerId   string
	Name          string
	State         State
	Image         string
	Cpu           float64
	Memory        int64
	Disk          int64
	ExposedPorts  nat.PortSet
	HostPorts     nat.PortMap
	PortBindings  map[string]string
	RestartPolicy string
	StartTime     time.Time
	FinishTime    time.Time
	HealthCheck   string
	RestartCount  int
}

/*
 TasksEvent is an internal object that
 our system uses to trigger tasks from
 one state to another
*/
type TaskEvent struct {
	ID         uuid.UUID
	State      State
	Timestatmp time.Time
	Task       Task
}

type Config struct {
	Name          string
	AttachStdin   bool
	AttachStdout  bool
	AttachStderr  bool
	ExposedPorts  nat.PortSet
	Cmd           []string
	Image         string
	Cpu           float64
	Memory        int64
	Disk          int64
	Env           []string
	RestartPolicy string
}

func NewConfig(task *Task) *Config {
	return &Config{
		Name:          task.Name,
		ExposedPorts:  task.ExposedPorts,
		Image:         task.Image,
		Cpu:           task.Cpu,
		Memory:        task.Memory,
		Disk:          task.Disk,
		RestartPolicy: task.RestartPolicy,
	}
}

type Docker struct {
	Client *client.Client
	Config Config
}

func NewDocker(config *Config) (*Docker, error) {
	dc, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &Docker{
		Client: dc,
		Config: *config,
	}, nil
}

type DockerResult struct {
	Error       error
	Action      string
	ContainerId string
	Result      string
}

type DockerInspectResponse struct {
	Error     error
	Container *types.ContainerJSON
}

func (d *Docker) Run() DockerResult {
	ctx := context.Background()
	reader, err := d.Client.ImagePull(
		ctx, d.Config.Image, types.ImagePullOptions{})
	if err != nil {
		log.Printf("Error pulling image %s: %v\n", d.Config.Image, err)
		return DockerResult{Error: err}
	}
	io.Copy(os.Stdout, reader)

	rp := container.RestartPolicy{
		Name: d.Config.RestartPolicy,
	}

	r := container.Resources{
		Memory:   d.Config.Memory,
		NanoCPUs: int64(d.Config.Cpu * math.Pow(10, 9)),
	}

	cc := container.Config{
		Image:        d.Config.Image,
		Tty:          false,
		Env:          d.Config.Env,
		ExposedPorts: d.Config.ExposedPorts,
	}

	hc := container.HostConfig{
		RestartPolicy:   rp,
		Resources:       r,
		PublishAllPorts: true,
	}

	resp, err := d.Client.ContainerCreate(
		ctx, &cc, &hc, nil, nil, d.Config.Name)
	if err != nil {
		log.Printf("Error creating container using image %s: %v\n", d.Config.Image, err)
		return DockerResult{Error: err}
	}

	if err := d.Client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		log.Printf("Error starting container %s: %v\n", resp.ID, err)
		return DockerResult{Error: err}
	}

	out, err := d.Client.ContainerLogs(
		ctx,
		resp.ID,
		types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		log.Printf("Error getting logs for container %s: %v\n", resp.ID, err)
		return DockerResult{Error: err}
	}

	stdcopy.StdCopy(os.Stdout, os.Stderr, out)

	return DockerResult{ContainerId: resp.ID, Action: "start", Result: "success"}
}

func (d *Docker) Stop(id string) DockerResult {
	log.Printf("Attempting to stop container %v", id)
	ctx := context.Background()
	if err := d.Client.ContainerStop(ctx, id, nil); err != nil {
		log.Printf("Error stopping container %s: %v\n", id, err)
		panic(err)
	}

	removeOptions := types.ContainerRemoveOptions{
		RemoveVolumes: true,
		RemoveLinks:   false,
		Force:         false,
	}

	if err := d.Client.ContainerRemove(ctx, id, removeOptions); err != nil {
		log.Printf("Error removing container %s: %v\n", id, err)
		panic(err)
	}

	return DockerResult{ContainerId: id, Action: "stop", Result: "success"}
}

func (d *Docker) Inspect(id string) DockerInspectResponse {
	ctx := context.Background()
	resp, err := d.Client.ContainerInspect(ctx, id)
	if err != nil {
		log.Printf("Error inspecting container %v, %v\n", id, err)
		return DockerInspectResponse{Error: err}
	}

	return DockerInspectResponse{Container: &resp}

}
