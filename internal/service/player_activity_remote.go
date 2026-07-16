package service

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"path"
	"strconv"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

//go:embed remote_player_activity_daemon.sh
var remotePlayerActivityDaemonScript []byte

const (
	remotePlayerActivitySpoolBytes    = 8 * 1024 * 1024
	remotePlayerActivityMaximumOutput = remotePlayerActivitySpoolBytes + 64*1024
)

// playerActivityRemotePaths returns stable remote files under the Server monitoring directory.
func playerActivityRemotePaths(server model.MinecraftServer) (dataDirectory, daemonPath, spoolPath, claimPath string) {
	dataDirectory = path.Join(server.RemotePath, ".mineops-monitoring")
	daemonPath = path.Join(dataDirectory, "player-activity-daemon.sh")
	spoolPath = path.Join(dataDirectory, "player-activity.spool")
	claimPath = path.Join(dataDirectory, "player-activity.claim")
	return
}

// playerActivityCollectorConfigDigest binds every daemon input so stale collectors are replaced safely.
func playerActivityCollectorConfigDigest(server model.MinecraftServer, processIdentityID, managedPID, managedPGID, logPath string, intervalSeconds int) string {
	hash := sha256.New()
	for _, value := range [][]byte{
		remotePlayerActivityDaemonScript, []byte(server.ID.String()), []byte(server.RemotePath), []byte(processIdentityID),
		[]byte(managedPID), []byte(managedPGID), []byte(logPath), []byte(strconv.Itoa(intervalSeconds)),
	} {
		_, _ = hash.Write(value)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// playerActivityCollectorClaimScript atomically promotes a complete Spool to a retryable Claim.
func playerActivityCollectorClaimScript() string {
	return `set -eu
claim_path=$1
spool_path=$2
attempt=0
while [ ! -s "$claim_path" ] && [ ! -s "$spool_path" ] && [ "$attempt" -lt 4 ]; do
  sleep 1
  attempt=$((attempt + 1))
done
if [ ! -s "$claim_path" ] && [ -s "$spool_path" ]; then mv -f "$spool_path" "$claim_path"; fi
if [ -s "$claim_path" ]; then cat "$claim_path"; fi`
}

// playerActivityCollectorStopScript kills only a PID whose command line still names the managed daemon.
func playerActivityCollectorStopScript() string {
	return `set -eu
pid_path=$1
daemon_path=$2
config_path=$3
pid=""
if [ -f "$pid_path" ]; then pid=$(cat "$pid_path" 2>/dev/null || true); fi
if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
  command_line=$(tr '\000' ' ' < "/proc/$pid/cmdline" 2>/dev/null || true)
  case "$command_line" in
    *"$daemon_path"*)
      kill "$pid" 2>/dev/null || true
      attempt=0
      while kill -0 "$pid" 2>/dev/null && [ "$attempt" -lt 5 ]; do
        sleep 1
        attempt=$((attempt + 1))
      done ;;
  esac
fi
rm -f "$pid_path" "$config_path"`
}
