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

package cluster

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	dpv1alpha1 "github.com/apecloud/kubeblocks/apis/dataprotection/v1alpha1"
	dptypes "github.com/apecloud/kubeblocks/pkg/dataprotection/types"
	"github.com/apecloud/kubeblocks/pkg/dataprotection/utils/boolptr"

	"github.com/apecloud/kbcli/pkg/cluster"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
)

var (
	describeExample = templates.Examples(`
		# describe a specified cluster
		kbcli cluster describe mycluster`)

	newTbl = func(out io.Writer, title string, header ...interface{}) *printer.TablePrinter {
		if title != "" {
			fmt.Fprintln(out, title)
		}
		tbl := printer.NewTablePrinter(out)
		tbl.SetHeader(header...)
		return tbl
	}
)

type describeOptions struct {
	factory   cmdutil.Factory
	client    clientset.Interface
	dynamic   dynamic.Interface
	namespace string

	// resource type and names
	gvr   schema.GroupVersionResource
	names []string

	*cluster.ClusterObjects
	genericiooptions.IOStreams
}

func newOptions(f cmdutil.Factory, streams genericiooptions.IOStreams) *describeOptions {
	return &describeOptions{
		factory:   f,
		IOStreams: streams,
		gvr:       types.ClusterGVR(),
	}
}

func NewDescribeCmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := newOptions(f, streams)
	cmd := &cobra.Command{
		Use:               "describe NAME",
		Short:             "Show details of a specific cluster.",
		Example:           describeExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.complete(args))
			util.CheckErr(o.run())
		},
	}
	return cmd
}

func (o *describeOptions) complete(args []string) error {
	var err error

	if len(args) == 0 {
		return fmt.Errorf("cluster name should be specified")
	}
	o.names = args

	if o.client, err = o.factory.KubernetesClientSet(); err != nil {
		return err
	}

	if o.dynamic, err = o.factory.DynamicClient(); err != nil {
		return err
	}

	if o.namespace, _, err = o.factory.ToRawKubeConfigLoader().Namespace(); err != nil {
		return err
	}
	return nil
}

func (o *describeOptions) run() error {
	for _, name := range o.names {
		if err := o.describeCluster(name); err != nil {
			return err
		}
	}
	return nil
}

func (o *describeOptions) describeCluster(name string) error {
	clusterGetter := cluster.ObjectsGetter{
		Client:    o.client,
		Dynamic:   o.dynamic,
		Name:      name,
		Namespace: o.namespace,
		GetOptions: cluster.GetOptions{
			WithClusterDef:     cluster.Maybe,
			WithCompDef:        cluster.Maybe,
			WithService:        cluster.Need,
			WithPod:            cluster.Need,
			WithPVC:            cluster.Need,
			WithDataProtection: cluster.Need,
		},
	}

	var err error
	if o.ClusterObjects, err = clusterGetter.Get(); err != nil {
		return err
	}

	// cluster summary
	showCluster(o.Cluster, o.Out)

	// show endpoints
	if err = showEndpoints(o.dynamic, o.Nodes, o.Cluster, o.Services, o.Out); err != nil {
		return err
	}

	// topology
	showTopology(o.ClusterObjects.GetInstanceInfo(), o.Out)

	comps := o.ClusterObjects.GetComponentInfo()
	// resources
	showResource(comps, o.Out)

	// images
	showImages(comps, o.Out)

	// data protection info
	defaultBackupRepo, err := o.getDefaultBackupRepo()
	if err != nil {
		return err
	}
	recoverableTime, continuousMethod, err := o.getBackupRecoverableTime()
	if err != nil {
		return err
	}
	showDataProtection(o.BackupPolicies, o.BackupSchedules, defaultBackupRepo, continuousMethod, recoverableTime, o.Out)

	// events
	showEvents(o.Cluster.Name, o.Cluster.Namespace, o.Out)
	fmt.Fprintln(o.Out)

	return nil
}

