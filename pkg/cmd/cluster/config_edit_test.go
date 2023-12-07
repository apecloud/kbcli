package cluster

import (
	"github.com/apecloud/kbcli/pkg/action"
	"github.com/apecloud/kbcli/pkg/types"
	appsv1alpha1 "github.com/apecloud/kubeblocks/apis/apps/v1alpha1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	dynamicfakeclient "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
)

func NewConfigEditTest(ns, cName string, opsType appsv1alpha1.OpsType, objs ...runtime.Object) (*cmdtesting.TestFactory, *action.CreateOptions) {
	streams, _, _, _ := genericiooptions.NewTestIOStreams()
	tf := cmdtesting.NewTestFactory().WithNamespace(ns)
	var editConfigOps = editConfigOptions{
		configOpsOptions: configOpsOptions{
			ComponentName: "mysql",
			Parameters:    []string{"config-spec"},
		},
		enableDelete: false,
	}
	baseOptions := &action.CreateOptions{
		IOStreams: streams,
		Name:      cName,
		Namespace: ns,
	}

	err := appsv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}

	// TODO  using config-spec and component to test the program
	ParamsMapping := map[schema.GroupVersionResource]string{
		types.ClusterDefGVR():       types.KindClusterDef + "List",
		types.ClusterVersionGVR():   types.KindClusterVersion + "List",
		types.ClusterGVR():          types.KindCluster + "List",
		types.ConfigConstraintGVR(): types.KindConfigConstraint + "List",
		types.BackupGVR():           types.KindBackup + "List",
		types.RestoreGVR():          types.KindRestore + "List",
		types.OpsGVR():              types.KindOps + "List",
	}
	baseOptions.Dynamic = dynamicfakeclient.NewSimpleDynamicClientWithCustomListKinds(scheme.Scheme, ParamsMapping, objs...)
	editConfigOptions.Run(editConfigOps, func() error {
		if opsType == appsv1alpha1.ReasonOpsRequestFailed {
			return errors.New("OpsRequestFailed")
		} else if opsType == appsv1alpha1.ClusterDefinitionKind {
			return errors.New("ClusterDefinitionKind")
		} else {
			return nil
		}
	})
	return tf, baseOptions
}
