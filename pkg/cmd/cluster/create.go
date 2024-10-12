/*
Copyright (C) 2022-2024 ApeCloud Co., Ltd

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

package cluster

import (
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var clusterCreateExample = templates.Examples(`
	# Create a postgresql 
	kbcli cluster create postgresql my-cluster

   # Get the cluster yaml by dry-run
	kbcli cluster create postgresql my-cluster --dry-run

	# Edit cluster yaml before creation.
	kbcli cluster create mycluster --edit
`)

// UpdatableFlags is the flags that cat be updated by update command
type UpdatableFlags struct {
	// Options for cluster termination policy
	TerminationPolicy string `json:"terminationPolicy"`

	// Add-on switches for cluster observability
	DisableExporter bool `json:"monitor"`
	EnableAllLogs   bool `json:"enableAllLogs"`

	// Configuration and options for cluster affinity and tolerations
	PodAntiAffinity string `json:"podAntiAffinity"`
	// TopologyKeys if TopologyKeys is nil, add omitempty json tag, because CueLang can not covert null to list.
	TopologyKeys   []string          `json:"topologyKeys,omitempty"`
	NodeLabels     map[string]string `json:"nodeLabels,omitempty"`
	Tenancy        string            `json:"tenancy"`
	TolerationsRaw []string          `json:"-"`

	// backup config
	BackupEnabled                 bool   `json:"-"`
	BackupRetentionPeriod         string `json:"-"`
	BackupMethod                  string `json:"-"`
	BackupCronExpression          string `json:"-"`
	BackupStartingDeadlineMinutes int64  `json:"-"`
	BackupRepoName                string `json:"-"`
	BackupPITREnabled             bool   `json:"-"`
}

type CreateOptions struct {
	Cmd *cobra.Command `json:"-"`

	action.CreateOptions `json:"-"`
}

func NewCreateCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := NewCreateOptions(f, streams)
	cmd := &cobra.Command{
		Use:     "create [NAME]",
		Short:   "Create a cluster.",
		Example: clusterCreateExample,
		Run: func(cmd *cobra.Command, args []string) {
			println("no implement")
		},
	}

	// add all subcommands for supported cluster type
	cmd.AddCommand(buildCreateSubCmds(&o.CreateOptions)...)

	o.Cmd = cmd

	return cmd
}

func NewCreateOptions(f cmdutil.Factory, streams genericiooptions.IOStreams) *CreateOptions {
	o := &CreateOptions{CreateOptions: action.CreateOptions{
		Factory:   f,
		IOStreams: streams,
		GVR:       types.ClusterGVR(),
	}}
	return o
}

func (f *UpdatableFlags) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.PodAntiAffinity, "pod-anti-affinity", "Preferred", "Pod anti-affinity type, one of: (Preferred, Required)")
	cmd.Flags().BoolVar(&f.DisableExporter, "disable-exporter", true, "Enable or disable monitoring")
	cmd.Flags().BoolVar(&f.EnableAllLogs, "enable-all-logs", false, "Enable advanced application all log extraction, set to true will ignore enabledLogs of component level, default is false")
	cmd.Flags().StringVar(&f.TerminationPolicy, "termination-policy", "Delete", "Termination policy, one of: (DoNotTerminate, Halt, Delete, WipeOut)")
	cmd.Flags().StringArrayVar(&f.TopologyKeys, "topology-keys", nil, "Topology keys for affinity")
	cmd.Flags().StringToStringVar(&f.NodeLabels, "node-labels", nil, "Node label selector")
	cmd.Flags().StringSliceVar(&f.TolerationsRaw, "tolerations", nil, `Tolerations for cluster, such as "key=value:effect, key:effect", for example '"engineType=mongo:NoSchedule", "diskType:NoSchedule"'`)
	cmd.Flags().StringVar(&f.Tenancy, "tenancy", "SharedNode", "Tenancy options, one of: (SharedNode, DedicatedNode)")
	cmd.Flags().BoolVar(&f.BackupEnabled, "backup-enabled", false, "Specify whether enabled automated backup")
	cmd.Flags().StringVar(&f.BackupRetentionPeriod, "backup-retention-period", "1d", "a time string ending with the 'd'|'D'|'h'|'H' character to describe how long the Backup should be retained")
	cmd.Flags().StringVar(&f.BackupMethod, "backup-method", "", "the backup method, view it by \"kbcli cd describe <cluster-definition>\", if not specified, the default backup method will be to take snapshots of the volume")
	cmd.Flags().StringVar(&f.BackupCronExpression, "backup-cron-expression", "", "the cron expression for schedule, the timezone is in UTC. see https://en.wikipedia.org/wiki/Cron.")
	cmd.Flags().Int64Var(&f.BackupStartingDeadlineMinutes, "backup-starting-deadline-minutes", 0, "the deadline in minutes for starting the backup job if it misses its scheduled time for any reason")
	cmd.Flags().StringVar(&f.BackupRepoName, "backup-repo-name", "", "the backup repository name")
	cmd.Flags().BoolVar(&f.BackupPITREnabled, "pitr-enabled", false, "Specify whether enabled point in time recovery")

	util.CheckErr(cmd.RegisterFlagCompletionFunc(
		"termination-policy",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{
				"DoNotTerminate\tblock delete operation",
				"Halt\tdelete workload resources such as statefulset, deployment workloads but keep PVCs",
				"Delete\tbased on Halt and deletes PVCs",
				"WipeOut\tbased on Delete and wipe out all volume snapshots and snapshot data from backup storage location",
			}, cobra.ShellCompDirectiveNoFileComp
		}))
	util.CheckErr(cmd.RegisterFlagCompletionFunc(
		"pod-anti-affinity",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{
				"Preferred\ttry to spread pods of the cluster by the specified topology-keys",
				"Required\tmust spread pods of the cluster by the specified topology-keys",
			}, cobra.ShellCompDirectiveNoFileComp
		}))
	util.CheckErr(cmd.RegisterFlagCompletionFunc(
		"tenancy",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{
				"SharedNode\tpods of the cluster may share the same node",
				"DedicatedNode\teach pod of the cluster will run on their own dedicated node",
			}, cobra.ShellCompDirectiveNoFileComp
		}))
}

// MultipleSourceComponents gets component data from multiple source, such as stdin, URI and local file
func MultipleSourceComponents(fileName string, in io.Reader) ([]byte, error) {
	var data io.Reader
	switch {
	case fileName == "-":
		data = in
	case strings.Index(fileName, "http://") == 0 || strings.Index(fileName, "https://") == 0:
		resp, err := http.Get(fileName)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		data = resp.Body
	default:
		f, err := os.Open(fileName)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data = f
	}
	return io.ReadAll(data)
}
