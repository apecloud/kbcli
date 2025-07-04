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

package playground

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	gv "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"golang.org/x/net/context"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/klog/v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/yaml"

	cp "github.com/apecloud/kbcli/pkg/cloudprovider"
	"github.com/apecloud/kbcli/pkg/cluster"
	cmdcluster "github.com/apecloud/kbcli/pkg/cmd/cluster"
	"github.com/apecloud/kbcli/pkg/cmd/kubeblocks"
	"github.com/apecloud/kbcli/pkg/printer"
	"github.com/apecloud/kbcli/pkg/spinner"
	"github.com/apecloud/kbcli/pkg/types"
	"github.com/apecloud/kbcli/pkg/util"
	"github.com/apecloud/kbcli/pkg/util/helm"
	"github.com/apecloud/kbcli/pkg/util/prompt"
	"github.com/apecloud/kbcli/version"
)

var (
	initLong = templates.LongDesc(`Bootstrap a kubernetes cluster and install KubeBlocks for playground.

If no cloud provider is specified, a k3d cluster named kb-playground will be created on local host,
otherwise a kubernetes cluster will be created on the specified cloud. Then KubeBlocks will be installed
on the created kubernetes cluster, and an apecloud-mysql cluster named mycluster will be created.`)

	initExample = templates.Examples(`
		# create a k3d cluster on local host and install KubeBlocks
		kbcli playground init

		# create an AWS EKS cluster and install KubeBlocks, the region is required
		kbcli playground init --cloud-provider aws --region us-west-1

		# after init, run the following commands to experience KubeBlocks quickly
		# list database cluster and check its status
		kbcli cluster list

		# get cluster information
		kbcli cluster describe mycluster

		# connect to database
		kbcli exec -it mycluster-mysql-0 bash
	    mysql -h 127.1 -u root -p$MYSQL_ROOT_PASSWORD

		# view the Grafana
		kbcli dashboard open kubeblocks-grafana

		# destroy playground
		kbcli playground destroy`)

	supportedCloudProviders = []string{cp.Local, cp.AWS}

	spinnerMsg = func(format string, a ...any) spinner.Option {
		return spinner.WithMessage(fmt.Sprintf("%-50s", fmt.Sprintf(format, a...)))
	}
)

type initOptions struct {
	genericiooptions.IOStreams
	helmCfg       *helm.Config
	clusterType   string
	kbVersion     string
	cloudProvider string
	region        string
	autoApprove   bool
	dockerVersion *gv.Version

	k3dClusterOptions
	baseOptions
}

type k3dClusterOptions struct {
	k3sImage      string
	k3dProxyImage string
}

func newInitCmd(streams genericiooptions.IOStreams) *cobra.Command {
	o := &initOptions{
		IOStreams: streams,
	}

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Bootstrap a kubernetes cluster and install KubeBlocks for playground.",
		Long:    initLong,
		Example: initExample,
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.complete(cmd))
			util.CheckErr(o.validate())
			util.CheckErr(o.run())
		},
	}

	cmd.Flags().StringVar(&o.clusterType, "cluster-type", defaultClusterType, "Specify the cluster type to create, use 'kbcli cluster create --help' to get the available cluster type.")
	cmd.Flags().StringVar(&o.kbVersion, "version", version.DefaultKubeBlocksVersion, "KubeBlocks version")
	cmd.Flags().StringVar(&o.cloudProvider, "cloud-provider", defaultCloudProvider, fmt.Sprintf("Cloud provider type, one of %v", supportedCloudProviders))
	cmd.Flags().StringVar(&o.region, "region", "", "The region to create kubernetes cluster")
	cmd.Flags().DurationVar(&o.Timeout, "timeout", 600*time.Second, "Time to wait for init playground, such as --timeout=10m")
	cmd.Flags().BoolVar(&o.autoApprove, "auto-approve", false, "Skip interactive approval during the initialization of playground")
	cmd.Flags().StringVar(&o.k3sImage, "k3s-image", cp.K3sImageDefault, "Specify k3s image that you want to use for the nodes if you want to init playground locally")
	cmd.Flags().StringVar(&o.k3dProxyImage, "k3d-proxy-image", cp.K3dProxyImageDefault, "Specify k3d proxy image if you want to init playground locally")

	util.CheckErr(cmd.RegisterFlagCompletionFunc(
		"cloud-provider",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return cp.CloudProviders(), cobra.ShellCompDirectiveNoFileComp
		}))
	return cmd
}

