/*
copyright (c) 2022-2024 apecloud co., ltd

this file is part of kubeblocks project

this program is free software: you can redistribute it and/or modify
it under the terms of the gnu affero general public license as published by
the free software foundation, either version 3 of the license, or
(at your option) any later version.

this program is distributed in the hope that it will be useful
but without any warranty; without even the implied warranty of
merchantability or fitness for a particular purpose.  see the
gnu affero general public license for more details.

you should have received a copy of the gnu affero general public license
along with this program.  if not, see <http://www.gnu.org/licenses/>.
*/

package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	kbappsv1 "github.com/apecloud/kubeblocks/apis/apps/v1"
	kbappsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	"github.com/apecloud/kubeblocks/pkg/constant"
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/prompt"
)

const (
	kbIncrementConverterAK = "kb-increment-converter"

	escape = "\x1b"
)

type componentDefRefConvert struct {
	cmpdPrefix     string
	serviceVersion string
}

var (
	clusterVersionConvert = map[string]map[string]componentDefRefConvert{}
	// 0.9 cmpd name => 1.0 cmpd prefix
	componentDefWithChartVersion = []string{
		"clickhouse-24",
		"elasticsearch-8",
		"kafka-combine", "kafka-controller", "kafka-exporter", "kafka-broker",
		"loki-backend", "loki-gateway", "loki-write", "loki-read",
		"milvus-datanode", "milvus-indexnode", "milvus-minio",
		"milvus-mixcoord", "milvus-proxy", "milvus-querynode", "milvus-standalone",
		"minio",
		"mongodb",
		"mysql-8.0", "mysql-8.4", "mysql-5.7",
		"ob-ce",
		"orchestrator-raft", "orchestrator-shareend",
		"postgresql-12", "postgresql-14", "postgresql-15", "postgresql-16",
		"qdrant",
		"rabbitmq",
		"redis-7", "redis-cluster-7", "redis-sentinel-7", "redis-twemproxy-0.5",
		"pulsar-bookkeeper", "pulsar-broker-3", "pulsar-proxy-3", "pulsar-zookeeper-3",
		"starrocks-ce-be", "starrocks-ce-fe",
		"tidb-pd-7", "tidb-7", "tikv-7", "tikv-8", "tidb-8", "tidb-pd-8",
		"vanilla-postgresql-12", "vanilla-postgresql-14", "vanilla-postgresql-15", "vanilla-postgresql-supabase15",
		"vm-insert", "vm-select", "vm-storage",
		"weaviate",
		"zookeeper",
	}

	// 0.9 cmpd prefix => 1.0 cmpd prefix
	componentDefPrefixConvert = map[string]string{
		"etcd-":             "etcd-3",
		"ch-keeper-24":      "clickhouse-keeper-24",
		"pulsar-bkrecovery": "pulsar-bookies-recovery-3",
	}
)

