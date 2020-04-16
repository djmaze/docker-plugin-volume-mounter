package main

import (
  "context"
  "fmt"
  "time"
  "path/filepath"
  "os"
  "os/exec"
  "os/signal"
  "strings"
  "syscall"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
  "github.com/docker/docker/client"
)

const DOCKER_CLIENT_VERSION = "1.39"
const PLUGIN_ROOT_PATH_TEMPLATE = "/var/lib/docker/plugins/%s"
const VOLUME_ROOT_PATH_TEMPLATE = PLUGIN_ROOT_PATH_TEMPLATE + "/propagated-mount"
const API_TIMEOUT = 5000 * time.Millisecond
const VolumePathRoot = "/mnt/docker-volumes"

type Mounter struct {
  cli *client.Client
}

func targetPath(volumeName string) string {
  return filepath.Join(VolumePathRoot, volumeName)
}

func newMounter(cli *client.Client) Mounter {
  return Mounter {
    cli: cli,
  }
}

func (mounter Mounter) MountCurrentVolumes() {
  d := time.Now().Add(API_TIMEOUT)
  ctx, cancel := context.WithDeadline(context.Background(), d)
  defer cancel()

  volumes, err := mounter.cli.VolumeList(ctx, filters.Args {})
  if err != nil {
    panic(err)
  }

  for _, volume := range volumes.Volumes {
    if volume.Driver != "local" {
      volume_path := mounter.getVolumePath(ctx, volume)
      fmt.Println("Checking for volume path " + volume_path)
      _, err := exec.Command("sh", "-c", "mount | grep " + volume_path).Output()
      if err == nil {
        mounter.MountVolume(ctx, volume)
      }
    }
  }
}

func (mounter Mounter) UnmountCurrentVolumes() {
  _, err := exec.Command("sh", "-c", "mount | grep " + VolumePathRoot).Output()
  if err == nil {
    fmt.Println("TERM signal received. Unmounting volumes")

    _, err = exec.Command("sh", "-c", "umount " + filepath.Join(VolumePathRoot, "*")).Output()
    if err != nil {
      panic(err)
    }

    _, err = exec.Command("sh", "-c", "rmdir " + filepath.Join(VolumePathRoot, "*")).Output()
    if err != nil {
      panic(err)
    }
  }
}

func (mounter Mounter) HandleVolumeEvent(msg events.Message) {
  if msg.Action != "mount" && msg.Action != "unmount" {
    return
  }

  volume_name := msg.Actor.ID
  plugin_name := msg.Actor.Attributes["driver"]

  d := time.Now().Add(API_TIMEOUT)
  ctx, cancel := context.WithDeadline(context.Background(), d)
  defer cancel()

  if plugin_name != "local" {
    if msg.Action == "mount" {
      volume := mounter.getVolumeByName(ctx, volume_name)
      mounter.MountVolume(ctx, volume)
    } else {
      mounter.UnmountVolume(volume_name)
    }
  }
}

func (mounter Mounter) MountVolume(ctx context.Context, volume *types.Volume) {
  d := time.Now().Add(API_TIMEOUT)
  ctx, cancel := context.WithDeadline(context.Background(), d)
  defer cancel()

  target_path := targetPath(volume.Name)
  fmt.Printf("Bind-Mounting volume %s from %s to %s\n", volume.Name, volume.Mountpoint, target_path)

  err := os.MkdirAll(target_path, 600)
  if err != nil {
    panic(err)
  }

  cmd := exec.Command("mount", "--bind", volume.Mountpoint, target_path)
  stdoutStderr, err := cmd.CombinedOutput()
  if err != nil {
    fmt.Printf("%s\n", stdoutStderr)
    panic(err)
  }

  cmd = exec.Command("mount", "-obind,remount,ro", target_path)
  stdoutStderr, err = cmd.CombinedOutput()
  if err != nil {
    fmt.Printf("%s\n", stdoutStderr)
    panic(err)
  }
}

func (mounter Mounter) UnmountVolume(volumeName string) {
  target_path := targetPath(volumeName)
  fmt.Printf("Unmounting volume %s at %s\n", volumeName, target_path)
  cmd := exec.Command("umount", target_path)
  stdoutStderr, err := cmd.CombinedOutput()
  if err != nil {
    fmt.Printf("%s\n", stdoutStderr)
    panic(err)
  }

  err = os.Remove(target_path)
  if err != nil {
    panic(err)
  }
}

func (mounter Mounter) getVolumeByName(ctx context.Context, volumeName string) *types.Volume {
  volumes, err := mounter.cli.VolumeList(ctx, filters.Args {})
  if err != nil {
    panic(err)
  }

  for _, volume := range volumes.Volumes {
    if volume.Name == volumeName {
      return volume
    }
  }

  panic("Could not find volume " + volumeName)
}

func (mounter Mounter) getVolumePath(ctx context.Context, volume *types.Volume) string {
  plugin, _, err := mounter.cli.PluginInspectWithRaw(ctx, volume.Driver)
  if err != nil {
    panic(err)
  }

  volume_base_path := fmt.Sprintf(VOLUME_ROOT_PATH_TEMPLATE, plugin.ID)
  volume_relative_path, err := filepath.Rel(volume_base_path, volume.Mountpoint)
  if err != nil {
    panic(err)
  }

  // use only first path component as mount dir
  parts := strings.Split(volume_relative_path, "/")
  volume_relative_path = parts[0]

  return filepath.Join(volume_base_path, volume_relative_path)
}

func main() {
  cli, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion(DOCKER_CLIENT_VERSION))
  if err != nil {
    panic(err)
  }

  msgs, errs := cli.Events(context.Background(), types.EventsOptions {
    Filters: filters.NewArgs(filters.Arg("type", "volume")),
  })

  mounter := newMounter(cli)
  mounter.MountCurrentVolumes()

  c := make(chan os.Signal, 2)
  signal.Notify(c, os.Interrupt, syscall.SIGTERM)

  for {
    select {
      case err := <-errs: fmt.Printf("Error: %s\n", err)
      case msg := <-msgs: mounter.HandleVolumeEvent(msg)
      case <-c:
        mounter.UnmountCurrentVolumes()
        os.Exit(0)
    }
  }
}