func (o *initOptions) complete(cmd *cobra.Command) error {
	var err error

	if o.dockerVersion, err = util.GetDockerVersion(); err != nil {
		return err
	}
	// default write log to file
	if err = util.EnableLogToFile(cmd.Flags()); err != nil {
		fmt.Fprintf(o.Out, "Failed to enable the log file %s", err.Error())
	}

	return nil
}

func (o *initOptions) validate() error {
	if !slices.Contains(supportedCloudProviders, o.cloudProvider) {
		return fmt.Errorf("cloud provider %s is not supported, only support %v", o.cloudProvider, supportedCloudProviders)
	}

	if o.cloudProvider != cp.Local && o.region == "" {
		return fmt.Errorf("region should be specified when cloud provider %s is specified", o.cloudProvider)
	}

	if o.clusterType == "" {
		return fmt.Errorf("a valid cluster type is needed, use --cluster-type to specify one")
	}

	if o.cloudProvider == cp.Local && o.dockerVersion.LessThan(version.MinimumDockerVersion) {
		return fmt.Errorf("your docker version %s is lower than the minimum version %s, please upgrade your docker", o.dockerVersion, version.MinimumDockerVersion)
	}

	if err := o.baseOptions.validate(); err != nil {
		return err
	}
	return o.checkExistedCluster()
}

func (o *initOptions) run() error {
	if o.cloudProvider == cp.Local {
		return o.local()
	}
	return o.cloud()
}

// local bootstraps a playground in the local host
func (o *initOptions) local() error {
	provider, err := cp.New(o.cloudProvider, "", o.Out, o.ErrOut)
	if err != nil {
		return err
	}

	o.startTime = time.Now()

	var clusterInfo *cp.K8sClusterInfo
	if o.prevCluster != nil {
		clusterInfo = o.prevCluster
	} else {
		clusterInfo = &cp.K8sClusterInfo{
			CloudProvider: provider.Name(),
			ClusterName:   types.K3dClusterName,
			K3dClusterInfo: cp.K3dClusterInfo{
				K3sImage:      o.k3sImage,
				K3dProxyImage: o.k3dProxyImage,
			},
		}
	}

	if clusterInfo.K3sImage == "" {
		if o.prevCluster != nil {
			playgrouddir, err := initPlaygroundDir()
			if err != nil {
				return err
			}
			return fmt.Errorf("k3s image not specified, you can run `rm -rf %s ` and retry", playgrouddir)
		}
		clusterInfo.K3sImage = cp.K3sImageDefault
		clusterInfo.K3dProxyImage = cp.K3dProxyImageDefault
	}

	if err = writeClusterInfo(o.stateFilePath, clusterInfo); err != nil {
		return errors.Wrapf(err, "failed to write kubernetes cluster info to state file %s:\n  %v", o.stateFilePath, clusterInfo)
	}

	// create a local kubernetes cluster (k3d cluster) to deploy KubeBlocks
	s := spinner.New(o.Out, spinnerMsg("Create k3d cluster: "+clusterInfo.ClusterName))
	defer s.Fail()
	if err = provider.CreateK8sCluster(clusterInfo); err != nil {
		return errors.Wrap(err, "failed to set up k3d cluster")
	}
	s.Success()

	clusterInfo, err = o.writeStateFile(provider)
	if err != nil {
		return err
	}

	if err = o.setKubeConfig(clusterInfo); err != nil {
		return err
	}

	// install KubeBlocks and create a database cluster
	return o.installKBAndCluster(clusterInfo)
}