func init() {
	registerCVConvert := func(cvName string, cvConvert map[string]componentDefRefConvert) {
		clusterVersionConvert[cvName] = cvConvert
	}
	// 1. apecloud-mysql
	apeMysqlCompDefConvert := componentDefRefConvert{cmpdPrefix: "apecloud-mysql", serviceVersion: "8.0.30"}
	apeMysqlCVConvert := map[string]componentDefRefConvert{
		"mysql":        apeMysqlCompDefConvert,
		"vtcontroller": apeMysqlCompDefConvert,
		"vtgate":       apeMysqlCompDefConvert,
	}
	registerCVConvert("ac-mysql-8.0.30", apeMysqlCVConvert)
	registerCVConvert("ac-mysql-8.0.30-1", apeMysqlCVConvert)

	registerCVConvert("clickhouse-24.8.3", map[string]componentDefRefConvert{
		"clickhouse": {cmpdPrefix: "clickhouse-24", serviceVersion: "24.8.3"},
		"ch-keeper":  {cmpdPrefix: "clickhouse-keeper-24", serviceVersion: "24.8.3"},
		// Warning: 3.8.0->3.8.4
		"zookeeper": {cmpdPrefix: "zookeeper", serviceVersion: "3.8.4"},
	})

	registerCVConvert("elasticsearch-8.8.2", map[string]componentDefRefConvert{
		"elasticsearch": {cmpdPrefix: "elasticsearch-8", serviceVersion: "8.8.2"},
	})

	registerCVConvert("etcd-v3.5.6", map[string]componentDefRefConvert{
		"etcd": {cmpdPrefix: "etcd-3", serviceVersion: "3.5.6"},
	})

	registerCVConvert("greptimedb-0.3.2", map[string]componentDefRefConvert{
		"datanode": {cmpdPrefix: "greptimedb-datanode", serviceVersion: "0.3.2"},
		// Warning: 3.5.5->3.5.6
		"etcd":     {cmpdPrefix: "etcd-3", serviceVersion: "3.5.6"},
		"meta":     {cmpdPrefix: "greptimedb-meta", serviceVersion: "0.3.2"},
		"frontend": {cmpdPrefix: "greptimedb-frontend", serviceVersion: "0.3.2"},
	})
	registerCVConvert("influxdb-2.7.4", map[string]componentDefRefConvert{
		"influxdb": {cmpdPrefix: "influxdb", serviceVersion: "2.7.4"},
	})
	registerCVConvert("kafka-3.3.2", map[string]componentDefRefConvert{
		"kafka-server":   {cmpdPrefix: "kafka-combine", serviceVersion: "3.3.2"},
		"kafka-broker":   {cmpdPrefix: "kafka-broker", serviceVersion: "3.3.2"},
		"controller":     {cmpdPrefix: "kafka-controller", serviceVersion: "3.3.2"},
		"kafka-exporter": {cmpdPrefix: "kafka-exporter", serviceVersion: "3.3.2"},
	})
	registerCVConvert("mariadb-10.6.15", map[string]componentDefRefConvert{
		"mariadb-compdef": {cmpdPrefix: "mariadb", serviceVersion: "10.6.15"},
	})
	registerCVConvert("mogdb-5.0.5", map[string]componentDefRefConvert{
		"mogdb": {cmpdPrefix: "mogdb", serviceVersion: "5.0.5"},
	})
	// mongodb
	registerCVConvert("mongodb-5.0", map[string]componentDefRefConvert{
		"mongodb": {cmpdPrefix: "mongodb", serviceVersion: "5.0.28"},
	})
	registerCVConvert("mongodb-6.0", map[string]componentDefRefConvert{
		"mongodb": {cmpdPrefix: "mongodb", serviceVersion: "6.0.16"},
	})
	registerCVConvert("mongodb-4.4", map[string]componentDefRefConvert{
		"mongodb": {cmpdPrefix: "mongodb", serviceVersion: "4.4.29"},
	})
	registerCVConvert("mongodb-4.2", map[string]componentDefRefConvert{
		"mongodb": {cmpdPrefix: "mongodb", serviceVersion: "4.2.24"},
	})

	registerCVConvert("mysql-8.0.33", map[string]componentDefRefConvert{
		"mysql": {cmpdPrefix: "mysql-8.0", serviceVersion: "8.0.33"},
	})
	registerCVConvert("mysql-5.7.44", map[string]componentDefRefConvert{
		"mysql": {cmpdPrefix: "mysql-5.7", serviceVersion: "5.7.44"},
	})
	registerCVConvert("mysql-8.4.2", map[string]componentDefRefConvert{
		"mysql": {cmpdPrefix: "mysql-8.4", serviceVersion: "8.4.2"},
	})
	registerCVConvert("nebula-v3.5.0", map[string]componentDefRefConvert{
		"nebula-console":  {cmpdPrefix: "nebula-console", serviceVersion: "3.5.0"},
		"nebula-graphd":   {cmpdPrefix: "nebula-graphd", serviceVersion: "3.5.0"},
		"nebula-metad":    {cmpdPrefix: "nebula-metad", serviceVersion: "3.5.0"},
		"nebula-storaged": {cmpdPrefix: "nebula-storaged", serviceVersion: "3.5.0"},
	})
	registerCVConvert("neon-pg14-1.0.0", map[string]componentDefRefConvert{
		"neon-compute":       {cmpdPrefix: "nneon-compute", serviceVersion: "1.0.0"},
		"neon-storagebroker": {cmpdPrefix: "neon-storagebroker", serviceVersion: "1.0.0"},
		"neon-safekeeper":    {cmpdPrefix: "neon-safekeeper", serviceVersion: "1.0.0"},
		"neon-pageserver":    {cmpdPrefix: "neon-pageserver", serviceVersion: "1.0.0"},
	})
	registerCVConvert("ob-ce-4.3.0.1-100000242024032211", map[string]componentDefRefConvert{
		"ob-ce": {cmpdPrefix: "oceanbase-ce", serviceVersion: "4.3.0.1"},
	})
	registerCVConvert("opensearch-2.7.0", map[string]componentDefRefConvert{
		"opensearch": {cmpdPrefix: "opensearch", serviceVersion: "2.7.0"},
	})
	registerCVConvert("orioledb-beta1", map[string]componentDefRefConvert{
		"orioledb": {cmpdPrefix: "orioledb", serviceVersion: "14.7.2"},
	})
	registerCVConvert("polardbx-v2.3", map[string]componentDefRefConvert{
		"gms": {cmpdPrefix: "polardbx-gms", serviceVersion: "2.3.0"},
		"dn":  {cmpdPrefix: "polardbx-dn", serviceVersion: "2.3.0"},
		"cn":  {cmpdPrefix: "polardbx-cn", serviceVersion: "2.3.0"},
		"cdc": {cmpdPrefix: "polardbx-cdc", serviceVersion: "2.3.0"},
	})
	// postgresql
	registerCVConvert("postgresql-14.7.2", map[string]componentDefRefConvert{
		"postgresql": {cmpdPrefix: "postgresql-14", serviceVersion: "14.7.2"},
	})
	registerCVConvert("postgresql-12.14.1", map[string]componentDefRefConvert{
		"postgresql": {cmpdPrefix: "postgresql-12", serviceVersion: "12.14.1"},
	})
	registerCVConvert("postgresql-12.15.0", map[string]componentDefRefConvert{
		"postgresql": {cmpdPrefix: "postgresql-12", serviceVersion: "12.15.0"},
	})
	registerCVConvert("postgresql-14.8.0", map[string]componentDefRefConvert{
		"postgresql": {cmpdPrefix: "postgresql-14", serviceVersion: "14.8.0"},
	})
	registerCVConvert("postgresql-12.14.0", map[string]componentDefRefConvert{
		"postgresql": {cmpdPrefix: "postgresql-12", serviceVersion: "12.14.0"},
	})
	registerCVConvert("postgresql-15.7.0", map[string]componentDefRefConvert{
		"postgresql": {cmpdPrefix: "postgresql-15", serviceVersion: "15.7.0"},
	})
	registerCVConvert("postgresql-16.4.0", map[string]componentDefRefConvert{
		"postgresql": {cmpdPrefix: "postgresql-16", serviceVersion: "16.4.0"},
	})
	// pulsar
	registerCVConvert("pulsar-2.11.2", map[string]componentDefRefConvert{
		"bookies":          {cmpdPrefix: "pulsar-bookkeeper-2", serviceVersion: "2.11.2"},
		"bookies-recovery": {cmpdPrefix: "pulsar-bookies-recovery-2", serviceVersion: "2.11.2"},
		"pulsar-broker":    {cmpdPrefix: "pulsar-broker-2", serviceVersion: "2.11.2"},
		"zookeeper":        {cmpdPrefix: "pulsar-zookeeper-2", serviceVersion: "2.11.2"},
		"pulsar-proxy":     {cmpdPrefix: "pulsar-proxy-2", serviceVersion: "2.11.2"},
	})
	registerCVConvert("pulsar-3.0.2", map[string]componentDefRefConvert{
		"bookies":          {cmpdPrefix: "pulsar-bookkeeper-3", serviceVersion: "3.0.2"},
		"bookies-recovery": {cmpdPrefix: "pulsar-bookies-recovery-3", serviceVersion: "3.0.2"},
		"pulsar-broker":    {cmpdPrefix: "pulsar-broker-3", serviceVersion: "3.0.2"},
		"zookeeper":        {cmpdPrefix: "pulsar-zookeeper-3", serviceVersion: "3.0.2"},
		"pulsar-proxy":     {cmpdPrefix: "pulsar-proxy-3", serviceVersion: "3.0.2"},
	})
	// qdrant
	registerCVConvert("qdrant-1.5.0", map[string]componentDefRefConvert{
		"qdrant": {cmpdPrefix: "qdrant", serviceVersion: "1.5.0"},
	})
	registerCVConvert("qdrant-1.7.3", map[string]componentDefRefConvert{
		"qdrant": {cmpdPrefix: "qdrant", serviceVersion: "1.7.3"},
	})
	registerCVConvert("qdrant-1.8.1", map[string]componentDefRefConvert{
		"qdrant": {cmpdPrefix: "qdrant", serviceVersion: "1.8.1"},
	})
	registerCVConvert("qdrant-1.8.4", map[string]componentDefRefConvert{
		"qdrant": {cmpdPrefix: "qdrant", serviceVersion: "1.8.4"},
	})
	registerCVConvert("qdrant-1.10.0", map[string]componentDefRefConvert{
		"qdrant": {cmpdPrefix: "qdrant", serviceVersion: "1.10.0"},
	})
	// redis
	registerCVConvert("redis-7.2.4", map[string]componentDefRefConvert{
		"redis": {cmpdPrefix: "redis-7", serviceVersion: "7.2.4"},
	})
	registerCVConvert("redis-7.0.6", map[string]componentDefRefConvert{
		"redis": {cmpdPrefix: "redis-7", serviceVersion: "7.0.6"},
	})
	registerCVConvert("risingwave-v1.0.0", map[string]componentDefRefConvert{
		"meta":      {cmpdPrefix: "risingwave-meta", serviceVersion: "v1.0.0"},
		"frontend":  {cmpdPrefix: "risingwave-frontend", serviceVersion: "v1.0.0"},
		"compute":   {cmpdPrefix: "risingwave-compute", serviceVersion: "v1.0.0"},
		"compactor": {cmpdPrefix: "risingwave-compactor", serviceVersion: "v1.0.0"},
		"connector": {cmpdPrefix: "risingwave-connector", serviceVersion: "v1.0.0"},
	})
	registerCVConvert("tdengine-3.0.5.0", map[string]componentDefRefConvert{
		"tdengine": {cmpdPrefix: "tdengine", serviceVersion: "3.0.5"},
	})
	registerCVConvert("weaviate-1.18.0", map[string]componentDefRefConvert{
		"weaviate": {cmpdPrefix: "weaviate", serviceVersion: "1.19.6"},
	})
	registerCVConvert("yashandb-personal-23.1.1.100", map[string]componentDefRefConvert{
		"yashandb-compdef": {cmpdPrefix: "yashandb", serviceVersion: "23.1.1-100"},
	})
}

