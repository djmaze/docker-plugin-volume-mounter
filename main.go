package main

import (
  "context"
  "fmt"
  "time"
  "path/filepath"
  "os"
  "os/exec"
  "os/signal"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
  "github.com/docker/docker/client"
)

const DOCKER_CLIENT_VERSION = "1.39"
const PLUGIN_MOUNT_PATH_TEMPLATE = "/var/lib/docker/plugins/%s/propagated-mount/%s"
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
      _, err := exec.Command("sh", "-c", "mount | grep " + volume.Mountpoint).Output()
      if err == nil {
        mounter.MountVolume(volume.Name, volume.Driver)
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

  if plugin_name != "local" {
    if msg.Action == "mount" {
      mounter.MountVolume(volume_name, plugin_name)
    } else {
      mounter.UnmountVolume(volume_name)
    }
  }
}

func (mounter Mounter) MountVolume(volumeName string, pluginName string) {
  d := time.Now().Add(5000 * time.Millisecond)
  ctx, cancel := context.WithDeadline(context.Background(), d)
  defer cancel()

  // Get plugin id
  plugin, _, err := mounter.cli.PluginInspectWithRaw(ctx, pluginName)
  if err != nil {
    panic(err)
  }

  // Get mountpoint
  volume, err := mounter.cli.VolumeInspect(ctx, volumeName)
  if err != nil {
    panic(err)
  }

  volume_id := filepath.Base(volume.Mountpoint)
  volume_path := fmt.Sprintf(PLUGIN_MOUNT_PATH_TEMPLATE, plugin.ID, volume_id)
  target_path := targetPath(volumeName)

  fmt.Printf("Bind-Mounting volume %s from %s to %s\n", volumeName, volume_path, target_path)

  err = os.MkdirAll(target_path, 600)
  if err != nil {
    panic(err)
  }

  cmd := exec.Command("mount", "-obind", volume_path, target_path)
  stdoutStderr, err := cmd.CombinedOutput()
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

  c := make(chan os.Signal, 1)
  signal.Notify(c, os.Interrupt)

  for {
    select {
      case err := <-errs: fmt.Printf("%s\n", err)
      case msg := <-msgs: mounter.HandleVolumeEvent(msg)
      case <-c:
        mounter.UnmountCurrentVolumes()
        os.Exit(0)
    }
  }
}