// bootstraps a playground in the remote cloud
func (o *initOptions) cloud() error {
	cpPath, err := cloudProviderRepoDir("")
	if err != nil {
		return err
	}

	var clusterInfo *cp.K8sClusterInfo

	// if kubernetes cluster exists, confirm to continue or not, if not, user should
	// destroy the old cluster first
	if o.prevCluster != nil {
		clusterInfo = o.prevCluster
		if err = o.confirmToContinue(); err != nil {
			return err
		}
	} else {
		clusterName := fmt.Sprintf("%s-%s", cloudClusterNamePrefix, rand.String(5))
		clusterInfo = &cp.K8sClusterInfo{
			ClusterName:   clusterName,
			CloudProvider: o.cloudProvider,
			Region:        o.region,
		}
		if err = o.confirmInitNewKubeCluster(); err != nil {
			return err
		}

		fmt.Fprintf(o.Out, "\nWrite cluster info to state file %s\n", o.stateFilePath)
		if err := writeClusterInfo(o.stateFilePath, clusterInfo); err != nil {
			return errors.Wrapf(err, "failed to write kubernetes cluster info to state file %s:\n  %v", o.stateFilePath, clusterInfo)
		}

		fmt.Fprintf(o.Out, "Creating %s %s cluster %s ... \n", o.cloudProvider, cp.K8sService(o.cloudProvider), clusterName)
	}

	o.startTime = time.Now()
	printer.PrintBlankLine(o.Out)

	// clone apecloud/cloud-provider repo to local path
	fmt.Fprintf(o.Out, "Clone ApeCloud cloud-provider repo to %s...\n", cpPath)
	branchName := "kb-playground"
	if err = util.CloneGitRepo(cp.GitRepoURL, branchName, cpPath); err != nil {
		return err
	}

	provider, err := cp.New(o.cloudProvider, cpPath, o.Out, o.ErrOut)
	if err != nil {
		return err
	}

	// create a kubernetes cluster in the cloud
	if err = provider.CreateK8sCluster(clusterInfo); err != nil {
		klog.V(1).Infof("create K8S cluster failed: %s", err.Error())
		return err
	}
	klog.V(1).Info("create K8S cluster success")

	printer.PrintBlankLine(o.Out)

	// write cluster info to state file and get new cluster info with kubeconfig
	clusterInfo, err = o.writeStateFile(provider)
	if err != nil {
		return err
	}

	// write cluster kubeconfig to default kubeconfig file and switch current context to it
	if err = o.setKubeConfig(clusterInfo); err != nil {
		return err
	}

	// install KubeBlocks and create a database cluster
	klog.V(1).Info("start to install KubeBlocks in K8S cluster... ")
	return o.installKBAndCluster(clusterInfo)
}

// confirmToContinue confirms to continue init process if there is an existed kubernetes cluster
func (o *initOptions) confirmToContinue() error {
	clusterName := o.prevCluster.ClusterName
	if !o.autoApprove {
		printer.Warning(o.Out, "Found an existed cluster %s, do you want to continue to initialize this cluster?\n  Only 'yes' will be accepted to confirm.\n\n", clusterName)
		entered, _ := prompt.NewPrompt("Enter a value:", nil, o.In).Run()
		if entered != yesStr {
			fmt.Fprintf(o.Out, "\nPlayground init cancelled, please destroy the old cluster first.\n")
			return cmdutil.ErrExit
		}
	}
	fmt.Fprintf(o.Out, "Continue to initialize %s %s cluster %s... \n",
		o.cloudProvider, cp.K8sService(o.cloudProvider), clusterName)
	return nil
}

func (o *initOptions) confirmInitNewKubeCluster() error {
	printer.Warning(o.Out, `This action will create a kubernetes cluster on the cloud that may
  incur charges. Be sure to delete your infrastructure properly to avoid additional charges.
`)

	fmt.Fprintf(o.Out, `
The whole process will take about %s, please wait patiently,
if it takes a long time, please check the network environment and try again.
`, printer.BoldRed("20 minutes"))

	if o.autoApprove {
		return nil
	}
	// confirm to run
	fmt.Fprintf(o.Out, "\nDo you want to perform this action?\n  Only 'yes' will be accepted to approve.\n\n")
	entered, _ := prompt.NewPrompt("Enter a value:", nil, o.In).Run()
	if entered != yesStr {
		fmt.Fprintf(o.Out, "\nPlayground init cancelled.\n")
		return cmdutil.ErrExit
	}
	return nil
}

// writeStateFile writes cluster info to state file and return the new cluster info with kubeconfig
func (o *initOptions) writeStateFile(provider cp.Interface) (*cp.K8sClusterInfo, error) {
	clusterInfo, err := provider.GetClusterInfo()
	if err != nil {
		return nil, err
	}
	if clusterInfo.KubeConfig == "" {
		return nil, errors.New("failed to get kubernetes cluster kubeconfig")
	}
	if err = writeClusterInfo(o.stateFilePath, clusterInfo); err != nil {
		return nil, errors.Wrapf(err, "failed to write kubernetes cluster info to state file %s:\n  %v",
			o.stateFilePath, clusterInfo)
	}
	return clusterInfo, nil
}