func (o *describeOptions) getDefaultBackupRepo() (string, error) {
	backupRepoListObj, err := o.dynamic.Resource(types.BackupRepoGVR()).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return printer.NoneString, err
	}
	for _, item := range backupRepoListObj.Items {
		repo := dpv1alpha1.BackupRepo{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &repo); err != nil {
			return printer.NoneString, err
		}
		if repo.Status.IsDefault {
			return repo.Name, nil
		}
	}
	return printer.NoneString, nil
}

func showCluster(c *kbappsv1.Cluster, out io.Writer) {
	if c == nil {
		return
	}
	title := fmt.Sprintf("Name: %s\t Created Time: %s", c.Name, util.TimeFormat(&c.CreationTimestamp))
	tbl := newTbl(out, title, "NAMESPACE", "CLUSTER-DEFINITION", "TOPOLOGY", "STATUS", "TERMINATION-POLICY")
	tbl.AddRow(c.Namespace, c.Spec.ClusterDef, c.Spec.Topology, string(c.Status.Phase), string(c.Spec.TerminationPolicy))
	tbl.Print()
}

func showTopology(instances []*cluster.InstanceInfo, out io.Writer) {
	tbl := newTbl(out, "\nTopology:", "COMPONENT", "SERVICE-VERSION", "INSTANCE", "ROLE", "STATUS", "AZ", "NODE", "CREATED-TIME")
	for _, ins := range instances {
		tbl.AddRow(ins.Component, ins.ServiceVersion, ins.Name, ins.Role, ins.Status, ins.AZ, ins.Node, ins.CreatedTime)
	}
	tbl.Print()
}

func showResource(comps []*cluster.ComponentInfo, out io.Writer) {
	tbl := newTbl(out, "\nResources Allocation:", "COMPONENT", "INSTANCE-TEMPLATE", "CPU(REQUEST/LIMIT)", "MEMORY(REQUEST/LIMIT)", "STORAGE-SIZE", "STORAGE-CLASS")
	for _, c := range comps {
		tbl.AddRow(c.Name, c.InstanceTemplateName,
			c.CPU, c.Memory, cluster.BuildStorageSize(c.Storage), cluster.BuildStorageClass(c.Storage))
	}
	tbl.Print()
}

func showImages(comps []*cluster.ComponentInfo, out io.Writer) {
	tbl := newTbl(out, "\nImages:", "COMPONENT", "COMPONENT-DEFINITION", "IMAGE")
	for _, c := range comps {
		tbl.AddRow(c.Name, c.ComponentDef, c.Image)
	}
	tbl.Print()
}

func showEvents(name string, namespace string, out io.Writer) {
	// hint user how to get events
	fmt.Fprintf(out, "\nShow cluster events: kbcli cluster list-events -n %s %s", namespace, name)
}

func showEndpoints(dynamic dynamic.Interface, nodes []*corev1.Node, c *kbappsv1.Cluster, svcList *corev1.ServiceList, out io.Writer) error {
	if c == nil {
		return nil
	}

	tbl := newTbl(out, "\nEndpoints:", "COMPONENT", "INTERNAL", "EXTERNAL")
	componentPairs, err := cluster.GetClusterComponentPairs(dynamic, c)
	if err != nil {
		return err
	}
	for _, componentPair := range componentPairs {
		internalEndpoints, externalEndpoints := cluster.GetComponentEndpoints(nodes, svcList, componentPair.ComponentName)
		if len(internalEndpoints) == 0 && len(externalEndpoints) == 0 {
			continue
		}
		tbl.AddRow(cluster.BuildShardingComponentName(componentPair.ShardingName, componentPair.ComponentName),
			util.CheckEmpty(strings.Join(internalEndpoints, "\n")), util.CheckEmpty(strings.Join(externalEndpoints, "\n")))
	}
	tbl.Print()
	return nil
}