type UpgradeToV1Options struct {
	Cmd *cobra.Command `json:"-"`

	Client    clientset.Interface
	f         cmdutil.Factory
	Dynamic   dynamic.Interface
	DryRun    bool
	NoDiff    bool
	Name      string
	Namespace string
	genericiooptions.IOStreams
	compDefList *unstructured.UnstructuredList
}

func NewUpgradeToV1Option(f cmdutil.Factory, streams genericiooptions.IOStreams) *UpgradeToV1Options {
	return &UpgradeToV1Options{
		f:         f,
		IOStreams: streams,
	}
}

var convertExample = templates.Examples(`
		# upgrade a v1alpha1 cluster to v1 cluster
		kbcli cluster upgrade-to-v1 mycluster

		# upgrade a v1alpha1 cluster with --dry-run
		kbcli cluster upgrade-to-v1 mycluster --dry-run
`)

func NewUpgradeToV1Cmd(f cmdutil.Factory, streams genericiooptions.IOStreams) *cobra.Command {
	o := NewUpgradeToV1Option(f, streams)
	cmd := &cobra.Command{
		Use:               "upgrade-to-v1 [NAME]",
		Short:             "upgrade cluster to v1 api version.",
		Example:           convertExample,
		ValidArgsFunction: util.ResourceNameCompletionFunc(f, types.ClusterGVR()),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.BehaviorOnFatal(printer.FatalWithRedColor)
			cmdutil.CheckErr(o.complete(args))
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().BoolVar(&o.DryRun, "dry-run", false, "dry run mode")
	cmd.Flags().BoolVar(&o.NoDiff, "no-diff", false, "only print the new cluster yaml")
	return cmd
}

func (o *UpgradeToV1Options) complete(args []string) error {

	if len(args) == 0 {
		return fmt.Errorf("must specify cluster name")
	}
	o.Name = args[0]
	o.Namespace, _, _ = o.f.ToRawKubeConfigLoader().Namespace()
	o.Dynamic, _ = o.f.DynamicClient()
	o.Client, _ = o.f.KubernetesClientSet()
	return nil
}

func (o *UpgradeToV1Options) GetConvertedCluster() (*kbappsv1.Cluster, *kbappsv1alpha1.Cluster, bool, error) {
	cluster := &kbappsv1.Cluster{}
	err := util.GetK8SClientObject(o.Dynamic, cluster, types.ClusterGVR(), o.Namespace, o.Name)
	if err != nil {
		return nil, nil, false, err
	}
	if cluster.Annotations[constant.CRDAPIVersionAnnotationKey] == kbappsv1.GroupVersion.String() {
		return nil, nil, false, fmt.Errorf("cluster %s is already v1", o.Name)
	}
	// 1. get cluster v1alpha1 spec
	clusterV1alpha1 := &kbappsv1alpha1.Cluster{}
	err = util.GetK8SClientObject(o.Dynamic, clusterV1alpha1, types.ClusterV1alphaGVR(), o.Namespace, o.Name)
	if err != nil {
		return nil, nil, false, err
	}
	o.compDefList, err = o.Dynamic.Resource(types.CompDefGVR()).Namespace("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, nil, false, err
	}

	var existUnsupportedSpec bool
	if len(clusterV1alpha1.Spec.ClusterVersionRef) > 0 {
		if err := o.ConvertForClusterVersion(cluster, clusterV1alpha1.Spec, &existUnsupportedSpec); err != nil {
			return nil, nil, false, err
		}
	} else {
		if err := o.Convert09ComponentDef(cluster, clusterV1alpha1.Spec, &existUnsupportedSpec); err != nil {
			return nil, nil, false, err
		}
	}
	delete(cluster.Annotations, kbIncrementConverterAK)
	cluster.Annotations[constant.CRDAPIVersionAnnotationKey] = kbappsv1.GroupVersion.String()
	return cluster, clusterV1alpha1, existUnsupportedSpec, nil
}

func (o *UpgradeToV1Options) Run() error {
	cluster, clusterV1alpha1, existUnsupportedSpec, err := o.GetConvertedCluster()
	if err != nil {
		return err
	}
	o.printDiff(clusterV1alpha1.DeepCopy(), cluster.DeepCopy())
	if existUnsupportedSpec {
		return fmt.Errorf(`cluster "%s" has unknown clusterVersion or componentDefinition, you can replace with accorrding ComponentDefinition with 1.0 api`, o.Name)
	}
	if o.DryRun {
		return nil
	}
	fmt.Println(printer.BoldYellow(fmt.Sprintf("Cluster %s will be converted to v1 with output as yaml.", o.Name)))
	if err = prompt.Confirm(nil, o.In, "", "Please type 'Yes/yes' to confirm your operation:"); err != nil {
		return err
	}
	if len(clusterV1alpha1.Spec.ClusterVersionRef) > 0 {
		if err = o.convertCredential(clusterV1alpha1.Spec.ClusterDefRef); err != nil {
			return err
		}
	}
	if err = o.convertAccounts(); err != nil {
		return err
	}
	// convert to v1
	clusterObj, err := apiruntime.DefaultUnstructuredConverter.ToUnstructured(cluster)
	if err != nil {
		return err
	}
	_, err = o.Dynamic.Resource(types.ClusterGVR()).Namespace(o.Namespace).Update(context.TODO(), &unstructured.Unstructured{Object: clusterObj}, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	output := fmt.Sprintf("Cluster %s has converted successfully, you can view the spec:", o.Name)
	printer.PrintLine(output)
	nextLine := fmt.Sprintf("\tkubectl get clusters.apps.kubeblocks.io %s -n %s -oyaml", o.Name, o.Namespace)
	printer.PrintLine(nextLine)
	if err = o.deleteConfiguration(); err != nil {
		return err
	}
	return o.normalizeConfigMaps()
}

func (o *UpgradeToV1Options) convertCredential(cdName string) error {
	oldSecret := &corev1.Secret{}
	err := util.GetK8SClientObject(o.Dynamic, oldSecret, types.SecretGVR(), o.Namespace, fmt.Sprintf("%s-conn-credential", o.Name))
	if err != nil {
		return err
	}
	var (
		compName    string
		accountName string
	)
	// TODO: support all cluster definition
	switch cdName {
	case "postgresql":
		compName = "postgresql"
		accountName = "postgres"
	case "mysql":
		compName = "mysql"
		accountName = "root"
	case "apecloud-mysql":
		compName = "mysql"
		accountName = "root"
	case "etcd":
		compName = "etcd"
		accountName = "root"
	case "weaviate":
		compName = "weaviate"
		accountName = "root"
	case "redis":
		compName = "redis"
		accountName = "default"
	case "qdrant":
		compName = "qdrant"
		accountName = "root"
	case "polardbx":
		compName = "gms"
		accountName = "polardbx_root"
	case "orioledb":
		compName = "orioledb"
		accountName = "postgres"
	case "neon":
		compName = "neon-compute"
		accountName = "cloud_admin"
	case "mongodb":
		compName = "mongodb"
		accountName = "root"
	default:
		return fmt.Errorf("unknown cluster definition %s", cdName)
	}
	newSecret := &corev1.Secret{}
	newSecret.Name = constant.GenerateAccountSecretName(o.Name, compName, accountName)
	newSecret.Namespace = oldSecret.Namespace
	newSecret.Labels = constant.GetCompLabels(o.Name, compName)
	newSecret.Labels["apps.kubeblocks.io/system-account"] = accountName
	newSecret.Data = map[string][]byte{
		"username": oldSecret.Data["username"],
		"password": oldSecret.Data["password"],
	}
	if _, err := o.Client.CoreV1().Secrets(oldSecret.Namespace).Create(context.TODO(), newSecret, metav1.CreateOptions{}); err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	return nil
}

func (o *UpgradeToV1Options) convertAccounts() error {
	secretList, err := o.Dynamic.Resource(types.SecretGVR()).Namespace(o.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", constant.AppInstanceLabelKey, o.Name),
	})
	if err != nil {
		return err
	}
	for i := range secretList.Items {
		secret := secretList.Items[i]
		labels := secret.GetLabels()
		account, ok := labels["account.kubeblocks.io/name"]
		if !ok {
			continue
		}
		labels["apps.kubeblocks.io/system-account"] = account
		secret.SetLabels(labels)
		if _, err = o.Dynamic.Resource(types.SecretGVR()).Namespace(o.Namespace).Update(context.TODO(), &secret, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func (o *UpgradeToV1Options) normalizeConfigMaps() error {
	cmList, err := o.Dynamic.Resource(types.ConfigmapGVR()).Namespace(o.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", constant.AppInstanceLabelKey, o.Name),
	})
	if err != nil {
		return err
	}
	patch := func(cm unstructured.Unstructured) error {
		// in-place upgrade
		newData, err := json.Marshal(cm)
		if err != nil {
			return err
		}
		if _, err = o.Dynamic.Resource(types.ConfigmapGVR()).Namespace(o.Namespace).Patch(context.TODO(), cm.GetName(), apitypes.MergePatchType, newData, metav1.PatchOptions{}); err != nil {
			return client.IgnoreNotFound(err)
		}
		return nil
	}
	for i := range cmList.Items {
		cm := cmList.Items[i]
		labels := cm.GetLabels()
		if _, ok := labels[constant.CMConfigurationSpecProviderLabelKey]; !ok {
			// add file-template label for scripts
			if _, isScripts := labels[constant.CMTemplateNameLabelKey]; isScripts {
				if err = o.Dynamic.Resource(types.ConfigmapGVR()).Namespace(cm.GetNamespace()).Delete(context.TODO(), cm.GetName(), metav1.DeleteOptions{}); client.IgnoreNotFound(err) != nil {
					return err
				}
			}
			continue
		}
		if _, ok := labels[constant.CMConfigurationSpecProviderLabelKey]; !ok {
			continue
		}
		cm.SetOwnerReferences(nil)
		if err = patch(cm); err != nil {
			return err
		}
	}
	return nil
}

func (o *UpgradeToV1Options) getLatestComponentDef(componentDefPrefix string) (string, error) {
	var matchedCompDefs []string
	for _, v := range o.compDefList.Items {
		if v.GetAnnotations()[constant.CRDAPIVersionAnnotationKey] != kbappsv1.GroupVersion.String() {
			continue
		}
		if strings.HasPrefix(v.GetName(), componentDefPrefix) {
			matchedCompDefs = append(matchedCompDefs, v.GetName())
		}
	}
	if len(matchedCompDefs) == 0 {
		return "", fmt.Errorf("no matched componentDefinition for componentDef %s", componentDefPrefix)
	}
	// TODO: sort with semantic version?
	slices.Sort(matchedCompDefs)
	return matchedCompDefs[len(matchedCompDefs)-1], nil
}

func (o *UpgradeToV1Options) deleteConfiguration() error {
	configList, err := o.Dynamic.Resource(types.ConfigurationGVR()).Namespace(o.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", constant.AppInstanceLabelKey, o.Name),
	})
	if err != nil {
		return err
	}
	for i := range configList.Items {
		if err = o.Dynamic.Resource(types.ConfigurationGVR()).Namespace(o.Namespace).Delete(context.TODO(), configList.Items[i].GetName(), metav1.DeleteOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func (o *UpgradeToV1Options) ConvertForClusterVersion(cluster *kbappsv1.Cluster,
	clusterV1alpha1Spec kbappsv1alpha1.ClusterSpec,
	existUnsupportedSpec *bool) error {
	convert, ok := clusterVersionConvert[clusterV1alpha1Spec.ClusterVersionRef]
	if !ok {
		*existUnsupportedSpec = true
		return nil
	}
	for i := range clusterV1alpha1Spec.ComponentSpecs {
		compDefRef := clusterV1alpha1Spec.ComponentSpecs[i].ComponentDefRef
		compDefConVert, ok := convert[compDefRef]
		if !ok {
			*existUnsupportedSpec = true
			cluster.Spec.ComponentSpecs[i].ComponentDef = "<yourComponentDef>"
			cluster.Spec.ComponentSpecs[i].ServiceVersion = "<yourServiceVersion>"
			continue
		}
		compDef, err := o.getLatestComponentDef(compDefConVert.cmpdPrefix)
		if err != nil {
			return err
		}
		cluster.Spec.ComponentSpecs[i].ComponentDef = compDef
		cluster.Spec.ComponentSpecs[i].ServiceVersion = compDefConVert.serviceVersion
	}
	// remove deprecated v1alpha1
	cluster.Spec.ClusterDef = ""
	return nil
}

func (o *UpgradeToV1Options) Convert09ComponentDef(cluster *kbappsv1.Cluster,
	clusterV1alpha1Spec kbappsv1alpha1.ClusterSpec,
	existUnsupportedSpec *bool) error {
	componentDefWithChartVersionSet := sets.New(componentDefWithChartVersion...)
	convertCompDef := func(compDef string) (string, error) {
		if componentDefWithChartVersionSet.Has(compDef) {
			return o.getLatestComponentDef(compDef)
		}
		for k, cmpdDefPrefix := range componentDefPrefixConvert {
			if strings.HasPrefix(compDef, k) {
				return o.getLatestComponentDef(cmpdDefPrefix)
			}
		}
		*existUnsupportedSpec = true
		return "<yourComponentDef>", nil
	}
	for i := range clusterV1alpha1Spec.ComponentSpecs {
		compDef, err := convertCompDef(clusterV1alpha1Spec.ComponentSpecs[i].ComponentDef)
		if err != nil {
			return err
		}
		cluster.Spec.ComponentSpecs[i].ComponentDef = compDef
		// reset service account name
		if cluster.Spec.ComponentSpecs[i].ServiceAccountName == fmt.Sprintf("kb-%s", cluster.Name) {
			cluster.Spec.ComponentSpecs[i].ServiceAccountName = ""
		}
	}
	for i := range clusterV1alpha1Spec.ShardingSpecs {
		compDef, err := convertCompDef(clusterV1alpha1Spec.ShardingSpecs[i].Template.ComponentDef)
		if err != nil {
			return err
		}
		cluster.Spec.Shardings[i].Template.ComponentDef = compDef
		// reset service account name
		if cluster.Spec.Shardings[i].Template.ServiceAccountName == fmt.Sprintf("kb-%s", cluster.Name) {
			cluster.Spec.Shardings[i].Template.ServiceAccountName = ""
		}
	}
	delete(cluster.Annotations, kbIncrementConverterAK)
	return nil
}

func (o *UpgradeToV1Options) printDiff(clusterV1Alpha1 *kbappsv1alpha1.Cluster, clusterV1 *kbappsv1.Cluster) {
	delete(clusterV1Alpha1.Annotations, corev1.LastAppliedConfigAnnotation)
	delete(clusterV1.Annotations, corev1.LastAppliedConfigAnnotation)
	clusterV1Alpha1.Status = kbappsv1alpha1.ClusterStatus{}
	clusterV1Alpha1.ObjectMeta.ManagedFields = nil
	clusterV1.Status = kbappsv1.ClusterStatus{}
	clusterV1.ObjectMeta.ManagedFields = nil
	clusterV1Alpha1Srr, _ := yaml.Marshal(clusterV1Alpha1)
	clusterV1Str, _ := yaml.Marshal(clusterV1)
	if o.NoDiff {
		fmt.Println(string(clusterV1Str))
		return
	}
	getDiffContext := func(isV1Cluster bool) string {
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(string(clusterV1Alpha1Srr), string(clusterV1Str), false)
		var diffStr string
		for _, d := range diffs {
			switch d.Type {
			case diffmatchpatch.DiffInsert:
				if isV1Cluster {
					diffStr += o.colorGreen(d.Text)
				}
			case diffmatchpatch.DiffDelete:
				if !isV1Cluster {
					diffStr += o.colorRed(d.Text)
				}
			case diffmatchpatch.DiffEqual:
				diffStr += d.Text
			}
		}
		return diffStr
	}
	// Add a purple, rectangular border
	var style = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("63"))
	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Left, style.Render(getDiffContext(false)), "    ", style.Render(getDiffContext(true))))
}

func (o *UpgradeToV1Options) colorRed(str string) string {
	return strings.ReplaceAll(color.RedString(str), "\n", fmt.Sprintf("%s[0m\n%s[31m", escape, escape))
}

func (o *UpgradeToV1Options) colorGreen(str string) string {
	return strings.ReplaceAll(color.GreenString(str), "\n", fmt.Sprintf("%s[0m\n%s[32m", escape, escape))
}