// merge created kubernetes cluster kubeconfig to ~/.kube/config and set it as default
func (o *initOptions) setKubeConfig(info *cp.K8sClusterInfo) error {
	s := spinner.New(o.Out, spinnerMsg("Merge kubeconfig to "+defaultKubeConfigPath))
	defer s.Fail()

	// check if the default kubeconfig file exists, if not, create it
	if _, err := os.Stat(defaultKubeConfigPath); os.IsNotExist(err) {
		if err = os.MkdirAll(filepath.Dir(defaultKubeConfigPath), 0755); err != nil {
			return errors.Wrapf(err, "failed to create directory %s", filepath.Dir(defaultKubeConfigPath))
		}
		if err = os.WriteFile(defaultKubeConfigPath, []byte{}, 0644); err != nil {
			return errors.Wrapf(err, "failed to create file %s", defaultKubeConfigPath)
		}
	}

	if err := kubeConfigWrite(info.KubeConfig, defaultKubeConfigPath,
		writeKubeConfigOptions{UpdateExisting: true, UpdateCurrentContext: true}); err != nil {
		return errors.Wrapf(err, "failed to write cluster %s kubeconfig", info.ClusterName)
	}
	s.Success()

	currentContext, err := kubeConfigCurrentContext(info.KubeConfig)
	s = spinner.New(o.Out, spinnerMsg("Switch current context to "+currentContext))
	defer s.Fail()
	if err != nil {
		return err
	}
	s.Success()

	return nil
}

func (o *initOptions) installKBAndCluster(info *cp.K8sClusterInfo) error {
	var err error

	// write kubeconfig content to a temporary file and use it
	if err = writeAndUseKubeConfig(info.KubeConfig, o.kubeConfigPath, o.Out); err != nil {
		return err
	}

	// create helm config
	o.helmCfg = helm.NewConfig("", o.kubeConfigPath, "", klog.V(1).Enabled())

	// install KubeBlocks
	if err = o.installKubeBlocks(info.ClusterName); err != nil {
		return errors.Wrap(err, "failed to install KubeBlocks")
	}
	klog.V(1).Info("KubeBlocks installed successfully")
	if err = o.createSnapshotController(); err != nil {
		return errors.Wrap(err, "failed to install snapshot controller")
	}
	klog.V(1).Info("create snapshot controller addon successfully")
	// install database cluster
	clusterInfo := "ClusterType: " + o.clusterType
	s := spinner.New(o.Out, spinnerMsg("Create cluster %s (%s)", kbClusterName, clusterInfo))
	defer s.Fail()
	if err = o.createCluster(); err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create cluster %s", kbClusterName)
	}
	s.Success()

	fmt.Fprintf(os.Stdout, "\nKubeBlocks playground init SUCCESSFULLY!\n\n")
	fmt.Fprintf(os.Stdout, "Kubernetes cluster \"%s\" has been created.\n", info.ClusterName)
	fmt.Fprintf(os.Stdout, "Cluster \"%s\" has been created.\n", kbClusterName)

	// output elapsed time
	if !o.startTime.IsZero() {
		fmt.Fprintf(o.Out, "Elapsed time: %s\n", time.Since(o.startTime).Truncate(time.Second))
	}

	fmt.Fprintf(o.Out, guideStr, kbClusterName)
	return nil
}

