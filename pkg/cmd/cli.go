/*
Copyright (C) 2022-2025 ApeCloud Co., Ltd

This file is part of KubeBlocks project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"
	kccmd "k8s.io/kubectl/pkg/cmd"
	kcplugin "k8s.io/kubectl/pkg/cmd/plugin"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	utilcomp "k8s.io/kubectl/pkg/util/completion"
	"k8s.io/kubectl/pkg/util/templates"
	ctrl "sigs.k8s.io/controller-runtime"

	viper "github.com/apecloud/kubeblocks/pkg/viperx"

	"github.com/apecloud/kbcli/pkg/cmd/addon"
	"github.com/apecloud/kbcli/pkg/cmd/backuprepo"
	"github.com/apecloud/kbcli/pkg/cmd/cluster"
	"github.com/apecloud/kbcli/pkg/cmd/clusterdefinition"
	"github.com/apecloud/kbcli/pkg/cmd/componentdefinition"
	"github.com/apecloud/kbcli/pkg/cmd/componentversion"
	"github.com/apecloud/kbcli/pkg/cmd/dataprotection"
	"github.com/apecloud/kbcli/pkg/cmd/kubeblocks"
	"github.com/apecloud/kbcli/pkg/cmd/opsdefinition"
	"github.com/apecloud/kbcli/pkg/cmd/options"
	"github.com/apecloud/kbcli/pkg/cmd/playground"
	"github.com/apecloud/kbcli/pkg/cmd/plugin"
	"github.com/apecloud/kbcli/pkg/cmd/report"
	"github.com/apecloud/kbcli/pkg/cmd/trace"
	"github.com/apecloud/kbcli/pkg/cmd/version"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

const (
	cliName = "kbcli"
)

func init() {
	if _, err := util.GetCliHomeDir(); err != nil {
		fmt.Println("Failed to create kbcli home dir:", err)
	}

	// replace the kubectl plugin filename prefixes with ours
	kcplugin.ValidPluginFilenamePrefixes = plugin.ValidPluginFilenamePrefixes

	// put the download directory of the plugin into the PATH
	if err := util.AddDirToPath(fmt.Sprintf("%s/.%s/plugins/bin", os.Getenv("HOME"), cliName)); err != nil {
		fmt.Println("Failed to add kbcli bin dir to PATH:", err)
	}

	// when the kubernetes cluster is not ready, the runtime will output the error
	// message like "couldn't get resource list for", we ignore it
	utilruntime.ErrorHandlers[0] = func(err error) {
		if klog.V(2).Enabled() {
			klog.ErrorDepth(2, err)
		}
	}
}

func NewDefaultCliCmd() *cobra.Command {
	cmd := NewCliCmd()

	pluginHandler := kccmd.NewDefaultPluginHandler(plugin.ValidPluginFilenamePrefixes)

	if len(os.Args) > 1 {
		cmdPathPieces := os.Args[1:]

		// only look for suitable extension executables if
		// the specified command does not exist
		if _, _, err := cmd.Find(cmdPathPieces); err != nil {
			var cmdName string
			for _, arg := range cmdPathPieces {
				if !strings.HasPrefix(arg, "-") {
					cmdName = arg
					break
				}
			}

			switch cmdName {
			case "help", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
				// Don't search for a plugin
			default:
				if err := kccmd.HandlePluginCommand(pluginHandler, cmdPathPieces, true); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}
		}
	}

	return cmd
}

func NewCliCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   cliName,
		Short: "KubeBlocks CLI.",
		Long: `
=============================================
 __    __ _______   ______  __       ______ 
|  \  /  \       \ /      \|  \     |      \
| ▓▓ /  ▓▓ ▓▓▓▓▓▓▓\  ▓▓▓▓▓▓\ ▓▓      \▓▓▓▓▓▓
| ▓▓/  ▓▓| ▓▓__/ ▓▓ ▓▓   \▓▓ ▓▓       | ▓▓  
| ▓▓  ▓▓ | ▓▓    ▓▓ ▓▓     | ▓▓       | ▓▓  
| ▓▓▓▓▓\ | ▓▓▓▓▓▓▓\ ▓▓   __| ▓▓       | ▓▓  
| ▓▓ \▓▓\| ▓▓__/ ▓▓ ▓▓__/  \ ▓▓_____ _| ▓▓_ 
| ▓▓  \▓▓\ ▓▓    ▓▓\▓▓    ▓▓ ▓▓     \   ▓▓ \
 \▓▓   \▓▓\▓▓▓▓▓▓▓  \▓▓▓▓▓▓ \▓▓▓▓▓▓▓▓\▓▓▓▓▓▓

=============================================
A Command Line Interface for KubeBlocks`,

		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == cobra.ShellCompRequestCmd {
				kcplugin.SetupPluginCompletion(cmd, args)
			}
			return nil
		},
	}

	// Start from this point we get warnings on flags that contain "_" separators
	// when adding them with hyphen instead of the original name.
	cmd.SetGlobalNormalizationFunc(cliflag.WarnWordSepNormalizeFunc)

	flags := cmd.PersistentFlags()

	// add kubernetes flags like kubectl
	kubeConfigFlags := util.NewConfigFlagNoWarnings()
	kubeConfigFlags.AddFlags(flags)
	matchVersionKubeConfigFlags := cmdutil.NewMatchVersionFlags(kubeConfigFlags)
	matchVersionKubeConfigFlags.AddFlags(flags)

	// add klog flags
	util.AddKlogFlags(flags)

	f := cmdutil.NewFactory(matchVersionKubeConfigFlags)
	ioStreams := genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	// Add subcommands
	cmd.AddCommand(
		playground.NewPlaygroundCmd(ioStreams),
		kubeblocks.NewKubeBlocksCmd(f, ioStreams),
		options.NewCmdOptions(ioStreams.Out),
		version.NewVersionCmd(f),
		clusterdefinition.NewClusterDefinitionCmd(f, ioStreams),
		componentdefinition.NewComponentDefinitionCmd(f, ioStreams),
		componentversion.NewComponentVersionCmd(f, ioStreams),
		opsdefinition.NewOpsDefinitionCmd(f, ioStreams),
		addon.NewAddonCmd(f, ioStreams),
		plugin.NewPluginCmd(ioStreams),
		report.NewReportCmd(f, ioStreams),
		backuprepo.NewBackupRepoCmd(f, ioStreams),
		dataprotection.NewDataProtectionCmd(f, ioStreams),
		trace.NewTraceCmd(f, ioStreams),
	)

	filters := []string{"options"}
	templates.ActsAsRootCommand(cmd, filters, []templates.CommandGroup{}...)

	helpFunc := cmd.HelpFunc()
	usageFunc := cmd.UsageFunc()

	// clusterCmd sets its own usage and help function and its subcommand will inherit it,
	// so we need to set its subcommand's usage and help function back to the root command
	clusterCmd := cluster.NewClusterCmd(f, ioStreams)
	registerUsageAndHelpFuncForSubCommand(clusterCmd, helpFunc, usageFunc)
	cmd.AddCommand(clusterCmd)

	utilcomp.SetFactoryForCompletion(f)
	registerCompletionFuncForGlobalFlags(cmd, f)

	cobra.OnInitialize(initConfig, initLog)
	return cmd
}

func initLog() {
	ctrl.SetLogger(klog.NewKlogr())
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(fmt.Sprintf("/etc/%s/", cliName))
	viper.AddConfigPath(fmt.Sprintf("$HOME/.%s/", cliName))
	viper.AutomaticEnv() // read in environment variables that match
	viper.SetEnvPrefix(cliName)

	viper.SetDefault(types.CfgKeyClusterDefaultStorageSize, "20Gi")
	viper.SetDefault(types.CfgKeyClusterDefaultReplicas, 1)
	viper.SetDefault(types.CfgKeyClusterDefaultCPU, "1000m")
	viper.SetDefault(types.CfgKeyClusterDefaultMemory, "1Gi")

	viper.SetDefault(types.CfgKeyHelmRepoURL, "")
	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		klog.V(2).Infof("Using config file: %s", viper.ConfigFileUsed())
	}
}

func registerCompletionFuncForGlobalFlags(cmd *cobra.Command, f cmdutil.Factory) {
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"namespace",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.CompGetResource(f, "namespace", toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"context",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.ListContextsInConfig(toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"cluster",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.ListClustersInConfig(toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"user",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return utilcomp.ListUsersInConfig(toComplete), cobra.ShellCompDirectiveNoFileComp
		}))
}

func registerUsageAndHelpFuncForSubCommand(cmd *cobra.Command, helpFunc func(*cobra.Command, []string), usageFunc func(command *cobra.Command) error) {
	for _, subCmd := range cmd.Commands() {
		subCmd.SetHelpFunc(helpFunc)
		subCmd.SetUsageFunc(usageFunc)
	}
}
