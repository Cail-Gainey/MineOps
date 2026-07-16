package service

import (
	"fmt"
	"path"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

type minecraftServerRuntimeProfile struct {
	configurationFile string
	requiresEULA      bool
	readyPatterns     []string
	stopCommand       string
}

type minecraftServerLaunchCommand struct {
	executable string
	arguments  []string
}

func runtimeProfileForServerType(serverType enums.MinecraftServerType) minecraftServerRuntimeProfile {
	switch serverType {
	case enums.ServerVelocity:
		return minecraftServerRuntimeProfile{
			configurationFile: "velocity.toml",
			readyPatterns:     []string{"Done (", "Listening on"},
			stopCommand:       "shutdown",
		}
	case enums.ServerWaterfall, enums.ServerBungee:
		return minecraftServerRuntimeProfile{
			configurationFile: "config.yml",
			readyPatterns:     []string{"Listening on", "For help, type"},
			stopCommand:       "end",
		}
	default:
		return minecraftServerRuntimeProfile{
			configurationFile: "server.properties",
			requiresEULA:      true,
			readyPatterns:     []string{"Done (", "Listening on", "For help, type"},
			stopCommand:       "stop",
		}
	}
}

func defaultServerArguments(serverType enums.MinecraftServerType) []string {
	switch serverType {
	case enums.ServerVelocity, enums.ServerWaterfall, enums.ServerBungee:
		return nil
	default:
		return []string{"nogui"}
	}
}

func serverArgumentsForRuntime(serverType enums.MinecraftServerType, arguments []string) []string {
	if len(arguments) == 1 && arguments[0] == "nogui" && len(defaultServerArguments(serverType)) == 0 {
		return nil
	}
	return arguments
}

func launchCommandForServer(server model.MinecraftServer, javaExecutable string) minecraftServerLaunchCommand {
	serverArguments := serverArgumentsForRuntime(server.Type, server.LaunchProfile.ServerArguments)
	if server.Type == enums.ServerForge || server.Type == enums.ServerNeoForge {
		javaHome := path.Dir(path.Dir(javaExecutable))
		arguments := []string{
			"-c",
			`java_home=$1; shift; export JAVA_HOME="$java_home"; export PATH="$JAVA_HOME/bin:$PATH"; exec sh run.sh "$@"`,
			"mineops-forge",
			javaHome,
		}
		return minecraftServerLaunchCommand{executable: "sh", arguments: append(arguments, serverArguments...)}
	}

	arguments := []string{fmt.Sprintf("-Xms%dM", server.LaunchProfile.XmsMiB), fmt.Sprintf("-Xmx%dM", server.LaunchProfile.XmxMiB)}
	arguments = append(arguments, server.LaunchProfile.JVMArguments...)
	arguments = append(arguments, "-jar", server.LaunchProfile.JarPath)
	arguments = append(arguments, serverArguments...)
	return minecraftServerLaunchCommand{executable: javaExecutable, arguments: arguments}
}