func (o *initOptions) installKubeBlocks(k8sClusterName string) error {
	f := util.NewFactory()
	client, err := f.KubernetesClientSet()
	if err != nil {
		return err
	}
	dynamic, err := f.DynamicClient()
	if err != nil {
		return err
	}
	insOpts := kubeblocks.InstallOptions{
		Options: kubeblocks.Options{
			HelmCfg:    o.helmCfg,
			Namespace:  defaultNamespace,
			IOStreams:  o.IOStreams,
			Client:     client,
			Dynamic:    dynamic,
			Wait:       true,
			WaitAddons: true,
			Timeout:    o.Timeout,
		},
		Version: o.kbVersion,
		Quiet:   true,
		Check:   true,
	}

	// enable monitor components by default
	insOpts.ValueOpts.Values = append(insOpts.ValueOpts.Values,
		"prometheus.enabled=true",
		"grafana.enabled=true",
		"agamotto.enabled=true",
	)

	if o.cloudProvider == cp.Local {
		insOpts.ValueOpts.Values = append(insOpts.ValueOpts.Values,
			// use hostpath csi driver to support snapshot
			"snapshot-controller.enabled=true",
			"csi-hostpath-driver.enabled=true",
		)
	} else if o.cloudProvider == cp.AWS {
		insOpts.ValueOpts.Values = append(insOpts.ValueOpts.Values,
			// enable aws-load-balancer-controller addon automatically on playground
			"aws-load-balancer-controller.enabled=true",
			fmt.Sprintf("aws-load-balancer-controller.clusterName=%s", k8sClusterName),
		)
	}

	if err = insOpts.PreCheck(); err != nil {
		// if the KubeBlocks has been installed, we ignore the error
		errMsg := err.Error()
		if strings.Contains(errMsg, "repeated installation is not supported") {
			fmt.Fprintf(o.Out, strings.Split(errMsg, ",")[0]+"\n")
			return nil
		}
		return err
	}
	if err = insOpts.CompleteInstallOptions(); err != nil {
		return err
	}
	return insOpts.Install()
}

func (o *initOptions) createSnapshotController() error {
	if o.cloudProvider != cp.Local {
		return nil
	}
	cli, err := util.NewFactory().DynamicClient()
	if err != nil {
		return err
	}
	_, currentFile, _, _ := runtime.Caller(1)
	baseDir := filepath.Dir(currentFile)
	getUnstructured := func(fileName string) (*unstructured.Unstructured, error) {
		cmBytes, err := os.ReadFile(fileName)
		if err != nil {
			return nil, err
		}
		cm := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(cmBytes, cm); err != nil {
			return nil, err
		}
		return cm, err
	}
	snapshotControllerCM, err := getUnstructured(baseDir + "/snapshot-controller/snapshot-controller-cm.yaml")
	if err != nil {
		return err
	}
	snapshotControllerCM.SetNamespace(defaultNamespace)
	if _, err = cli.Resource(types.ConfigmapGVR()).Namespace(defaultNamespace).Create(context.TODO(), snapshotControllerCM, metav1.CreateOptions{}); err != nil {
		return err
	}

	snapshotControllerAddon, err := getUnstructured(baseDir + "/snapshot-controller/snapshot-controller-addon.yaml")
	if err != nil {
		return err
	}
	if _, err = cli.Resource(types.AddonGVR()).Namespace("").Create(context.TODO(), snapshotControllerAddon, metav1.CreateOptions{}); err != nil {
		return err
	}
	return nil
}

// createCluster constructs a cluster create options and run
func (o *initOptions) createCluster() error {
	c, err := cmdcluster.NewSubCmdsOptions(&cmdcluster.NewCreateOptions(util.NewFactory(), genericiooptions.NewTestIOStreamsDiscard()).CreateOptions, cluster.ClusterType(o.clusterType))
	if err != nil {
		return err
	}
	c.Args = []string{kbClusterName}
	err = c.CreateOptions.Complete()
	if err != nil {
		return err
	}
	err = c.Complete(nil)
	if err != nil {
		return err
	}
	err = c.Validate()
	if err != nil {
		return err
	}
	return c.Run()
}

// checkExistedCluster checks playground kubernetes cluster exists or not, a kbcli client only
// support a single playground, they are bound to each other with a hidden context config file,
// the hidden file ensures that when destroy the playground it always goes with the fixed context,
// it makes the dangerous operation more safe and prevents from manipulating another context
func (o *initOptions) checkExistedCluster() error {
	if o.prevCluster == nil {
		return nil
	}

	warningMsg := fmt.Sprintf("playground only supports one kubernetes cluster,\n  if a cluster is already existed, please destroy it first.\n%s\n", o.prevCluster.String())
	// if cloud provider is not same with the existed cluster cloud provider, suggest
	// user to destroy the previous cluster first
	if o.prevCluster.CloudProvider != o.cloudProvider {
		printer.Warning(o.Out, warningMsg)
		return cmdutil.ErrExit
	}

	if o.prevCluster.CloudProvider == cp.Local {
		return nil
	}

	// previous kubernetes cluster is a cloud provider cluster, check if the region
	// is same with the new cluster region, if not, suggest user to destroy the previous
	// cluster first
	if o.prevCluster.Region != o.region {
		printer.Warning(o.Out, warningMsg)
		return cmdutil.ErrExit
	}
	return nil
}