func showDataProtection(backupPolicies []dpv1alpha1.BackupPolicy, backupSchedules []dpv1alpha1.BackupSchedule, defaultBackupRepo, continuousMethod, recoverableTimeRange string, out io.Writer) {
	if len(backupPolicies) == 0 || len(backupSchedules) == 0 {
		return
	}
	tbl := newTbl(out, "\nData Protection:", "BACKUP-REPO", "AUTO-BACKUP", "BACKUP-SCHEDULE", "BACKUP-METHOD", "BACKUP-RETENTION", "RECOVERABLE-TIME")
	getEnableString := func(enable bool) string {
		if enable {
			return "Enabled"
		}
		return "Disabled"
	}
	for _, schedule := range backupSchedules {
		backupRepo := defaultBackupRepo
		for _, policy := range backupPolicies {
			if policy.Name != schedule.Spec.BackupPolicyName {
				continue
			}
			if policy.Spec.BackupRepoName != nil {
				backupRepo = *policy.Spec.BackupRepoName
			}
		}
		for _, schedulePolicy := range schedule.Spec.Schedules {
			if recoverableTimeRange != "" && continuousMethod == schedulePolicy.BackupMethod {
				// continuous backup with recoverable time
				tbl.AddRow(backupRepo, getEnableString(boolptr.IsSetToTrue(schedulePolicy.Enabled)), schedulePolicy.CronExpression, schedulePolicy.BackupMethod, schedulePolicy.RetentionPeriod.String(), recoverableTimeRange)
			} else if boolptr.IsSetToTrue(schedulePolicy.Enabled) {
				tbl.AddRow(backupRepo, "Enabled", schedulePolicy.CronExpression, schedulePolicy.BackupMethod, schedulePolicy.RetentionPeriod.String(), "")
			}
		}
	}
	tbl.Print()
}

// getBackupRecoverableTime returns the recoverable time range string
func (o *describeOptions) getBackupRecoverableTime() (string, string, error) {
	continuousBackups, err := o.getBackupList(dpv1alpha1.BackupTypeContinuous)
	if err != nil {
		return "", "", err
	}
	if len(continuousBackups) == 0 {
		return "", "", nil
	}
	continuousBackup := continuousBackups[0]
	if continuousBackup.GetStartTime() == nil || continuousBackup.GetEndTime() == nil {
		return "", "", nil
	}
	fullBackups, err := o.getBackupList(dpv1alpha1.BackupTypeFull)
	if err != nil {
		return "", "", err
	}
	isTimeInRange := func(t metav1.Time, start *metav1.Time, end *metav1.Time) bool {
		return !t.Before(start) && !t.After(end.Time)
	}
	sortBackup(fullBackups, false)
	for _, backup := range fullBackups {
		completeTime := backup.GetEndTime()
		if completeTime != nil && isTimeInRange(*completeTime, continuousBackup.GetStartTime(), continuousBackup.GetEndTime()) {
			return fmt.Sprintf("%s ~ %s", util.TimeFormatWithDuration(completeTime, time.Second),
				util.TimeFormatWithDuration(continuousBackup.GetEndTime(), time.Second)), continuousBackup.Spec.BackupMethod, nil
		}
	}
	return "", "", nil
}

func (o *describeOptions) getBackupList(backupType dpv1alpha1.BackupType) ([]*dpv1alpha1.Backup, error) {
	backupList, err := o.dynamic.Resource(types.BackupGVR()).Namespace(o.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s,%s=%s",
			constant.AppInstanceLabelKey, o.Cluster.Name,
			dptypes.BackupTypeLabelKey, backupType),
	})
	if err != nil {
		return nil, err
	}
	var backups []*dpv1alpha1.Backup
	for i := range backupList.Items {
		fullBackup := &dpv1alpha1.Backup{}
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(backupList.Items[i].Object, fullBackup); err != nil {
			return nil, err
		}
		backups = append(backups, fullBackup)
	}
	return backups, nil
}

func sortBackup(backups []*dpv1alpha1.Backup, reverse bool) []*dpv1alpha1.Backup {
	sort.Slice(backups, func(i, j int) bool {
		if reverse {
			i, j = j, i
		}
		if backups[i].GetEndTime() == nil {
			return false
		}
		if backups[j].GetEndTime() == nil {
			return true
		}
		return backups[i].GetEndTime().Before(backups[j].GetEndTime())
	})
	return backups
}
